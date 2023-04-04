/*
Copyright (C) 2022-2023 Traefik Labs

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

package api

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformers "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	"github.com/traefik/hub-agent-kubernetes/pkg/edgeingress"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ktypes "k8s.io/apimachinery/pkg/types"
	kinformers "k8s.io/client-go/informers"
	kclientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
)

const portalCustomDomainSecretNamePrefix = "hub-certificate-portal-custom-domains"

// PlatformClient for the API service.
type PlatformClient interface {
	GetPortals(ctx context.Context) ([]Portal, error)
	GetWildcardCertificate(ctx context.Context) (edgeingress.Certificate, error)
	GetCertificateByDomains(ctx context.Context, domains []string) (edgeingress.Certificate, error)
	GetGateways(ctx context.Context) ([]Gateway, error)
	GetAPIs(ctx context.Context) ([]API, error)
	GetCollections(ctx context.Context) ([]Collection, error)
	GetAccesses(ctx context.Context) ([]Access, error)
}

// WatcherPortalConfig holds the portal watcher configuration.
type WatcherPortalConfig struct {
	IngressClassName            string
	AgentNamespace              string
	TraefikAPIEntryPoint        string
	TraefikTunnelEntryPoint     string
	DevPortalServiceName        string
	DevPortalPort               int
	PlatformIdentityProviderURL string

	PortalSyncInterval time.Duration
	CertSyncInterval   time.Duration
	CertRetryInterval  time.Duration
}

// WatcherPortal watches hub portals and sync them with the cluster.
type WatcherPortal struct {
	config *WatcherPortalConfig

	platform PlatformClient

	kubeClientSet kclientset.Interface
	kubeInformer  kinformers.SharedInformerFactory

	hubClientSet hubclientset.Interface
	hubInformer  hubinformers.SharedInformerFactory
}

// NewWatcherPortal returns a new WatcherPortal.
func NewWatcherPortal(client PlatformClient, kubeClientSet kclientset.Interface, kubeInformer kinformers.SharedInformerFactory, hubClientSet hubclientset.Interface, hubInformer hubinformers.SharedInformerFactory, config *WatcherPortalConfig) *WatcherPortal {
	return &WatcherPortal{
		config: config,

		platform: client,

		kubeClientSet: kubeClientSet,
		kubeInformer:  kubeInformer,

		hubClientSet: hubClientSet,
		hubInformer:  hubInformer,
	}
}

// Run runs WatcherPortal.
func (w *WatcherPortal) Run(ctx context.Context) {
	t := time.NewTicker(w.config.PortalSyncInterval)
	defer t.Stop()

	certSyncInterval := time.After(w.config.CertSyncInterval)
	ctxSync, cancel := context.WithTimeout(ctx, 20*time.Second)
	w.syncPortals(ctxSync)
	cancel()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping API watcher")
			return

		case <-t.C:
			ctxSync, cancel = context.WithTimeout(ctx, 20*time.Second)
			w.syncPortals(ctxSync)
			cancel()

		case <-certSyncInterval:
			ctxSync, cancel = context.WithTimeout(ctx, 20*time.Second)
			if err := w.syncCertificates(ctxSync); err != nil {
				log.Error().Err(err).Msg("Unable to synchronize certificates with platform")
				certSyncInterval = time.After(w.config.CertRetryInterval)
				cancel()
				continue
			}
			certSyncInterval = time.After(w.config.CertSyncInterval)
			cancel()
		}
	}
}

func (w *WatcherPortal) syncCertificates(ctx context.Context) error {
	clusterPortals, err := w.hubInformer.Hub().V1alpha1().APIPortals().Lister().List(labels.Everything())
	if err != nil {
		return err
	}

	for _, portal := range clusterPortals {
		if len(portal.Status.CustomDomains) == 0 {
			continue
		}

		if err = w.setupCertificates(ctx, portal); err != nil {
			log.Error().Err(err).
				Str("name", portal.Name).
				Str("namespace", portal.Namespace).
				Msg("unable to setup portal certificates")
		}
	}

	return nil
}

func (w *WatcherPortal) setupCertificates(ctx context.Context, portal *hubv1alpha1.APIPortal) error {
	cert, err := w.platform.GetCertificateByDomains(ctx, portal.Status.CustomDomains)
	if err != nil {
		return fmt.Errorf("get certificate by domains %q: %w", strings.Join(portal.Status.CustomDomains, ","), err)
	}

	secretName, err := getPortalCustomDomainSecretName(portal.Name)
	if err != nil {
		return fmt.Errorf("get portal custom domains secret name: %w", err)
	}

	if err = w.upsertCertificateSecret(ctx, cert, portal, secretName); err != nil {
		return fmt.Errorf("upsert certificate secret: %w", err)
	}

	return nil
}

func (w *WatcherPortal) syncPortals(ctx context.Context) {
	platformPortals, err := w.platform.GetPortals(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Unable to fetch APIPortals")
		return
	}

	clusterPortals, err := w.hubInformer.Hub().V1alpha1().APIPortals().Lister().List(labels.Everything())
	if err != nil {
		log.Error().Err(err).Msg("Unable to obtain APIPortals")
		return
	}

	portalsByName := map[string]*hubv1alpha1.APIPortal{}
	for _, portal := range clusterPortals {
		portalsByName[portal.Name] = portal
	}

	for _, portal := range platformPortals {
		platformPortal := portal

		clusterPortal, found := portalsByName[platformPortal.Name]

		// Portals that will remain in the map will be deleted.
		delete(portalsByName, platformPortal.Name)

		if !found {
			if err = w.createPortal(ctx, &platformPortal); err != nil {
				log.Error().Err(err).
					Str("name", platformPortal.Name).
					Msg("Unable to create APIPortal")
			}
			continue
		}

		if err = w.updatePortal(ctx, clusterPortal, &platformPortal); err != nil {
			log.Error().Err(err).
				Str("name", platformPortal.Name).
				Msg("Unable to update APIPortal")
		}
	}

	w.cleanPortals(ctx, portalsByName)
}

func (w *WatcherPortal) createPortal(ctx context.Context, portal *Portal) error {
	obj, err := portal.Resource()
	if err != nil {
		return fmt.Errorf("build APIPortal resource: %w", err)
	}

	obj, err = w.hubClientSet.HubV1alpha1().APIPortals().Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating APIPortal: %w", err)
	}

	log.Debug().
		Str("name", obj.Name).
		Msg("APIPortal created")

	return w.syncChildResources(ctx, obj, portal.HubACPConfig)
}

func (w *WatcherPortal) updatePortal(ctx context.Context, oldPortal *hubv1alpha1.APIPortal, newPortal *Portal) error {
	obj, err := newPortal.Resource()
	if err != nil {
		return fmt.Errorf("build APIPortal resource: %w", err)
	}

	obj.ObjectMeta = oldPortal.ObjectMeta

	if obj.Status.Version != oldPortal.Status.Version {
		obj, err = w.hubClientSet.HubV1alpha1().APIPortals().Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating APIPortal: %w", err)
		}

		log.Debug().
			Str("name", obj.Name).
			Msg("APIPortal updated")
	}

	return w.syncChildResources(ctx, obj, newPortal.HubACPConfig)
}

func (w *WatcherPortal) cleanPortals(ctx context.Context, portals map[string]*hubv1alpha1.APIPortal) {
	for _, portal := range portals {
		// Foreground propagation allow us to delete all resources owned by the APIPortal.
		policy := metav1.DeletePropagationForeground

		opts := metav1.DeleteOptions{
			PropagationPolicy: &policy,
		}
		err := w.hubClientSet.HubV1alpha1().APIPortals().Delete(ctx, portal.Name, opts)
		if err != nil {
			log.Error().Err(err).Msg("Unable to delete APIPortal")

			continue
		}

		log.Debug().
			Str("name", portal.Name).
			Msg("APIPortal deleted")
	}
}

func (w *WatcherPortal) syncChildResources(ctx context.Context, portal *hubv1alpha1.APIPortal, hubACPConfig OIDCConfig) error {
	acp, err := w.upsertPortalACP(ctx, portal, hubACPConfig)
	if err != nil {
		return fmt.Errorf("upsert portal ACP: %w", err)
	}

	if err = w.upsertPortalEdgeIngress(ctx, portal, acp.Name); err != nil {
		return fmt.Errorf("upsert portal edge ingress: %w", err)
	}

	if len(portal.Status.CustomDomains) == 0 {
		return nil
	}

	if err = w.setupCertificates(ctx, portal); err != nil {
		return fmt.Errorf("setup certificate: %w", err)
	}

	if err = w.upsertPortalIngress(ctx, portal, acp.Name); err != nil {
		return fmt.Errorf("upsert portal ingress: %w", err)
	}

	return nil
}

func (w *WatcherPortal) upsertPortalACP(ctx context.Context, portal *hubv1alpha1.APIPortal, hubACPConfig OIDCConfig) (*hubv1alpha1.AccessControlPolicy, error) {
	acpName, err := getACPPortalName(portal.Name)
	if err != nil {
		return nil, fmt.Errorf("get ACP name: %w", err)
	}

	if err = w.upsertACPSecret(ctx, portal, acpName, hubACPConfig.ClientSecret); err != nil {
		return nil, fmt.Errorf("upsert ACP secret: %w", err)
	}

	acp := &hubv1alpha1.AccessControlPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: acpName,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "traefik-hub",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: portal.APIVersion,
					Kind:       portal.Kind,
					Name:       portal.Name,
					UID:        portal.UID,
				},
			},
		},
		Spec: hubv1alpha1.AccessControlPolicySpec{
			OIDC: &hubv1alpha1.AccessControlPolicyOIDC{
				Issuer:      w.config.PlatformIdentityProviderURL,
				RedirectURL: "/callback",
				ClientID:    hubACPConfig.ClientID,
				Scopes:      []string{"openid", "offline_access"},
				Session: &hubv1alpha1.Session{
					Refresh: pointer.Bool(true),
				},
				Secret: &corev1.SecretReference{
					Name:      acpName,
					Namespace: w.config.AgentNamespace,
				},
				ForwardHeaders: map[string]string{
					"Hub-Groups": "groups",
					"Hub-Email":  "sub",
				},
			},
		},
	}

	existingACP, err := w.hubClientSet.HubV1alpha1().AccessControlPolicies().Get(ctx, acpName, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return nil, fmt.Errorf("get existing ACP: %w", err)
	}

	if kerror.IsNotFound(err) {
		existingACP, err = w.hubClientSet.HubV1alpha1().AccessControlPolicies().Create(ctx, acp, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("create ACP: %w", err)
		}

		log.Debug().
			Str("name", existingACP.Name).
			Msg("ACP created")

		return acp, nil
	}

	existingACP.Spec = acp.Spec
	// Override Annotations and Labels in case new values are added in the future.
	existingACP.ObjectMeta.Annotations = acp.ObjectMeta.Annotations
	existingACP.ObjectMeta.Labels = acp.ObjectMeta.Labels

	existingACP, err = w.hubClientSet.HubV1alpha1().AccessControlPolicies().Update(ctx, existingACP, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("update ACP: %w", err)
	}

	log.Debug().
		Str("name", existingACP.Name).
		Msg("ACP updated")

	return acp, nil
}

func (w *WatcherPortal) upsertACPSecret(ctx context.Context, portal *hubv1alpha1.APIPortal, name, clientSecret string) error {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: w.config.AgentNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: portal.APIVersion,
					Kind:       portal.Kind,
					Name:       portal.Name,
					UID:        portal.UID,
				},
			},
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "traefik-hub",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"clientSecret": []byte(clientSecret),
		},
	}

	existingSecret, err := w.kubeClientSet.CoreV1().Secrets(w.config.AgentNamespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get existing ACP secret: %w", err)
	}

	if kerror.IsNotFound(err) {
		_, err = w.kubeClientSet.CoreV1().Secrets(w.config.AgentNamespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create ACP secret: %w", err)
		}

		log.Debug().
			Str("name", secret.Name).
			Str("namespace", secret.Namespace).
			Msg("ACP secret created")

		return nil
	}

	existingSecret.Data = secret.Data
	existingSecret.ObjectMeta.Annotations = secret.ObjectMeta.Annotations
	existingSecret.ObjectMeta.Labels = secret.ObjectMeta.Labels

	_, err = w.kubeClientSet.CoreV1().Secrets(w.config.AgentNamespace).Update(ctx, existingSecret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update existing ACP secret: %w", err)
	}

	log.Debug().
		Str("name", existingSecret.Name).
		Str("namespace", existingSecret.Namespace).
		Msg("ACP secret updated")

	return nil
}

func (w *WatcherPortal) upsertPortalEdgeIngress(ctx context.Context, portal *hubv1alpha1.APIPortal, acpName string) error {
	ingName, err := getEdgeIngressPortalName(portal.Name)
	if err != nil {
		return fmt.Errorf("get edge ingress name: %w", err)
	}

	ing := &hubv1alpha1.EdgeIngress{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "hub.traefik.io/v1alpha1",
			Kind:       "EdgeIngress",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingName,
			Namespace: w.config.AgentNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "traefik-hub",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: portal.APIVersion,
					Kind:       portal.Kind,
					Name:       portal.Name,
					UID:        portal.UID,
				},
			},
		},
		Spec: hubv1alpha1.EdgeIngressSpec{
			ACP: &hubv1alpha1.EdgeIngressACP{
				Name: acpName,
			},
			Service: hubv1alpha1.EdgeIngressService{
				Name: w.config.DevPortalServiceName,
				Port: w.config.DevPortalPort,
			},
		},
	}

	clusterIng, err := w.hubClientSet.HubV1alpha1().EdgeIngresses(w.config.AgentNamespace).Get(ctx, ingName, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get edge ingress: %w", err)
	}

	if kerror.IsNotFound(err) {
		clusterIng, err = w.hubClientSet.HubV1alpha1().EdgeIngresses(w.config.AgentNamespace).Create(ctx, ing, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create edge ingress: %w", err)
		}

		log.Debug().
			Str("name", clusterIng.Name).
			Str("namespace", w.config.AgentNamespace).
			Msg("Edge ingress created")
	} else {
		clusterIng.Spec = ing.Spec
		// Override Annotations and Labels in case new values are added in the future.
		clusterIng.ObjectMeta.Annotations = ing.ObjectMeta.Annotations
		clusterIng.ObjectMeta.Labels = ing.ObjectMeta.Labels

		clusterIng, err = w.hubClientSet.HubV1alpha1().EdgeIngresses(w.config.AgentNamespace).Update(ctx, clusterIng, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update edge ingress: %w", err)
		}

		log.Debug().
			Str("name", clusterIng.Name).
			Str("namespace", w.config.AgentNamespace).
			Msg("Edge ingress updated")
	}

	// Set the APIPortal HubDomain with the domain obtained from the EdgeIngress.
	patch := []byte(fmt.Sprintf(`[
		{ "op": "replace", "path": "/status/hubDomain", "value": %q }
	]`, clusterIng.Status.Domain))

	if _, err = w.hubClientSet.HubV1alpha1().APIPortals().Patch(ctx, portal.Name, ktypes.JSONPatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("patch APIPortal: %w", err)
	}

	return nil
}

func (w *WatcherPortal) upsertPortalIngress(ctx context.Context, portal *hubv1alpha1.APIPortal, acpName string) error {
	ingressName, err := getIngressPortalName(portal.Name)
	if err != nil {
		return fmt.Errorf("get ingress name: %w", err)
	}

	secretName, err := getPortalCustomDomainSecretName(portal.Name)
	if err != nil {
		return fmt.Errorf("get portal custom domains secret name: %w", err)
	}

	existingIngress, err := w.kubeClientSet.NetworkingV1().Ingresses(w.config.AgentNamespace).Get(ctx, ingressName, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get ingress: %w", err)
	}

	ingress := w.buildIngress(portal, ingressName, secretName, acpName)
	if kerror.IsNotFound(err) {
		_, err = w.kubeClientSet.NetworkingV1().Ingresses(w.config.AgentNamespace).Create(ctx, ingress, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create ingress: %w", err)
		}

		log.Debug().
			Str("name", ingress.Name).
			Str("namespace", ingress.Namespace).
			Msg("Ingress created")

		return nil
	}

	existingIngress.Spec = ingress.Spec
	existingIngress.ObjectMeta.Annotations = ingress.ObjectMeta.Annotations
	existingIngress.ObjectMeta.Labels = ingress.ObjectMeta.Labels

	_, err = w.kubeClientSet.NetworkingV1().Ingresses(ingress.Namespace).Update(ctx, existingIngress, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update ingress: %w", err)
	}

	log.Debug().
		Str("name", ingress.Name).
		Str("namespace", ingress.Namespace).
		Msg("Ingress updated")

	return nil
}

func (w *WatcherPortal) buildIngress(portal *hubv1alpha1.APIPortal, ingressName, secretName, acpName string) *netv1.Ingress {
	pathPrefix := netv1.PathTypePrefix
	rule := netv1.IngressRuleValue{
		HTTP: &netv1.HTTPIngressRuleValue{
			Paths: []netv1.HTTPIngressPath{
				{
					PathType: &pathPrefix,
					Path:     "/",
					Backend: netv1.IngressBackend{
						Service: &netv1.IngressServiceBackend{
							Name: w.config.DevPortalServiceName,
							Port: netv1.ServiceBackendPort{
								Number: int32(w.config.DevPortalPort),
							},
						},
					},
				},
			},
		},
	}
	var rules []netv1.IngressRule
	for _, customDomain := range portal.Status.CustomDomains {
		rules = append(rules, netv1.IngressRule{
			Host:             customDomain,
			IngressRuleValue: rule,
		})
	}

	return &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.k8s.io/v1",
			Kind:       "Ingress",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName,
			Namespace: w.config.AgentNamespace,
			Annotations: map[string]string{
				"hub.traefik.io/access-control-policy":             acpName,
				"traefik.ingress.kubernetes.io/router.tls":         "true",
				"traefik.ingress.kubernetes.io/router.entrypoints": w.config.TraefikAPIEntryPoint,
			},
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "traefik-hub",
			},
			// Set OwnerReference allow us to delete Ingresses owned by an APIPortal.
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: portal.APIVersion,
					Kind:       portal.Kind,
					Name:       portal.Name,
					UID:        portal.UID,
				},
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: pointer.String(w.config.IngressClassName),
			TLS: []netv1.IngressTLS{
				{
					Hosts:      portal.Status.CustomDomains,
					SecretName: secretName,
				},
			},
			Rules: rules,
		},
	}
}

func (w *WatcherPortal) upsertCertificateSecret(ctx context.Context, cert edgeingress.Certificate, portal *hubv1alpha1.APIPortal, name string) error {
	existingSecret, err := w.kubeClientSet.CoreV1().Secrets(w.config.AgentNamespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get secret: %w", err)
	}

	secret := w.buildSecret(cert, portal, name)
	if kerror.IsNotFound(err) {
		_, err = w.kubeClientSet.CoreV1().Secrets(w.config.AgentNamespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create secret: %w", err)
		}

		log.Debug().
			Str("name", secret.Name).
			Str("namespace", secret.Namespace).
			Msg("Portal certificate Secret created")

		return nil
	}

	existingSecret.Data = secret.Data
	existingSecret.ObjectMeta.Annotations = secret.ObjectMeta.Annotations
	existingSecret.ObjectMeta.Labels = secret.ObjectMeta.Labels

	_, err = w.kubeClientSet.CoreV1().Secrets(w.config.AgentNamespace).Update(ctx, existingSecret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update secret: %w", err)
	}

	log.Debug().
		Str("name", existingSecret.Name).
		Str("namespace", existingSecret.Namespace).
		Msg("Portal certificate Secret updated")

	return nil
}

func (w *WatcherPortal) buildSecret(cert edgeingress.Certificate, portal *hubv1alpha1.APIPortal, name string) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: w.config.AgentNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "traefik-hub",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: portal.APIVersion,
				Kind:       portal.Kind,
				Name:       portal.Name,
				UID:        portal.UID,
			}},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": cert.Certificate,
			"tls.key": cert.PrivateKey,
		},
	}
}

// getEdgeIngressPortalName compute the name of the edge ingress of a portal.
// The name follow this format: {portal-name}-{hash(portal-name)}-portal
// This hash is here to reduce the chance of getting a collision on an existing ingress.
func getEdgeIngressPortalName(portalName string) (string, error) {
	h, err := hash(portalName)
	if err != nil {
		return "", err
	}

	// EdgeIngresses generate Ingresses with the same name. Therefore, to prevent any conflicts between the portal
	// ingress and the portal ingresses the term "-portal" must be added as a suffix.
	return fmt.Sprintf("%s-%d-portal", portalName, h), nil
}

// getIngressPortalName compute the name of the ingress of a portal.
// The name follow this format: {portal-name}-{hash(portal-name)}-portal-ing
// This hash is here to reduce the chance of getting a collision on an existing ingress.
func getIngressPortalName(portalName string) (string, error) {
	h, err := hash(portalName)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d-portal-ing", portalName, h), nil
}

// getACPPortalName compute the name of the Hub ACP of a portal.
// The name follow this format: {portal-name}-{hash(portal-name)}-portal-acp
// This hash is here to reduce the chance of getting a collision on an existing ACP.
func getACPPortalName(portalName string) (string, error) {
	h, err := hash(portalName)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d-portal-acp", portalName, h), nil
}

// getPortalCustomDomainSecretName compute the name of the secret storing the certificate of the portal custom domains.
// The name follow this format: {portalCustomDomainSecretNamePrefix}-{hash(portal-name)}
// This hash is here to reduce the chance of getting a collision on an existing secret while staying under
// the limit of 63 characters.
func getPortalCustomDomainSecretName(portalName string) (string, error) {
	h, err := hash(portalName)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d", portalCustomDomainSecretNamePrefix, h), nil
}

func hash(name string) (uint32, error) {
	h := fnv.New32()

	if _, err := h.Write([]byte(name)); err != nil {
		return 0, fmt.Errorf("generate hash: %w", err)
	}

	return h.Sum32(), nil
}
