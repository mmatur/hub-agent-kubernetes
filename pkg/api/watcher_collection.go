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
	"github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/scheme"
	hubinformers "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kclientset "k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
)

// WatcherCollection watches hub APICollections and sync them with the cluster.
type WatcherCollection struct {
	collectionSyncInterval time.Duration

	platform PlatformClient

	kubeClientSet kclientset.Interface

	hubClientSet hubclientset.Interface
	hubInformer  hubinformers.SharedInformerFactory

	eventRecorder record.EventRecorder
}

// NewWatcherCollection returns a new WatcherCollection.
func NewWatcherCollection(client PlatformClient, kubeClientSet kclientset.Interface, hubClientSet hubclientset.Interface, hubInformer hubinformers.SharedInformerFactory, collectionSyncInterval time.Duration) *WatcherCollection {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1.EventSinkImpl{Interface: kubeClientSet.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{})

	return &WatcherCollection{
		collectionSyncInterval: collectionSyncInterval,
		platform:               client,

		kubeClientSet: kubeClientSet,

		hubClientSet: hubClientSet,
		hubInformer:  hubInformer,

		eventRecorder: eventRecorder,
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

		logger := log.With().Str("name", platformCollection.Name).Logger()

		oldClusterCollection, found := clusterCollectionsByName[platformCollection.Name]

		// Collections that will remain in the map will be deleted.
		delete(clusterCollectionsByName, platformCollection.Name)

		newClusterCollection, resourceErr := platformCollection.Resource()
		if resourceErr != nil {
			logger.Error().Err(resourceErr).Msg("Unable to build APICollection resource")
			continue
		}

		if !found {
			if err = w.createCollection(ctx, newClusterCollection); err != nil {
				logger.Error().Err(err).Msg("Unable to create APICollection")
			}
			continue
		}

		if err = w.updateCollection(ctx, oldClusterCollection, newClusterCollection); err != nil {
			logger.Error().Err(err).Msg("Unable to update APICollection")
		}
	}

	w.cleanCollections(ctx, clusterCollectionsByName)
}

func (w *WatcherCollection) createCollection(ctx context.Context, collection *hubv1alpha1.APICollection) error {
	createdCollection, err := w.hubClientSet.HubV1alpha1().APICollections().Create(ctx, collection, metav1.CreateOptions{})
	if err != nil {
		w.eventRecorder.Eventf(collection, "Failed", "Syncing", "Unable to synchronize with the Hub platform: %s", err)
		return fmt.Errorf("creating APICollection: %w", err)
	}

	log.Debug().
		Str("name", createdCollection.Name).
		Msg("APICollection created")

	w.eventRecorder.Event(createdCollection, corev1.EventTypeNormal, "Synced", "Synced successfully with the Hub platform")

	return nil
}

func (w *WatcherCollection) updateCollection(ctx context.Context, oldCollection, newCollection *hubv1alpha1.APICollection) error {
	meta := oldCollection.ObjectMeta
	meta.Labels = newCollection.Labels
	newCollection.ObjectMeta = meta

	if newCollection.Status.Version != oldCollection.Status.Version {
		updatedCollection, err := w.hubClientSet.HubV1alpha1().APICollections().Update(ctx, newCollection, metav1.UpdateOptions{})
		if err != nil {
			w.eventRecorder.Eventf(newCollection, "Failed", "Syncing", "Unable to synchronize with the Hub platform: %s", err)
			return fmt.Errorf("updating APICollection: %w", err)
		}

		log.Debug().
			Str("name", updatedCollection.Name).
			Msg("APICollection updated")

		w.eventRecorder.Event(updatedCollection, corev1.EventTypeNormal, "Synced", "Synced successfully with the Hub platform")
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
