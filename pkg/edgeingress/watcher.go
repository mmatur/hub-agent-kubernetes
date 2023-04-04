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

package edgeingress

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/admission/reviewer"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	traefikv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/traefik/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformers "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	"github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/typed/traefik/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kclientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
)

const (
	catchAllName            = "hub-catch-all"
	secretName              = "hub-certificate"
	secretCustomDomainsName = "hub-certificate-custom-domains"
)

// PlatformClient for the EdgeIngress service.
type PlatformClient interface {
	GetEdgeIngresses(ctx context.Context) ([]EdgeIngress, error)
	GetWildcardCertificate(ctx context.Context) (Certificate, error)
	GetCertificateByDomains(ctx context.Context, domains []string) (Certificate, error)
}

// WatcherConfig holds the watcher configuration.
type WatcherConfig struct {
	IngressClassName        string
	AgentNamespace          string
	TraefikTunnelEntryPoint string

	EdgeIngressSyncInterval time.Duration
	CertRetryInterval       time.Duration
	CertSyncInterval        time.Duration
}

// Watcher watches hub EdgeIngresses and sync them with the cluster.
type Watcher struct {
	config WatcherConfig

	wildCardCert   Certificate
	wildCardCertMu sync.RWMutex

	client           PlatformClient
	hubClientSet     hubclientset.Interface
	hubInformer      hubinformers.SharedInformerFactory
	clientSet        kclientset.Interface
	traefikClientSet v1alpha1.TraefikV1alpha1Interface
}

// NewWatcher returns a new Watcher.
func NewWatcher(client PlatformClient, hubClientSet hubclientset.Interface, clientSet kclientset.Interface, traefikClientSet v1alpha1.TraefikV1alpha1Interface, hubInformer hubinformers.SharedInformerFactory, config WatcherConfig) (*Watcher, error) {
	return &Watcher{
		config: config,

		client:           client,
		hubClientSet:     hubClientSet,
		hubInformer:      hubInformer,
		clientSet:        clientSet,
		traefikClientSet: traefikClientSet,
	}, nil
}

// Run runs Watcher.
func (w *Watcher) Run(ctx context.Context) {
	t := time.NewTicker(w.config.EdgeIngressSyncInterval)
	defer t.Stop()

	certSyncInterval := time.After(w.config.CertSyncInterval)
	ctxSync, cancel := context.WithTimeout(ctx, 20*time.Second)
	if err := w.syncCertificates(ctxSync); err != nil {
		log.Error().Err(err).Msg("Unable to synchronize certificates with platform")
		certSyncInterval = time.After(w.config.CertRetryInterval)
	}
	w.syncEdgeIngresses(ctxSync)
	cancel()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping EdgeIngress watcher")
			return

		case <-t.C:
			ctxSync, cancel = context.WithTimeout(ctx, 20*time.Second)
			w.syncEdgeIngresses(ctxSync)
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
	certificate, err := w.client.GetWildcardCertificate(ctx)
	if err != nil {
		return fmt.Errorf("get certificate: %w", err)
	}

	w.wildCardCertMu.RLock()
	if bytes.Equal(certificate.Certificate, w.wildCardCert.Certificate) &&
		bytes.Equal(certificate.PrivateKey, w.wildCardCert.PrivateKey) {
		w.wildCardCertMu.RUnlock()

		return nil
	}
	w.wildCardCertMu.RUnlock()

	if err = w.upsertSecret(ctx, certificate, secretName, w.config.AgentNamespace, nil); err != nil {
		return fmt.Errorf("upsert secret: %w", err)
	}

	w.wildCardCertMu.Lock()
	w.wildCardCert = certificate
	w.wildCardCertMu.Unlock()

	clusterEdgeIngresses, err := w.hubInformer.Hub().V1alpha1().EdgeIngresses().Lister().List(labels.Everything())
	if err != nil {
		return err
	}

	for _, edgeIngress := range clusterEdgeIngresses {
		err := w.setupCertificates(ctx, edgeIngress, certificate, edgeIngress.Status.CustomDomains)
		if err != nil {
			log.Error().Err(err).
				Str("name", edgeIngress.Name).
				Str("namespace", edgeIngress.Namespace).
				Msg("unable to setup edge ingress certificates")
		}
	}

	return w.createIngressCatchAll(ctx)
}

