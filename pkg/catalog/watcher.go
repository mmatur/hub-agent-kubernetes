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

package catalog

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
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

// PlatformClient for the Catalog service.
type PlatformClient interface {
	GetCatalogs(ctx context.Context) ([]Catalog, error)
	GetWildcardCertificate(ctx context.Context) (edgeingress.Certificate, error)
	GetCertificateByDomains(ctx context.Context, domains []string) (edgeingress.Certificate, error)
}

// OASRegistry is a registry of OpenAPI Spec URLs.
type OASRegistry interface {
	GetURL(name, namespace string) string
	Updated() <-chan struct{}
}

// WatcherConfig holds the watcher configuration.
type WatcherConfig struct {
	IngressClassName         string
	AgentNamespace           string
	TraefikCatalogEntryPoint string
	TraefikTunnelEntryPoint  string
	DevPortalServiceName     string
	DevPortalPort            int

	CatalogSyncInterval time.Duration
	CertSyncInterval    time.Duration
	CertRetryInterval   time.Duration
}

// Watcher watches hub Catalogs and sync them with the cluster.
type Watcher struct {
	config *WatcherConfig

	wildCardCertMu sync.RWMutex
	wildCardCert   edgeingress.Certificate

	platform    PlatformClient
	oasRegistry OASRegistry

	kubeClientSet clientset.Interface
	kubeInformer  informers.SharedInformerFactory

	hubClientSet hubclientset.Interface
	hubInformer  hubinformer.SharedInformerFactory
}

// NewWatcher returns a new Watcher.
func NewWatcher(client PlatformClient, oasRegistry OASRegistry, kubeClientSet clientset.Interface, kubeInformer informers.SharedInformerFactory, hubClientSet hubclientset.Interface, hubInformer hubinformer.SharedInformerFactory, config *WatcherConfig) *Watcher {
	return &Watcher{
		config: config,

		platform:    client,
		oasRegistry: oasRegistry,

		kubeClientSet: kubeClientSet,
		kubeInformer:  kubeInformer,

		hubClientSet: hubClientSet,
		hubInformer:  hubInformer,
	}
}

// Run runs Watcher.
func (w *Watcher) Run(ctx context.Context) {
	t := time.NewTicker(w.config.CatalogSyncInterval)
	defer t.Stop()

	certSyncInterval := time.After(w.config.CertSyncInterval)
	ctxSync, cancel := context.WithTimeout(ctx, 20*time.Second)
	if err := w.syncCertificates(ctxSync); err != nil {
		log.Error().Err(err).Msg("Unable to synchronize certificates with platform")
		certSyncInterval = time.After(w.config.CertRetryInterval)
	}
	w.syncCatalogs(ctxSync)
	cancel()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping Catalog watcher")
			return

		case <-t.C:
			ctxSync, cancel = context.WithTimeout(ctx, 20*time.Second)
			w.syncCatalogs(ctxSync)
			cancel()

		case <-w.oasRegistry.Updated():
			ctxSync, cancel = context.WithTimeout(ctx, 20*time.Second)
			w.syncCatalogs(ctxSync)
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
	certificate, err := w.platform.GetWildcardCertificate(ctx)
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

	if err = w.upsertSecret(ctx, certificate, hubDomainSecretName, w.config.AgentNamespace, nil); err != nil {
		return fmt.Errorf("upsert secret: %w", err)
	}

	w.wildCardCertMu.Lock()
	w.wildCardCert = certificate
	w.wildCardCertMu.Unlock()

	clusterCatalogs, err := w.hubInformer.Hub().V1alpha1().Catalogs().Lister().List(labels.Everything())
	if err != nil {
		return err
	}

	for _, catalog := range clusterCatalogs {
		err := w.setupCertificates(ctx, catalog, certificate)
		if err != nil {
			log.Error().Err(err).
				Str("name", catalog.Name).
				Str("namespace", catalog.Namespace).
				Msg("unable to setup catalog certificates")
		}
	}

	return nil
}

