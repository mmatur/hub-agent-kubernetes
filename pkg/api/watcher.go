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
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	traefikv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/traefik/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	"github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/typed/traefik/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/edgeingress"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
)

const (
	hubDomainSecretName          = "hub-certificate"
	customDomainSecretNamePrefix = "hub-certificate-custom-domains"
)

// PlatformClient for the API service.
type PlatformClient interface {
	GetPortals(ctx context.Context) ([]Portal, error)
	GetWildcardCertificate(ctx context.Context) (edgeingress.Certificate, error)
	GetCertificateByDomains(ctx context.Context, domains []string) (edgeingress.Certificate, error)
}

// WatcherConfig holds the watcher configuration.
type WatcherConfig struct {
	IngressClassName        string
	AgentNamespace          string
	TraefikAPIEntryPoint    string
	TraefikTunnelEntryPoint string
	DevPortalServiceName    string
	DevPortalPort           int

	APISyncInterval   time.Duration
	CertSyncInterval  time.Duration
	CertRetryInterval time.Duration
}

// Watcher watches hub portals and sync them with the cluster.
type Watcher struct {
	config *WatcherConfig

	wildCardCertMu sync.RWMutex
	wildCardCert   edgeingress.Certificate

	platform PlatformClient

	kubeClientSet clientset.Interface
	kubeInformer  informers.SharedInformerFactory

	hubClientSet hubclientset.Interface
	hubInformer  hubinformer.SharedInformerFactory

	traefikClientSet v1alpha1.TraefikV1alpha1Interface
}

// NewWatcher returns a new Watcher.
func NewWatcher(client PlatformClient, kubeClientSet clientset.Interface, kubeInformer informers.SharedInformerFactory, hubClientSet hubclientset.Interface, hubInformer hubinformer.SharedInformerFactory, traefikClientSet v1alpha1.TraefikV1alpha1Interface, config *WatcherConfig) *Watcher {
	return &Watcher{
		config: config,

		platform: client,

		kubeClientSet: kubeClientSet,
		kubeInformer:  kubeInformer,

		hubClientSet: hubClientSet,
		hubInformer:  hubInformer,

		traefikClientSet: traefikClientSet,
	}
}

// Run runs Watcher.
func (w *Watcher) Run(ctx context.Context) {
	t := time.NewTicker(w.config.APISyncInterval)
	defer t.Stop()

	certSyncInterval := time.After(w.config.CertSyncInterval)
	ctxSync, cancel := context.WithTimeout(ctx, 20*time.Second)
	if err := w.syncCertificates(ctxSync); err != nil {
		log.Error().Err(err).Msg("Unable to synchronize certificates with platform")
		certSyncInterval = time.After(w.config.CertRetryInterval)
	}
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

func (w *Watcher) syncCertificates(ctx context.Context) error {
	wildcardCert, err := w.platform.GetWildcardCertificate(ctx)
	if err != nil {
		return fmt.Errorf("get wildcardCert: %w", err)
	}

	w.wildCardCertMu.RLock()
	if bytes.Equal(wildcardCert.Certificate, w.wildCardCert.Certificate) &&
		bytes.Equal(wildcardCert.PrivateKey, w.wildCardCert.PrivateKey) {
		w.wildCardCertMu.RUnlock()

		return nil
	}
	w.wildCardCertMu.RUnlock()

	if err = w.upsertSecret(ctx, wildcardCert, hubDomainSecretName, w.config.AgentNamespace, nil); err != nil {
		return fmt.Errorf("upsert secret: %w", err)
	}

	w.wildCardCertMu.Lock()
	w.wildCardCert = wildcardCert
	w.wildCardCertMu.Unlock()

	clusterPortals, err := w.hubInformer.Hub().V1alpha1().APIPortals().Lister().List(labels.Everything())
	if err != nil {
		return err
	}

	for _, portal := range clusterPortals {
		err = w.setupCertificates(ctx, portal)
		if err != nil {
			log.Error().Err(err).
				Str("name", portal.Name).
				Str("namespace", portal.Namespace).
				Msg("unable to setup portal certificates")
		}
	}

	return nil
}

func (w *Watcher) setupCertificates(ctx context.Context, portal *hubv1alpha1.APIPortal) error {
	apisNamespaces := make(map[string]struct{})
	// TODO: fill apisNamespaces from portal APIAccesses

	if len(portal.Status.CustomDomains) == 0 {
		return nil
	}

	var cert edgeingress.Certificate
	// TODO: fill cert with the certificate for gateways custom domains

	secretName, err := getCustomDomainSecretName(portal.Name)
	if err != nil {
		return fmt.Errorf("get custom domains secret name: %w", err)
	}

	for namespace := range apisNamespaces {
		if err := w.upsertSecret(ctx, cert, secretName, namespace, portal); err != nil {
			return fmt.Errorf("upsert secret: %w", err)
		}
	}

	return nil
}

func (w *Watcher) upsertSecret(ctx context.Context, cert edgeingress.Certificate, name, namespace string, p *hubv1alpha1.APIPortal) error {
	secret, err := w.kubeClientSet.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get secret: %w", err)
	}

	if kerror.IsNotFound(err) {
		secret = &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "core.k8s.io/v1",
				Kind:       "Secret",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "traefik-hub",
				},
			},
			Type: corev1.SecretTypeTLS,
			Data: map[string][]byte{
				"tls.crt": cert.Certificate,
				"tls.key": cert.PrivateKey,
			},
		}
		if p != nil {
			secret.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: "hub.traefik.io/v1alpha1",
				Kind:       "APIPortal",
				Name:       p.Name,
				UID:        p.UID,
			}}
		}

		_, err = w.kubeClientSet.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create secret: %w", err)
		}

		log.Debug().
			Str("name", secret.Name).
			Str("namespace", secret.Namespace).
			Msg("Secret created")

		return nil
	}

	newOwners := secret.OwnerReferences
	if p != nil {
		newOwners = appendOwnerReference(secret.OwnerReferences, metav1.OwnerReference{
			APIVersion: "hub.traefik.io/v1alpha1",
			Kind:       "APIPortal",
			Name:       p.Name,
			UID:        p.UID,
		})
	}
	if bytes.Equal(secret.Data["tls.crt"], cert.Certificate) && len(secret.OwnerReferences) == len(newOwners) {
		return nil
	}

	secret.Data = map[string][]byte{
		"tls.crt": cert.Certificate,
		"tls.key": cert.PrivateKey,
	}
	secret.OwnerReferences = newOwners

	_, err = w.kubeClientSet.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update secret: %w", err)
	}

	log.Debug().
		Str("name", secret.Name).
		Str("namespace", secret.Namespace).
		Msg("Secret updated")

	return nil
}