func (w *Watcher) syncEdgeIngresses(ctx context.Context) {
	platformEdgeIngresses, err := w.client.GetEdgeIngresses(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Unable to fetch EdgeIngresses")
		return
	}

	clusterEdgeIngresses, err := w.hubInformer.Hub().V1alpha1().EdgeIngresses().Lister().List(labels.Everything())
	if err != nil {
		log.Error().Err(err).Msg("Unable to obtain EdgeIngresses")
		return
	}

	clusterEdgeIngressByID := map[string]*hubv1alpha1.EdgeIngress{}
	for _, edgeIng := range clusterEdgeIngresses {
		clusterEdgeIngressByID[edgeIng.Name+"@"+edgeIng.Namespace] = edgeIng
	}

	for _, p := range platformEdgeIngresses {
		platformEdgeIng := p

		clusterEdgeIng, found := clusterEdgeIngressByID[platformEdgeIng.Name+"@"+platformEdgeIng.Namespace]
		// We delete the edge ingress from the map, since we use this map to delete unused edge ingresses.
		delete(clusterEdgeIngressByID, platformEdgeIng.Name+"@"+platformEdgeIng.Namespace)

		if !found {
			if err := w.createEdgeIngress(ctx, &platformEdgeIng); err != nil {
				log.Error().Err(err).
					Str("name", platformEdgeIng.Name).
					Str("namespace", platformEdgeIng.Namespace).
					Msg("Unable to create EdgeIngress")
			}
			continue
		}

		if platformEdgeIng.Version == clusterEdgeIng.Status.Version {
			if clusterEdgeIng.Status.Connection == hubv1alpha1.EdgeIngressConnectionUp {
				continue
			}
			if err := w.syncChildAndUpdateConnectionStatus(ctx, clusterEdgeIng, platformEdgeIng.CustomDomains); err != nil {
				log.Error().Err(err).
					Str("name", platformEdgeIng.Name).
					Str("namespace", platformEdgeIng.Namespace).
					Msg("Unable to sync child resources")
			}

			continue
		}

		if err := w.updateEdgeIngress(ctx, clusterEdgeIng, &platformEdgeIng); err != nil {
			log.Error().Err(err).
				Str("name", clusterEdgeIng.Name).
				Str("namespace", clusterEdgeIng.Namespace).
				Msg("Unable to update EdgeIngress")
		}
	}

	w.cleanEdgeIngresses(ctx, clusterEdgeIngressByID)
}

func (w *Watcher) syncChildAndUpdateConnectionStatus(ctx context.Context, edgeIngress *hubv1alpha1.EdgeIngress, customDomains []CustomDomain) error {
	var customDomainsName []string
	for _, customDomain := range customDomains {
		if customDomain.Verified {
			customDomainsName = append(customDomainsName, customDomain.Name)
		}
	}

	w.wildCardCertMu.RLock()
	certificate := w.wildCardCert
	w.wildCardCertMu.RUnlock()

	if err := w.setupCertificates(ctx, edgeIngress, certificate, customDomainsName); err != nil {
		return fmt.Errorf("unable to setup secrets: %w", err)
	}

	if err := w.upsertIngress(ctx, edgeIngress, customDomainsName); err != nil {
		return fmt.Errorf("upsert ingress: %w", err)
	}

	if err := w.setEdgeIngressConnectionStatusUP(ctx, edgeIngress); err != nil {
		return fmt.Errorf("update edge ingress status: %w", err)
	}

	return nil
}

func (w *Watcher) setupCertificates(ctx context.Context, edgeIngress *hubv1alpha1.EdgeIngress, certificate Certificate, customDomainsName []string) error {
	if err := w.upsertSecret(ctx, certificate, secretName, edgeIngress.Namespace, edgeIngress); err != nil {
		return fmt.Errorf("upsert secret: %w", err)
	}

	if len(customDomainsName) == 0 {
		return nil
	}

	cert, err := w.client.GetCertificateByDomains(ctx, customDomainsName)
	if err != nil {
		return fmt.Errorf("get certificate by domains %q: %w", strings.Join(customDomainsName, ","), err)
	}

	if err := w.upsertSecret(ctx, cert, secretCustomDomainsName+"-"+edgeIngress.Name, edgeIngress.Namespace, edgeIngress); err != nil {
		return fmt.Errorf("upsert secret: %w", err)
	}

	return nil
}

func (w *Watcher) upsertIngress(ctx context.Context, edgeIng *hubv1alpha1.EdgeIngress, customDomains []string) error {
	ing, err := w.clientSet.NetworkingV1().Ingresses(edgeIng.Namespace).Get(ctx, edgeIng.Name, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get ingress: %w", err)
	}

	if kerror.IsNotFound(err) {
		ing = buildIngress(edgeIng, &netv1.Ingress{}, w.config.IngressClassName, w.config.TraefikTunnelEntryPoint, customDomains)
		_, err = w.clientSet.NetworkingV1().Ingresses(edgeIng.Namespace).Create(ctx, ing, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create ingress: %w", err)
		}

		log.Debug().
			Str("name", ing.Name).
			Str("namespace", ing.Namespace).
			Msg("Ingress created")

		return nil
	}

	ing = buildIngress(edgeIng, ing, w.config.IngressClassName, w.config.TraefikTunnelEntryPoint, customDomains)
	_, err = w.clientSet.NetworkingV1().Ingresses(edgeIng.Namespace).Update(ctx, ing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update ingress: %w", err)
	}

	log.Debug().
		Str("name", ing.Name).
		Str("namespace", ing.Namespace).
		Msg("Ingress updated")

	return nil
}