func (w *Watcher) setupCertificates(ctx context.Context, catalog *hubv1alpha1.Catalog, certificate edgeingress.Certificate) error {
	servicesNamespaces := make(map[string]struct{})
	for _, service := range catalog.Spec.Services {
		if _, found := servicesNamespaces[service.Namespace]; !found {
			if err := w.upsertSecret(ctx, certificate, hubDomainSecretName, service.Namespace, catalog); err != nil {
				return fmt.Errorf("upsert secret: %w", err)
			}
			servicesNamespaces[service.Namespace] = struct{}{}
		}
	}

	if len(catalog.Spec.CustomDomains) == 0 {
		return nil
	}

	cert, err := w.platform.GetCertificateByDomains(ctx, catalog.Spec.CustomDomains)
	if err != nil {
		return fmt.Errorf("get certificate by domains %q: %w", strings.Join(catalog.Spec.CustomDomains, ","), err)
	}

	secretName, err := getCustomDomainSecretName(catalog.Name)
	if err != nil {
		return fmt.Errorf("get custom domains secret name: %w", err)
	}

	for namespace := range servicesNamespaces {
		if err := w.upsertSecret(ctx, cert, secretName, namespace, catalog); err != nil {
			return fmt.Errorf("upsert secret: %w", err)
		}
	}

	return nil
}