func appendOwnerReference(references []metav1.OwnerReference, ref metav1.OwnerReference) []metav1.OwnerReference {
	for _, reference := range references {
		if reference.String() == ref.String() {
			return references
		}
	}

	return append(references, ref)
}

func (w *Watcher) syncPortals(ctx context.Context) {
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

func (w *Watcher) createPortal(ctx context.Context, portal *Portal) error {
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

	return w.syncChildResources(ctx, obj)
}

func (w *Watcher) updatePortal(ctx context.Context, oldPortal *hubv1alpha1.APIPortal, newPortal *Portal) error {
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

	return w.syncChildResources(ctx, obj)
}

func (w *Watcher) cleanPortals(ctx context.Context, portals map[string]*hubv1alpha1.APIPortal) {
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

func (w *Watcher) syncChildResources(ctx context.Context, portal *hubv1alpha1.APIPortal) error {
	if err := w.setupCertificates(ctx, portal); err != nil {
		return fmt.Errorf("unable to setup APIPortal certificates: %w", err)
	}

	if err := w.cleanupIngresses(ctx, portal); err != nil {
		return fmt.Errorf("clean up ingresses: %w", err)
	}

	if err := w.upsertIngresses(ctx, portal); err != nil {
		return fmt.Errorf("upsert ingresses: %w", err)
	}

	if err := w.upsertPortalEdgeIngress(ctx, portal); err != nil {
		return fmt.Errorf("upsert portal edge ingress: %w", err)
	}

	return nil
}

func (w *Watcher) upsertPortalEdgeIngress(ctx context.Context, portal *hubv1alpha1.APIPortal) error {
	ingName, err := getEdgeIngressPortalName(portal.Name)
	if err != nil {
		return fmt.Errorf("get edge ingress name: %w", err)
	}

	clusterIng, err := w.hubClientSet.HubV1alpha1().EdgeIngresses(w.config.AgentNamespace).Get(ctx, ingName, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get edge ingress: %w", err)
	}

	if kerror.IsNotFound(err) {
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
				Service: hubv1alpha1.EdgeIngressService{
					Name: w.config.DevPortalServiceName,
					Port: w.config.DevPortalPort,
				},
				CustomDomains: portal.Spec.CustomDomains,
			},
		}

		clusterIng, err = w.hubClientSet.HubV1alpha1().EdgeIngresses(w.config.AgentNamespace).Create(ctx, ing, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create edge ingress: %w", err)
		}
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

func (w *Watcher) upsertIngresses(ctx context.Context, portal *hubv1alpha1.APIPortal) error {
	apisByNamespace := make(map[string][]hubv1alpha1.API)
	// TODO: fill apisByNamespace from portal APIs

	for namespace, apis := range apisByNamespace {
		traefikMiddlewareName, err := w.setupStripPrefixMiddleware(ctx, portal, apis, namespace)
		if err != nil {
			return fmt.Errorf("setup stripPrefix middleware: %w", err)
		}

		ingress, err := w.buildHubDomainIngress(namespace, portal, apis, traefikMiddlewareName)
		if err != nil {
			return fmt.Errorf("build ingress for hub domain and namespace %q: %w", namespace, err)
		}

		if err = w.upsertIngress(ctx, ingress); err != nil {
			return fmt.Errorf("upsert ingress for hub domain and namespace %q: %w", namespace, err)
		}

		if len(portal.Status.CustomDomains) != 0 {
			ingress, err = w.buildCustomDomainsIngress(namespace, portal, apis, traefikMiddlewareName)
			if err != nil {
				return fmt.Errorf("build ingress for custom domains and namespace %q: %w", namespace, err)
			}

			if err = w.upsertIngress(ctx, ingress); err != nil {
				return fmt.Errorf("upsert ingress for custom domains and namespace %q: %w", namespace, err)
			}
		}
	}

	return nil
}

func (w *Watcher) setupStripPrefixMiddleware(ctx context.Context, portal *hubv1alpha1.APIPortal, apis []hubv1alpha1.API, namespace string) (string, error) {
	name, err := getStripPrefixMiddlewareName(portal.Name)
	if err != nil {
		return "", fmt.Errorf("get stripPrefix middleware name: %w", err)
	}

	existingMiddleware, existingErr := w.traefikClientSet.Middlewares(namespace).Get(ctx, name, metav1.GetOptions{})
	if existingErr != nil && !kerror.IsNotFound(existingErr) {
		return "", fmt.Errorf("get middleware: %w", existingErr)
	}

	middleware := newStripPrefixMiddleware(name, namespace, apis)

	traefikMiddlewareName, err := getTraefikStripPrefixMiddlewareName(namespace, portal)
	if err != nil {
		return "", fmt.Errorf("get Traefik stripPrefix middleware name: %w", err)
	}

	if kerror.IsNotFound(existingErr) {
		_, err = w.traefikClientSet.Middlewares(namespace).Create(ctx, &middleware, metav1.CreateOptions{})
		if err != nil {
			return "", fmt.Errorf("create middleware: %w", err)
		}

		log.Debug().
			Str("name", name).
			Str("namespace", namespace).
			Msg("Middleware created")

		return traefikMiddlewareName, nil
	}

	if middleware.Spec == existingMiddleware.Spec {
		return traefikMiddlewareName, nil
	}

	existingMiddleware.Spec = middleware.Spec

	_, err = w.traefikClientSet.Middlewares(existingMiddleware.Namespace).Update(ctx, existingMiddleware, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("update middleware: %w", err)
	}

	return traefikMiddlewareName, nil
}

func (w *Watcher) upsertIngress(ctx context.Context, ingress *netv1.Ingress) error {
	existingIngress, err := w.kubeClientSet.NetworkingV1().Ingresses(ingress.Namespace).Get(ctx, ingress.Name, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get ingress: %w", err)
	}

	if kerror.IsNotFound(err) {
		_, err = w.kubeClientSet.NetworkingV1().Ingresses(ingress.Namespace).Create(ctx, ingress, metav1.CreateOptions{})
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
	// Override Annotations and Labels in case new values are added in the future.
	existingIngress.ObjectMeta.Annotations = ingress.ObjectMeta.Annotations
	existingIngress.ObjectMeta.Labels = ingress.ObjectMeta.Labels

	_, err = w.kubeClientSet.NetworkingV1().Ingresses(ingress.Namespace).Update(ctx, existingIngress, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update ingress: %w", err)
	}

	return nil
}

// cleanupIngresses deletes the ingresses from namespaces that are no longer referenced in the APIPortal.
func (w *Watcher) cleanupIngresses(ctx context.Context, portal *hubv1alpha1.APIPortal) error {
	managedByHub, err := labels.NewRequirement("app.kubernetes.io/managed-by", selection.Equals, []string{"traefik-hub"})
	if err != nil {
		return fmt.Errorf("create managed by hub requirement: %w", err)
	}
	hubIngressesSelector := labels.NewSelector().Add(*managedByHub)
	hubIngresses, err := w.kubeInformer.Networking().V1().Ingresses().Lister().List(hubIngressesSelector)
	if err != nil {
		return fmt.Errorf("list ingresses: %w", err)
	}

	hubDomainIngressName, err := getHubDomainIngressName(portal.Name)
	if err != nil {
		return fmt.Errorf("get ingress name for hub domain: %w", err)
	}
	customDomainsIngressName, err := getCustomDomainsIngressName(portal.Name)
	if err != nil {
		return fmt.Errorf("get ingress name for custom domains: %w", err)
	}

	apisNamespaces := make(map[string]struct{})
	// TODO: fill apisNamespaces from portal APIs

	for _, ingress := range hubIngresses {
		if ingress.Name != hubDomainIngressName && ingress.Name != customDomainsIngressName {
			continue
		}

		if _, ok := apisNamespaces[ingress.Namespace]; !ok {
			err = w.kubeClientSet.CoreV1().
				Secrets(ingress.Namespace).
				Delete(ctx, ingress.Spec.TLS[0].SecretName, metav1.DeleteOptions{})

			if err != nil {
				log.Ctx(ctx).
					Error().
					Err(err).
					Str("api_portal", portal.Name).
					Str("secret_name", ingress.Spec.TLS[0].SecretName).
					Str("secret_namespace", ingress.Namespace).
					Msg("Unable to clean APIPortal's child Secret")
			}

			middlewareName, err := getStripPrefixMiddlewareName(portal.Name)
			if err != nil {
				log.Ctx(ctx).
					Error().
					Err(err).
					Str("api_portal", portal.Name).
					Str("middleware_namespace", ingress.Namespace).
					Msg("Unable to get APIPortal's child Middleware name")

				continue
			}

			err = w.traefikClientSet.
				Middlewares(ingress.Namespace).
				Delete(ctx, middlewareName, metav1.DeleteOptions{})

			if err != nil && !kerror.IsNotFound(err) {
				log.Ctx(ctx).
					Error().
					Err(err).
					Str("api_portal", portal.Name).
					Str("middleware_name", middlewareName).
					Str("middleware_namespace", ingress.Namespace).
					Msg("Unable to clean APIPortal's child Middleware")

				continue
			}

			err = w.kubeClientSet.NetworkingV1().
				Ingresses(ingress.Namespace).
				Delete(ctx, ingress.Name, metav1.DeleteOptions{})

			if err != nil {
				log.Ctx(ctx).
					Error().
					Err(err).
					Str("api_portal", portal.Name).
					Str("ingress_name", ingress.Name).
					Str("ingress_namespace", ingress.Namespace).
					Msg("Unable to clean APIPortal's child Ingress")

				continue
			}
		}
	}

	return nil
}

func (w *Watcher) buildHubDomainIngress(namespace string, portal *hubv1alpha1.APIPortal, apis []hubv1alpha1.API, traefikMiddlewareName string) (*netv1.Ingress, error) {
	name, err := getHubDomainIngressName(portal.Name)
	if err != nil {
		return nil, fmt.Errorf("get hub domain ingress name: %w", err)
	}

	return &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.k8s.io/v1",
			Kind:       "Ingress",
		},
		ObjectMeta: w.buildIngressObjectMeta(namespace, name, portal, w.config.TraefikTunnelEntryPoint, traefikMiddlewareName),
		Spec:       w.buildIngressSpec([]string{portal.Status.HubDomain}, apis, hubDomainSecretName),
	}, nil
}

func (w *Watcher) buildCustomDomainsIngress(namespace string, portal *hubv1alpha1.APIPortal, apis []hubv1alpha1.API, traefikMiddlewareName string) (*netv1.Ingress, error) {
	ingressName, err := getCustomDomainsIngressName(portal.Name)
	if err != nil {
		return nil, fmt.Errorf("get custom domains ingress name: %w", err)
	}

	secretName, err := getCustomDomainSecretName(portal.Name)
	if err != nil {
		return nil, fmt.Errorf("get custom domains secret name: %w", err)
	}

	return &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.k8s.io/v1",
			Kind:       "Ingress",
		},
		ObjectMeta: w.buildIngressObjectMeta(namespace, ingressName, portal, w.config.TraefikAPIEntryPoint, traefikMiddlewareName),
		Spec:       w.buildIngressSpec(portal.Status.CustomDomains, apis, secretName),
	}, nil
}