func (w *Watcher) createIngressCatchAll(ctx context.Context) error {
	if w.traefikClientSet == nil {
		return nil
	}

	stripPrefix := &traefikv1alpha1.Middleware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "strip-prefix-catch-all",
			Namespace: w.config.AgentNamespace,
		},
		Spec: traefikv1alpha1.MiddlewareSpec{
			StripPrefixRegex: &traefikv1alpha1.StripPrefixRegex{Regex: []string{".*"}},
		},
	}

	addPrefix := &traefikv1alpha1.Middleware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "add-prefix-catch-all",
			Namespace: w.config.AgentNamespace,
		},
		Spec: traefikv1alpha1.MiddlewareSpec{
			AddPrefix: &traefikv1alpha1.AddPrefix{Prefix: "/edge-ingresses/in-progress"},
		},
	}

	_, err := w.traefikClientSet.Middlewares(w.config.AgentNamespace).Create(ctx, stripPrefix, metav1.CreateOptions{})
	if err != nil && !kerror.IsAlreadyExists(err) {
		return err
	}

	_, err = w.traefikClientSet.Middlewares(w.config.AgentNamespace).Create(ctx, addPrefix, metav1.CreateOptions{})
	if err != nil && !kerror.IsAlreadyExists(err) {
		return err
	}

	middlewares := fmt.Sprintf("%s-strip-prefix-catch-all@kubernetescrd,%s-add-prefix-catch-all@kubernetescrd",
		w.config.AgentNamespace,
		w.config.AgentNamespace)

	pathType := netv1.PathTypePrefix
	ing := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      catchAllName,
			Namespace: w.config.AgentNamespace,
			Annotations: map[string]string{
				"traefik.ingress.kubernetes.io/router.tls":         "true",
				"traefik.ingress.kubernetes.io/router.entrypoints": w.config.TraefikTunnelEntryPoint,
				"traefik.ingress.kubernetes.io/router.middlewares": middlewares,
				"traefik.ingress.kubernetes.io/router.priority":    "1",
			},
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "traefik-hub",
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: pointer.String(w.config.IngressClassName),
			TLS:              []netv1.IngressTLS{{SecretName: secretName}},
			Rules: []netv1.IngressRule{
				{
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: catchAllName,
											Port: netv1.ServiceBackendPort{
												Number: 443,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = w.clientSet.NetworkingV1().Ingresses(w.config.AgentNamespace).Create(ctx, ing, metav1.CreateOptions{})
	if err != nil && !kerror.IsAlreadyExists(err) {
		return fmt.Errorf("create ingress: %w", err)
	}

	return nil
}

func (w *Watcher) upsertSecret(ctx context.Context, cert Certificate, name, namespace string, edgeIngress *hubv1alpha1.EdgeIngress) error {
	secret, err := w.clientSet.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get secret: %w", err)
	}

	if kerror.IsNotFound(err) {
		secret = &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
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
		if edgeIngress != nil {
			secret.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: "hub.traefik.io/v1alpha1",
				Kind:       "EdgeIngress",
				Name:       edgeIngress.Name,
				UID:        edgeIngress.UID,
			}}
		}

		_, err = w.clientSet.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
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
	if edgeIngress != nil {
		newOwners = appendOwnerReference(secret.OwnerReferences, metav1.OwnerReference{
			APIVersion: "hub.traefik.io/v1alpha1",
			Kind:       "EdgeIngress",
			Name:       edgeIngress.Name,
			UID:        edgeIngress.UID,
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

	_, err = w.clientSet.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
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

func (w *Watcher) setEdgeIngressConnectionStatusUP(ctx context.Context, edgeIngress *hubv1alpha1.EdgeIngress) error {
	edgeIngress.Status.Connection = hubv1alpha1.EdgeIngressConnectionUp

	ctxUpdate, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := w.hubClientSet.HubV1alpha1().EdgeIngresses(edgeIngress.Namespace).Update(ctxUpdate, edgeIngress, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating EdgeIngress: %w", err)
	}

	log.Debug().
		Str("name", edgeIngress.Name).
		Str("namespace", edgeIngress.Namespace).
		Msg("EdgeIngress connection set status up")

	return nil
}

func (w *Watcher) createEdgeIngress(ctx context.Context, edgeIng *EdgeIngress) error {
	obj, err := edgeIng.Resource()
	if err != nil {
		return fmt.Errorf("build EdgeIngress resource: %w", err)
	}

	obj, err = w.hubClientSet.HubV1alpha1().EdgeIngresses(obj.Namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating EdgeIngress: %w", err)
	}

	log.Debug().
		Str("name", obj.Name).
		Str("namespace", obj.Namespace).
		Msg("EdgeIngress created")

	return w.syncChildAndUpdateConnectionStatus(ctx, obj, edgeIng.CustomDomains)
}

func (w *Watcher) updateEdgeIngress(ctx context.Context, oldEdgeIng *hubv1alpha1.EdgeIngress, newEdgeIng *EdgeIngress) error {
	obj, err := newEdgeIng.Resource()
	if err != nil {
		return fmt.Errorf("build EdgeIngress resource: %w", err)
	}

	oldEdgeIng.Spec = obj.Spec
	oldEdgeIng.Status = obj.Status

	obj, err = w.hubClientSet.HubV1alpha1().EdgeIngresses(obj.Namespace).Update(ctx, oldEdgeIng, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating EdgeIngress: %w", err)
	}

	log.Debug().
		Str("name", obj.Name).
		Str("namespace", obj.Namespace).
		Msg("EdgeIngress updated")

	return w.syncChildAndUpdateConnectionStatus(ctx, obj, newEdgeIng.CustomDomains)
}

func (w *Watcher) cleanEdgeIngresses(ctx context.Context, edgeIngs map[string]*hubv1alpha1.EdgeIngress) {
	for _, edgeIng := range edgeIngs {
		// Foreground propagation allow us to delete all ingresses owned by the edgeIngress.
		policy := metav1.DeletePropagationForeground

		opts := metav1.DeleteOptions{
			PropagationPolicy: &policy,
		}
		err := w.hubClientSet.HubV1alpha1().EdgeIngresses(edgeIng.Namespace).Delete(ctx, edgeIng.Name, opts)
		if err != nil {
			log.Error().Err(err).Msg("Unable to delete EdgeIngress")

			continue
		}

		log.Debug().
			Str("name", edgeIng.Name).
			Str("namespace", edgeIng.Namespace).
			Msg("EdgeIngress deleted")
	}
}

func buildIngress(edgeIng *hubv1alpha1.EdgeIngress, ing *netv1.Ingress, ingressClassName, entryPoint string, customDomains []string) *netv1.Ingress {
	annotations := map[string]string{
		"traefik.ingress.kubernetes.io/router.tls":         "true",
		"traefik.ingress.kubernetes.io/router.entrypoints": entryPoint,
	}
	if edgeIng.Spec.ACP != nil && edgeIng.Spec.ACP.Name != "" {
		annotations[reviewer.AnnotationHubAuth] = edgeIng.Spec.ACP.Name
	}

	ing.ObjectMeta = metav1.ObjectMeta{
		Name:        edgeIng.Name,
		Namespace:   edgeIng.Namespace,
		Annotations: annotations,
		Labels: map[string]string{
			"app.kubernetes.io/managed-by": "traefik-hub",
		},
		// Set OwnerReference allow us to delete ingresses owned by an edgeIngress.
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: "hub.traefik.io/v1alpha1",
				Kind:       "EdgeIngress",
				Name:       edgeIng.Name,
				UID:        edgeIng.UID,
			},
		},
	}

	// No secret is needed for TLS because we will use the wildcard certificate configured in the catch-all ingress.
	pathType := netv1.PathTypePrefix
	IngressRule := netv1.IngressRuleValue{
		HTTP: &netv1.HTTPIngressRuleValue{
			Paths: []netv1.HTTPIngressPath{
				{
					Path:     "/",
					PathType: &pathType,
					Backend: netv1.IngressBackend{
						Service: &netv1.IngressServiceBackend{
							Name: edgeIng.Spec.Service.Name,
							Port: netv1.ServiceBackendPort{
								Number: int32(edgeIng.Spec.Service.Port),
							},
						},
					},
				},
			},
		},
	}
	ing.Spec = netv1.IngressSpec{
		IngressClassName: pointer.String(ingressClassName),
		TLS: []netv1.IngressTLS{
			{
				SecretName: secretName,
				Hosts:      []string{edgeIng.Status.Domain},
			},
		},
		Rules: []netv1.IngressRule{
			{
				Host:             edgeIng.Status.Domain,
				IngressRuleValue: IngressRule,
			},
		},
	}

	if len(customDomains) == 0 {
		return ing
	}

	ing.Spec.TLS = append(ing.Spec.TLS, netv1.IngressTLS{
		SecretName: secretCustomDomainsName + "-" + ing.Name,
		Hosts:      customDomains,
	})

	for _, customDomain := range customDomains {
		ing.Spec.Rules = append(ing.Spec.Rules, netv1.IngressRule{
			Host:             customDomain,
			IngressRuleValue: IngressRule,
		})
	}

	return ing
}
