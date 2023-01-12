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
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// PlatformClient for the Catalog service.
type PlatformClient interface {
	GetCatalogs(ctx context.Context) ([]Catalog, error)
}

// WatcherConfig holds the watcher configuration.
type WatcherConfig struct {
	CatalogSyncInterval  time.Duration
	AgentNamespace       string
	DevPortalServiceName string
	DevPortalPort        int
}

// Watcher watches hub Catalogs and sync them with the cluster.
type Watcher struct {
	config WatcherConfig

	client       PlatformClient
	hubClientSet hubclientset.Interface
	hubInformer  hubinformer.SharedInformerFactory
}

// NewWatcher returns a new Watcher.
func NewWatcher(client PlatformClient, hubClientSet hubclientset.Interface, hubInformer hubinformer.SharedInformerFactory, config WatcherConfig) *Watcher {
	return &Watcher{
		config: config,

		client:       client,
		hubClientSet: hubClientSet,
		hubInformer:  hubInformer,
	}
}

// Run runs Watcher.
func (w *Watcher) Run(ctx context.Context) {
	t := time.NewTicker(w.config.CatalogSyncInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping Catalog watcher")
			return

		case <-t.C:
			ctxSync, cancel := context.WithTimeout(ctx, 20*time.Second)
			w.syncCatalogs(ctxSync)
			cancel()
		}
	}
}

func (w *Watcher) syncCatalogs(ctx context.Context) {
	platformCatalogs, err := w.client.GetCatalogs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Unable to fetch Catalogs")
		return
	}

	clusterCatalogs, err := w.hubInformer.Hub().V1alpha1().Catalogs().Lister().List(labels.Everything())
	if err != nil {
		log.Error().Err(err).Msg("Unable to obtain Catalogs")
		return
	}

	catalogByName := map[string]*hubv1alpha1.Catalog{}
	for _, catalog := range clusterCatalogs {
		catalogByName[catalog.Name] = catalog
	}

	for _, catalog := range platformCatalogs {
		platformCatalog := catalog

		clusterCatalog, found := catalogByName[platformCatalog.Name]
		// We delete the catalog from the map, since we use this map to delete unused catalogs.
		delete(catalogByName, platformCatalog.Name)

		if !found {
			if err = w.createCatalog(ctx, &platformCatalog); err != nil {
				log.Error().Err(err).
					Str("name", platformCatalog.Name).
					Msg("Unable to create Catalog")
			}
			continue
		}

		if platformCatalog.Version == clusterCatalog.Status.Version {
			if err = w.syncChild(ctx, clusterCatalog); err != nil {
				log.Error().Err(err).
					Str("name", platformCatalog.Name).
					Msg("Unable to sync child resources")
			}

			log.Debug().
				Str("name", platformCatalog.Name).
				Msg("Same version found, skipping update catalog")

			continue
		}

		if err = w.updateCatalog(ctx, clusterCatalog, &platformCatalog); err != nil {
			log.Error().Err(err).
				Str("name", platformCatalog.Name).
				Msg("Unable to update Catalog")
		}
	}

	w.cleanCatalogs(ctx, catalogByName)
}

func (w *Watcher) createCatalog(ctx context.Context, catalog *Catalog) error {
	obj, err := catalog.Resource()
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

	return w.syncChild(ctx, obj)
}

func (w *Watcher) updateCatalog(ctx context.Context, oldCatalog *hubv1alpha1.Catalog, newCatalog *Catalog) error {
	obj, err := newCatalog.Resource()
	if err != nil {
		return fmt.Errorf("build Catalog resource: %w", err)
	}

	oldCatalog.Spec = obj.Spec
	oldCatalog.Status = obj.Status

	obj, err = w.hubClientSet.HubV1alpha1().Catalogs().Update(ctx, oldCatalog, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating Catalog: %w", err)
	}

	log.Debug().
		Str("name", obj.Name).
		Msg("Catalog updated")

	return nil
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

func (w *Watcher) syncChild(ctx context.Context, c *hubv1alpha1.Catalog) error {
	ingName, err := getEdgeIngressName(c.Name)
	if err != nil {
		return fmt.Errorf("get edgeIngress name: %w", err)
	}

	clusterIng, err := w.hubClientSet.HubV1alpha1().EdgeIngresses(w.config.AgentNamespace).Get(ctx, ingName, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get edge ingress: %w", err)
	}

	if kerror.IsNotFound(err) {
		ing := &hubv1alpha1.EdgeIngress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ingName,
				Namespace: w.config.AgentNamespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "traefik-hub",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "hub.traefik.io/v1alpha1",
						Kind:       "Catalog",
						Name:       c.Name,
						UID:        c.UID,
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

	c.Status.DevPortalDomain = clusterIng.Status.Domain
	if _, err := w.hubClientSet.HubV1alpha1().Catalogs().Update(ctx, c, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating Catalog: %w", err)
	}

	return nil
}

// getEdgeIngressName compute the ingress name from the catalog name. The name follow this format:
// {edgeIngress-name}-{hash(edgeIngress-name)}
// This hash is here to reduce the chance of getting a collision on an existing ingress.
func getEdgeIngressName(catalogName string) (string, error) {
	hash := fnv.New32()

	if _, err := hash.Write([]byte(catalogName)); err != nil {
		return "", fmt.Errorf("generate hash: %w", err)
	}

	return fmt.Sprintf("%s-%d", catalogName, hash.Sum32()), nil
}
