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
	"time"

	"github.com/rs/zerolog/log"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// WatcherCollection watches hub APICollections and sync them with the cluster.
type WatcherCollection struct {
	collectionSyncInterval time.Duration

	platform PlatformClient

	hubClientSet hubclientset.Interface
	hubInformer  hubinformer.SharedInformerFactory
}

// NewWatcherCollection returns a new WatcherCollection.
func NewWatcherCollection(client PlatformClient, hubClientSet hubclientset.Interface, hubInformer hubinformer.SharedInformerFactory, collectionSyncInterval time.Duration) *WatcherCollection {
	return &WatcherCollection{
		collectionSyncInterval: collectionSyncInterval,
		platform:               client,

		hubClientSet: hubClientSet,
		hubInformer:  hubInformer,
	}
}

// Run runs WatcherCollection.
func (w *WatcherCollection) Run(ctx context.Context) {
	t := time.NewTicker(w.collectionSyncInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping API collection watcher")
			return

		case <-t.C:
			ctxSync, cancel := context.WithTimeout(ctx, 20*time.Second)
			w.syncCollections(ctxSync)
			cancel()
		}
	}
}

func (w *WatcherCollection) syncCollections(ctx context.Context) {
	platformCollections, err := w.platform.GetCollections(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Unable to fetch APICollections")
		return
	}

	clusterCollections, err := w.hubInformer.Hub().V1alpha1().APICollections().Lister().List(labels.Everything())
	if err != nil {
		log.Error().Err(err).Msg("Unable to obtain APICollections")
		return
	}

	clusterCollectionsByName := map[string]*hubv1alpha1.APICollection{}
	for _, collection := range clusterCollections {
		clusterCollectionsByName[collection.Name] = collection
	}

	for _, collection := range platformCollections {
		platformCollection := collection

		clusterCollection, found := clusterCollectionsByName[platformCollection.Name]

		// Collections that will remain in the map will be deleted.
		delete(clusterCollectionsByName, platformCollection.Name)

		if !found {
			if err = w.createCollection(ctx, &platformCollection); err != nil {
				log.Error().Err(err).
					Str("name", platformCollection.Name).
					Msg("Unable to create APICollection")
			}
			continue
		}

		if err = w.updateCollection(ctx, clusterCollection, &platformCollection); err != nil {
			log.Error().Err(err).
				Str("name", platformCollection.Name).
				Msg("Unable to update APICollection")
		}
	}

	w.cleanCollections(ctx, clusterCollectionsByName)
}

func (w *WatcherCollection) createCollection(ctx context.Context, collection *Collection) error {
	obj, err := collection.Resource()
	if err != nil {
		return fmt.Errorf("build APICollection resource: %w", err)
	}

	obj, err = w.hubClientSet.HubV1alpha1().APICollections().Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating APICollection: %w", err)
	}

	log.Debug().
		Str("name", obj.Name).
		Msg("APICollection created")

	return nil
}

func (w *WatcherCollection) updateCollection(ctx context.Context, oldCollection *hubv1alpha1.APICollection, newCollection *Collection) error {
	obj, err := newCollection.Resource()
	if err != nil {
		return fmt.Errorf("build APICollection resource: %w", err)
	}

	obj.ObjectMeta = oldCollection.ObjectMeta
	obj.ObjectMeta.Labels = newCollection.Labels

	if obj.Status.Version != oldCollection.Status.Version {
		obj, err = w.hubClientSet.HubV1alpha1().APICollections().Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating APICollection: %w", err)
		}

		log.Debug().
			Str("name", obj.Name).
			Msg("APICollection updated")
	}

	return nil
}

func (w *WatcherCollection) cleanCollections(ctx context.Context, collections map[string]*hubv1alpha1.APICollection) {
	for _, collection := range collections {
		// Foreground propagation allow us to delete all resources owned by the Collection.
		policy := metav1.DeletePropagationForeground

		opts := metav1.DeleteOptions{
			PropagationPolicy: &policy,
		}
		err := w.hubClientSet.HubV1alpha1().APICollections().Delete(ctx, collection.Name, opts)
		if err != nil {
			log.Error().Err(err).Msg("Unable to delete APICollection")

			continue
		}

		log.Debug().
			Str("name", collection.Name).
			Msg("APICollection deleted")
	}
}
