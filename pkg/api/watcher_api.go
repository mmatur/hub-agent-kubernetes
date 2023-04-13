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

// WatcherAPI watches hub APIs and sync them with the cluster.
type WatcherAPI struct {
	apiSyncInterval time.Duration

	platform PlatformClient

	kubeClientSet kclientset.Interface

	hubClientSet hubclientset.Interface
	hubInformer  hubinformers.SharedInformerFactory

	eventRecorder record.EventRecorder
}

// NewWatcherAPI returns a new WatcherAPI.
func NewWatcherAPI(client PlatformClient, kubeClientSet kclientset.Interface, hubClientSet hubclientset.Interface, hubInformer hubinformers.SharedInformerFactory, apiSyncInterval time.Duration) *WatcherAPI {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1.EventSinkImpl{Interface: kubeClientSet.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{})

	return &WatcherAPI{
		apiSyncInterval: apiSyncInterval,
		platform:        client,

		kubeClientSet: kubeClientSet,

		hubClientSet: hubClientSet,
		hubInformer:  hubInformer,

		eventRecorder: eventRecorder,
	}
}

// Run runs WatcherAPI.
func (w *WatcherAPI) Run(ctx context.Context) {
	t := time.NewTicker(w.apiSyncInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping API watcher")
			return

		case <-t.C:
			ctxSync, cancel := context.WithTimeout(ctx, 20*time.Second)
			w.syncAPIs(ctxSync)
			cancel()
		}
	}
}

func (w *WatcherAPI) syncAPIs(ctx context.Context) {
	platformAPIs, err := w.platform.GetAPIs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Unable to fetch APIs")
		return
	}

	clusterAPIs, err := w.hubInformer.Hub().V1alpha1().APIs().Lister().List(labels.Everything())
	if err != nil {
		log.Error().Err(err).Msg("Unable to obtain APIs")
		return
	}

	clusterAPIsByNameNamespace := map[string]*hubv1alpha1.API{}
	for _, api := range clusterAPIs {
		clusterAPIsByNameNamespace[api.Name+"@"+api.Namespace] = api
	}

	for _, api := range platformAPIs {
		platformAPI := api

		logger := log.With().
			Str("name", platformAPI.Name).
			Str("namespace", platformAPI.Namespace).
			Logger()

		oldClusterAPI, found := clusterAPIsByNameNamespace[platformAPI.Name+"@"+platformAPI.Namespace]

		// APIs that will remain in the map will be deleted.
		delete(clusterAPIsByNameNamespace, platformAPI.Name+"@"+platformAPI.Namespace)

		newClusterAPI, resourceErr := platformAPI.Resource()
		if resourceErr != nil {
			logger.Error().Err(resourceErr).Msg("Unable to build API resource")
			continue
		}

		if !found {
			if err = w.createAPI(ctx, newClusterAPI); err != nil {
				logger.Error().Err(err).Msg("Unable to create API")
			}
			continue
		}

		if err = w.updateAPI(ctx, oldClusterAPI, newClusterAPI); err != nil {
			logger.Error().Err(err).Msg("Unable to update API")
		}
	}

	w.cleanAPIs(ctx, clusterAPIsByNameNamespace)
}

func (w *WatcherAPI) createAPI(ctx context.Context, api *hubv1alpha1.API) error {
	createdAPI, err := w.hubClientSet.HubV1alpha1().APIs(api.Namespace).Create(ctx, api, metav1.CreateOptions{})
	if err != nil {
		w.eventRecorder.Eventf(api, "Failed", "Syncing", "Unable to synchronize with the Hub platform: %s", err)
		return fmt.Errorf("creating API: %w", err)
	}

	log.Debug().
		Str("name", createdAPI.Name).
		Str("namespace", createdAPI.Namespace).
		Msg("API created")

	w.eventRecorder.Event(createdAPI, corev1.EventTypeNormal, "Synced", "Synced successfully with the Hub platform")

	return nil
}

func (w *WatcherAPI) updateAPI(ctx context.Context, oldAPI, newAPI *hubv1alpha1.API) error {
	meta := oldAPI.ObjectMeta
	meta.Labels = newAPI.Labels
	newAPI.ObjectMeta = meta

	if newAPI.Status.Version != oldAPI.Status.Version {
		updatedAPI, err := w.hubClientSet.HubV1alpha1().APIs(newAPI.Namespace).Update(ctx, newAPI, metav1.UpdateOptions{})
		if err != nil {
			w.eventRecorder.Eventf(newAPI, "Failed", "Syncing", "Unable to synchronize with the Hub platform: %s", err)
			return fmt.Errorf("updating API: %w", err)
		}

		log.Debug().
			Str("name", updatedAPI.Name).
			Str("namespace", updatedAPI.Namespace).
			Msg("API updated")

		w.eventRecorder.Event(updatedAPI, corev1.EventTypeNormal, "Synced", "Synced successfully with the Hub platform")
	}

	return nil
}

func (w *WatcherAPI) cleanAPIs(ctx context.Context, apis map[string]*hubv1alpha1.API) {
	for _, api := range apis {
		// Foreground propagation allow us to delete all resources owned by the API.
		policy := metav1.DeletePropagationForeground

		opts := metav1.DeleteOptions{
			PropagationPolicy: &policy,
		}
		err := w.hubClientSet.HubV1alpha1().APIs(api.Namespace).Delete(ctx, api.Name, opts)
		if err != nil {
			log.Error().Err(err).Msg("Unable to delete API")

			continue
		}

		log.Debug().
			Str("name", api.Name).
			Str("namespace", api.Namespace).
			Msg("API deleted")
	}
}
