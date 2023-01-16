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
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/rs/zerolog/log"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
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

// PlatformClient for the Catalog service.
type PlatformClient interface {
	GetCatalogs(ctx context.Context) ([]Catalog, error)
}

// OASRegistry is a registry of OpenAPI Spec URLs.
type OASRegistry interface {
	GetURL(name, namespace string) string
	Updated() <-chan struct{}
}

// WatcherConfig holds the watcher configuration.
type WatcherConfig struct {
	CatalogSyncInterval      time.Duration
	AgentNamespace           string
	DevPortalServiceName     string
	DevPortalPort            int
	TraefikCatalogEntryPoint string
	IngressClassName         string
}

// Watcher watches hub Catalogs and sync them with the cluster.
type Watcher struct {
	config WatcherConfig

	platform    PlatformClient
	oasRegistry OASRegistry

	kubeClientSet clientset.Interface
	kubeInformer  informers.SharedInformerFactory

	hubClientSet hubclientset.Interface
	hubInformer  hubinformer.SharedInformerFactory
}

// NewWatcher returns a new Watcher.
func NewWatcher(client PlatformClient, oasRegistry OASRegistry, kubeClientSet clientset.Interface, kubeInformer informers.SharedInformerFactory, hubClientSet hubclientset.Interface, hubInformer hubinformer.SharedInformerFactory, config WatcherConfig) *Watcher {
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

	ctxSync, cancel := context.WithTimeout(ctx, 20*time.Second)
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
		}
	}
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
		if err := w.upsertIngress(ctx, namespace, catalog, services); err != nil {
			return fmt.Errorf("upsert catalog ingress for namespace %q: %w", namespace, err)
		}
	}

	return nil
}

func (w *Watcher) upsertIngress(ctx context.Context, namespace string, catalog *hubv1alpha1.Catalog, services []hubv1alpha1.CatalogService) error {
	name, err := getIngressName(catalog.Name)
	if err != nil {
		return fmt.Errorf("get ingress name: %w", err)
	}

	existingIngress, err := w.kubeClientSet.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get ingress: %w", err)
	}

	if kerror.IsNotFound(err) {
		newIngress := w.buildIngress(namespace, name, catalog, services)
		_, err = w.kubeClientSet.NetworkingV1().Ingresses(namespace).Create(ctx, newIngress, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create ingress: %w", err)
		}

		log.Debug().
			Str("name", newIngress.Name).
			Str("namespace", newIngress.Namespace).
			Msg("Ingress created")

		return nil
	}

	updatedIngress := w.buildIngress(namespace, name, catalog, services)
	existingIngress.Spec = updatedIngress.Spec
	// Override Annotations and Labels in case new values are added in the future.
	existingIngress.ObjectMeta.Annotations = updatedIngress.ObjectMeta.Annotations
	existingIngress.ObjectMeta.Labels = updatedIngress.ObjectMeta.Labels

	_, err = w.kubeClientSet.NetworkingV1().Ingresses(namespace).Update(ctx, updatedIngress, metav1.UpdateOptions{})
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

	catalogIngressName, err := getIngressName(catalog.Name)
	if err != nil {
		return fmt.Errorf("get ingress name: %w", err)
	}

	catalogNamespaces := make(map[string]struct{})
	for _, service := range catalog.Spec.Services {
		catalogNamespaces[service.Namespace] = struct{}{}
	}

	for _, ingress := range hubIngresses {
		if ingress.Name != catalogIngressName {
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
		}
	}

	return nil
}

func (w *Watcher) buildIngress(namespace, name string, catalog *hubv1alpha1.Catalog, services []hubv1alpha1.CatalogService) *netv1.Ingress {
	annotations := map[string]string{
		"traefik.ingress.kubernetes.io/router.entrypoints": w.config.TraefikCatalogEntryPoint,
	}

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
	for _, domain := range catalog.Status.Domains {
		rules = append(rules, netv1.IngressRule{
			Host: domain,
			IngressRuleValue: netv1.IngressRuleValue{
				HTTP: &netv1.HTTPIngressRuleValue{
					Paths: paths,
				},
			},
		})
	}

	return &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "networking.k8s.io/v1",
			APIVersion: "Ingress",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
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
		},
		Spec: netv1.IngressSpec{
			IngressClassName: pointer.StringPtr(w.config.IngressClassName),
			Rules:            rules,
		},
	}
}

// getIngressName compute the ingress name from the catalog name. The name follow this format:
// {catalog-name}-{hash(catalog-name)}
// This hash is here to reduce the chance of getting a collision on an existing ingress.
func getIngressName(catalogName string) (string, error) {
	hash := fnv.New32()

	if _, err := hash.Write([]byte(catalogName)); err != nil {
		return "", fmt.Errorf("generate hash: %w", err)
	}

	return fmt.Sprintf("%s-%d", catalogName, hash.Sum32()), nil
}

// getEdgeIngressPortalName compute the edge ingress portal name from the catalog name. The name follow this format:
// {catalog-name}-portal-{hash(catalog-name)}
// This hash is here to reduce the chance of getting a collision on an existing ingress.
func getEdgeIngressPortalName(catalogName string) (string, error) {
	hash := fnv.New32()

	if _, err := hash.Write([]byte(catalogName)); err != nil {
		return "", fmt.Errorf("generate hash: %w", err)
	}

	// EdgeIngresses generate Ingresses with the same name. Therefore, to prevent any conflicts between the portal
	// ingress and the catalog ingresses the term "-portal-" must be added in between.
	return fmt.Sprintf("%s-portal-%d", catalogName, hash.Sum32()), nil
}