func (w *Watcher) upsertSecret(ctx context.Context, cert edgeingress.Certificate, name, namespace string, c *hubv1alpha1.Catalog) error {
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
		if c != nil {
			secret.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: "hub.traefik.io/v1alpha1",
				Kind:       "Catalog",
				Name:       c.Name,
				UID:        c.UID,
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
	if c != nil {
		newOwners = appendOwnerReference(secret.OwnerReferences, metav1.OwnerReference{
			APIVersion: "hub.traefik.io/v1alpha1",
			Kind:       "Catalog",
			Name:       c.Name,
			UID:        c.UID,
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

func (w *Watcher) syncCatalogs(ctx context.Context) {
	platformCatalogs, err := w.platform.GetCatalogs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Unable to fetch Catalogs")
		return
	}

	clusterCatalogs, err := w.hubInformer.Hub().V1alpha1().Catalogs().Lister().List(labels.Everything())
	if err != nil {
		log.Error().Err(err).Msg("Unable to obtain Catalogs")
		return
	}

	catalogsByName := map[string]*hubv1alpha1.Catalog{}
	for _, catalog := range clusterCatalogs {
		catalogsByName[catalog.Name] = catalog
	}

	for _, catalog := range platformCatalogs {
		platformCatalog := catalog

		clusterCatalog, found := catalogsByName[platformCatalog.Name]

		// Catalogs that will remain in the map will be deleted.
		delete(catalogsByName, platformCatalog.Name)

		if !found {
			if err = w.createCatalog(ctx, &platformCatalog); err != nil {
				log.Error().Err(err).
					Str("name", platformCatalog.Name).
					Msg("Unable to create Catalog")
			}
			continue
		}

		if err = w.updateCatalog(ctx, clusterCatalog, &platformCatalog); err != nil {
			log.Error().Err(err).
				Str("name", platformCatalog.Name).
				Msg("Unable to update Catalog")
		}
	}

	w.cleanCatalogs(ctx, catalogsByName)
}

func (w *Watcher) createCatalog(ctx context.Context, catalog *Catalog) error {
	obj, err := catalog.Resource(w.oasRegistry)
	if err != nil {
		return fmt.Errorf("build Catalog resource: %w", err)
	}

	obj, err = w.hubClientSet.HubV1alpha1().Catalogs().Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating Catalog: %w", err)
	}

	log.Debug().
		Str("name", obj.Name).
		Msg("Catalog created")

	return w.syncChildResources(ctx, obj)
}

func (w *Watcher) updateCatalog(ctx context.Context, oldCatalog *hubv1alpha1.Catalog, newCatalog *Catalog) error {
	obj, err := newCatalog.Resource(w.oasRegistry)
	if err != nil {
		return fmt.Errorf("build Catalog resource: %w", err)
	}

	obj.ObjectMeta = oldCatalog.ObjectMeta

	if obj.Status.SpecHash != oldCatalog.Status.SpecHash {
		obj, err = w.hubClientSet.HubV1alpha1().Catalogs().Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating Catalog: %w", err)
		}

		log.Debug().
			Str("name", obj.Name).
			Msg("Catalog updated")
	}

	return w.syncChildResources(ctx, obj)
}

func (w *Watcher) cleanCatalogs(ctx context.Context, catalogs map[string]*hubv1alpha1.Catalog) {
	for _, catalog := range catalogs {
		// Foreground propagation allow us to delete all resources owned by the Catalog.
		policy := metav1.DeletePropagationForeground

		opts := metav1.DeleteOptions{
			PropagationPolicy: &policy,
		}
		err := w.hubClientSet.HubV1alpha1().Catalogs().Delete(ctx, catalog.Name, opts)
		if err != nil {
			log.Error().Err(err).Msg("Unable to delete Catalog")

			continue
		}

		log.Debug().
			Str("name", catalog.Name).
			Msg("Catalog deleted")
	}
}

func (w *Watcher) syncChildResources(ctx context.Context, catalog *hubv1alpha1.Catalog) error {
	w.wildCardCertMu.RLock()
	certificate := w.wildCardCert
	w.wildCardCertMu.RUnlock()

	if err := w.setupCertificates(ctx, catalog, certificate); err != nil {
		return fmt.Errorf("unable to setup catalog certificates: %w", err)
	}

	if err := w.cleanupIngresses(ctx, catalog); err != nil {
		return fmt.Errorf("clean up ingresses: %w", err)
	}

	if err := w.upsertIngresses(ctx, catalog); err != nil {
		return fmt.Errorf("upsert ingresses: %w", err)
	}

	if err := w.upsertPortalEdgeIngress(ctx, catalog); err != nil {
		return fmt.Errorf("upsert portal edge ingress: %w", err)
	}

	return nil
}

func (w *Watcher) upsertPortalEdgeIngress(ctx context.Context, catalog *hubv1alpha1.Catalog) error {
	ingName, err := getEdgeIngressPortalName(catalog.Name)
	if err != nil {
		return fmt.Errorf("get edgeIngress name: %w", err)
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
						APIVersion: catalog.APIVersion,
						Kind:       catalog.Kind,
						Name:       catalog.Name,
						UID:        catalog.UID,
					},
				},
			},
			Spec: hubv1alpha1.EdgeIngressSpec{
				Service: hubv1alpha1.EdgeIngressService{
					Name: w.config.DevPortalServiceName,
					Port: w.config.DevPortalPort,
				},
			},
		}

		clusterIng, err = w.hubClientSet.HubV1alpha1().EdgeIngresses(w.config.AgentNamespace).Create(ctx, ing, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create edge ingress: %w", err)
		}
	}

	// Set the Catalog DevPortalDomain with the domain obtained from the EdgeIngress.
	patch := []byte(fmt.Sprintf(`[
		{ "op": "replace", "path": "/status/devPortalDomain", "value": %q }
	]`, clusterIng.Status.Domain))

	if _, err = w.hubClientSet.HubV1alpha1().Catalogs().Patch(ctx, catalog.Name, ktypes.JSONPatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("patch Catalog: %w", err)
	}

	return nil
}