func (w *Watcher) buildIngressObjectMeta(namespace, name string, portal *hubv1alpha1.APIPortal, entrypoint, traefikMiddlewareName string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
		Annotations: map[string]string{
			"traefik.ingress.kubernetes.io/router.tls":         "true",
			"traefik.ingress.kubernetes.io/router.entrypoints": entrypoint,
			"traefik.ingress.kubernetes.io/router.middlewares": traefikMiddlewareName,
		},
		Labels: map[string]string{
			"app.kubernetes.io/managed-by": "traefik-hub",
		},
		// Set OwnerReference allow us to delete ingresses owned by an APIPortal.
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: portal.APIVersion,
				Kind:       portal.Kind,
				Name:       portal.Name,
				UID:        portal.UID,
			},
		},
	}
}

func (w *Watcher) buildIngressSpec(domains []string, apis []hubv1alpha1.API, tlsSecretName string) netv1.IngressSpec {
	pathType := netv1.PathTypePrefix

	var paths []netv1.HTTPIngressPath
	for _, api := range apis {
		paths = append(paths, netv1.HTTPIngressPath{
			PathType: &pathType,
			Path:     api.Spec.PathPrefix,
			Backend: netv1.IngressBackend{
				Service: &netv1.IngressServiceBackend{
					Name: api.Name,
					Port: netv1.ServiceBackendPort(api.Spec.Service.Port),
				},
			},
		})
	}

	var rules []netv1.IngressRule
	for _, domain := range domains {
		rules = append(rules, netv1.IngressRule{
			Host: domain,
			IngressRuleValue: netv1.IngressRuleValue{
				HTTP: &netv1.HTTPIngressRuleValue{
					Paths: paths,
				},
			},
		})
	}

	return netv1.IngressSpec{
		IngressClassName: pointer.String(w.config.IngressClassName),
		Rules:            rules,
		TLS: []netv1.IngressTLS{
			{
				Hosts:      domains,
				SecretName: tlsSecretName,
			},
		},
	}
}