func (w *Watcher) upsertIngresses(ctx context.Context, catalog *hubv1alpha1.Catalog) error {
	servicesByNamespace := make(map[string][]hubv1alpha1.CatalogService)
	for _, service := range catalog.Spec.Services {
		namespace := service.Namespace
		if namespace == "" {
			namespace = "default"
		}

		servicesByNamespace[namespace] = append(servicesByNamespace[namespace], service)
	}

	for namespace, services := range servicesByNamespace {
		ingress, err := w.buildHubDomainIngress(namespace, catalog, services)
		if err != nil {
			return fmt.Errorf("build ingress for hub domain and namespace %q: %w", namespace, err)
		}

		if err = w.upsertIngress(ctx, ingress); err != nil {
			return fmt.Errorf("upsert ingress for hub domain and namespace %q: %w", namespace, err)
		}

		if len(catalog.Spec.CustomDomains) != 0 {
			ingress, err = w.buildCustomDomainsIngress(namespace, catalog, services)
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

// cleanupIngresses deletes the ingresses from namespaces that are no longer referenced in the catalog.
func (w *Watcher) cleanupIngresses(ctx context.Context, catalog *hubv1alpha1.Catalog) error {
	managedByHub, err := labels.NewRequirement("app.kubernetes.io/managed-by", selection.Equals, []string{"traefik-hub"})
	if err != nil {
		return fmt.Errorf("create managed by hub requirement: %w", err)
	}
	hubIngressesSelector := labels.NewSelector().Add(*managedByHub)
	hubIngresses, err := w.kubeInformer.Networking().V1().Ingresses().Lister().List(hubIngressesSelector)
	if err != nil {
		return fmt.Errorf("list ingresses: %w", err)
	}

	hubDomainIngressName, err := getHubDomainIngressName(catalog.Name)
	if err != nil {
		return fmt.Errorf("get ingress name for hub domain: %w", err)
	}
	customDomainsIngressName, err := getCustomDomainsIngressName(catalog.Name)
	if err != nil {
		return fmt.Errorf("get ingress name for custom domains: %w", err)
	}

	catalogNamespaces := make(map[string]struct{})
	for _, service := range catalog.Spec.Services {
		catalogNamespaces[service.Namespace] = struct{}{}
	}

	for _, ingress := range hubIngresses {
		if ingress.Name != hubDomainIngressName && ingress.Name != customDomainsIngressName {
			continue
		}

		if _, ok := catalogNamespaces[ingress.Namespace]; !ok {
			err = w.kubeClientSet.NetworkingV1().
				Ingresses(ingress.Namespace).
				Delete(ctx, ingress.Name, metav1.DeleteOptions{})

			if err != nil {
				log.Ctx(ctx).
					Error().
					Err(err).
					Str("catalog", catalog.Name).
					Str("ingress_name", ingress.Name).
					Str("ingress_namespace", ingress.Namespace).
					Msg("Unable to clean Catalog's child Ingress")

				continue
			}

			err = w.kubeClientSet.CoreV1().
				Secrets(ingress.Namespace).
				Delete(ctx, ingress.Spec.TLS[0].SecretName, metav1.DeleteOptions{})

			if err != nil {
				log.Ctx(ctx).
					Error().
					Err(err).
					Str("catalog", catalog.Name).
					Str("secret_name", ingress.Spec.TLS[0].SecretName).
					Str("secret_namespace", ingress.Namespace).
					Msg("Unable to clean Catalog's child Secret")
			}
		}
	}

	return nil
}

func (w *Watcher) buildHubDomainIngress(namespace string, catalog *hubv1alpha1.Catalog, services []hubv1alpha1.CatalogService) (*netv1.Ingress, error) {
	name, err := getHubDomainIngressName(catalog.Name)
	if err != nil {
		return nil, fmt.Errorf("get hub domain ingress name: %w", err)
	}

	return &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.k8s.io/v1",
			Kind:       "Ingress",
		},
		ObjectMeta: w.buildIngressObjectMeta(namespace, name, catalog, w.config.TraefikTunnelEntryPoint),
		Spec:       w.buildIngressSpec([]string{catalog.Status.Domain}, services, hubDomainSecretName),
	}, nil
}

func (w *Watcher) buildCustomDomainsIngress(namespace string, catalog *hubv1alpha1.Catalog, services []hubv1alpha1.CatalogService) (*netv1.Ingress, error) {
	ingressName, err := getCustomDomainsIngressName(catalog.Name)
	if err != nil {
		return nil, fmt.Errorf("get custom domains ingress name: %w", err)
	}

	secretName, err := getCustomDomainSecretName(catalog.Name)
	if err != nil {
		return nil, fmt.Errorf("get custom domains secret name: %w", err)
	}

	return &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.k8s.io/v1",
			Kind:       "Ingress",
		},
		ObjectMeta: w.buildIngressObjectMeta(namespace, ingressName, catalog, w.config.TraefikCatalogEntryPoint),
		Spec:       w.buildIngressSpec(catalog.Spec.CustomDomains, services, secretName),
	}, nil
}