// getHubDomainIngressName compute the ingress name for hub domain from the portal name.
// The name follow this format: {portal-name}-{hash(portal-name)}-hub
// This hash is here to reduce the chance of getting a collision on an existing ingress.
func getHubDomainIngressName(portalName string) (string, error) {
	h, err := hash(portalName)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d-hub", portalName, h), nil
}

// getCustomDomainsIngressName compute the ingress name for custom domains from the portal name.
// The name follow this format: {portal-name}-{hash(portal-name)}
// This hash is here to reduce the chance of getting a collision on an existing ingress.
func getCustomDomainsIngressName(portalName string) (string, error) {
	h, err := hash(portalName)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d", portalName, h), nil
}

// getEdgeIngressPortalName compute the edge ingress portal name from the portal name.
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

// getCustomDomainSecretName compute the name of the secret storing the certificate of the custom domains.
// The name follow this format: {customDomainSecretNamePrefix}-{hash(portal-name)}
// This hash is here to reduce the chance of getting a collision on an existing secret while staying under
// the limit of 63 characters.
func getCustomDomainSecretName(portalName string) (string, error) {
	h, err := hash(portalName)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d", customDomainSecretNamePrefix, h), nil
}

func newStripPrefixMiddleware(name, namespace string, apis []hubv1alpha1.API) traefikv1alpha1.Middleware {
	var prefixes []string
	for _, api := range apis {
		prefixes = append(prefixes, api.Spec.PathPrefix)
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i]) > len(prefixes[j])
	})

	return traefikv1alpha1.Middleware{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Middleware",
			APIVersion: "traefik.containo.us/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: traefikv1alpha1.MiddlewareSpec{
			StripPrefix: &traefikv1alpha1.StripPrefix{
				Prefixes:   prefixes,
				ForceSlash: false,
			},
		},
	}
}

// getStripPrefixMiddlewareName compute the name of the stripPrefix middleware.
// The name follow this format: {{portal-name}-hash({portal-name})-stripprefix}
// This hash is here to reduce the chance of getting a collision on an existing secret while staying under
// the limit of 63 characters.
func getStripPrefixMiddlewareName(portalName string) (string, error) {
	h, err := hash(portalName)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d-stripprefix", portalName, h), nil
}

func getTraefikStripPrefixMiddlewareName(namespace string, portal *hubv1alpha1.APIPortal) (string, error) {
	middlewareName, err := getStripPrefixMiddlewareName(portal.Name)
	if err != nil {
		return "", fmt.Errorf("get stripPrefix middleware name: %w", err)
	}
	return fmt.Sprintf("%s-%s@kubernetescrd", namespace, middlewareName), nil
}

func hash(name string) (uint32, error) {
	h := fnv.New32()

	if _, err := h.Write([]byte(name)); err != nil {
		return 0, fmt.Errorf("generate hash: %w", err)
	}

	return h.Sum32(), nil
}