func (w *Watcher) buildIngressObjectMeta(namespace, name string, catalog *hubv1alpha1.Catalog, entrypoint string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
		Annotations: map[string]string{
			"traefik.ingress.kubernetes.io/router.tls":         "true",
			"traefik.ingress.kubernetes.io/router.entrypoints": entrypoint,
		},
		Labels: map[string]string{
			"app.kubernetes.io/managed-by": "traefik-hub",
		},
		// Set OwnerReference allow us to delete ingresses owned by a catalog.
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: catalog.APIVersion,
				Kind:       catalog.Kind,
				Name:       catalog.Name,
				UID:        catalog.UID,
			},
		},
	}
}

func (w *Watcher) buildIngressSpec(domains []string, services []hubv1alpha1.CatalogService, tlsSecretName string) netv1.IngressSpec {
	pathType := netv1.PathTypePrefix

	var paths []netv1.HTTPIngressPath
	for _, service := range services {
		paths = append(paths, netv1.HTTPIngressPath{
			PathType: &pathType,
			Path:     service.PathPrefix,
			Backend: netv1.IngressBackend{
				Service: &netv1.IngressServiceBackend{
					Name: service.Name,
					Port: netv1.ServiceBackendPort{
						Number: int32(service.Port),
					},
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
		IngressClassName: pointer.StringPtr(w.config.IngressClassName),
		Rules:            rules,
		TLS: []netv1.IngressTLS{
			{
				Hosts:      domains,
				SecretName: tlsSecretName,
			},
		},
	}
}

// getHubDomainIngressName compute the ingress name for hub domain from the catalog name.
// The name follow this format: {catalog-name}-{hash(catalog-name)}-hub
// This hash is here to reduce the chance of getting a collision on an existing ingress.
func getHubDomainIngressName(catalogName string) (string, error) {
	h, err := hash(catalogName)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d-hub", catalogName, h), nil
}

// getCustomDomainsIngressName compute the ingress name for custom domains from the catalog name.
// The name follow this format: {catalog-name}-{hash(catalog-name)}
// This hash is here to reduce the chance of getting a collision on an existing ingress.
func getCustomDomainsIngressName(catalogName string) (string, error) {
	h, err := hash(catalogName)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d", catalogName, h), nil
}

// getEdgeIngressPortalName compute the edge ingress portal name from the catalog name.
// The name follow this format: {catalog-name}-{hash(catalog-name)}-portal
// This hash is here to reduce the chance of getting a collision on an existing ingress.
func getEdgeIngressPortalName(catalogName string) (string, error) {
	h, err := hash(catalogName)
	if err != nil {
		return "", err
	}

	// EdgeIngresses generate Ingresses with the same name. Therefore, to prevent any conflicts between the portal
	// ingress and the catalog ingresses the term "-portal" must be added as a suffix.
	return fmt.Sprintf("%s-%d-portal", catalogName, h), nil
}

// getCustomDomainSecretName compute the name of the secret storing the certificate of the custom domains.
// The name follow this format: {customDomainSecretNamePrefix}-{hash(catalog-name)}
// This hash is here to reduce the chance of getting a collision on an existing secret while staying under
// the limit of 63 characters.
func getCustomDomainSecretName(catalogName string) (string, error) {
	h, err := hash(catalogName)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d", customDomainSecretNamePrefix, h), nil
}

func hash(name string) (uint32, error) {
	h := fnv.New32()

	if _, err := h.Write([]byte(name)); err != nil {
		return 0, fmt.Errorf("generate hash: %w", err)
	}

	return h.Sum32(), nil
}
