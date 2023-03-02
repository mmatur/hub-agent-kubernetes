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

// WatcherAPI watches hub APIs and sync them with the cluster.
type WatcherAPI struct {
	apiSyncInterval time.Duration

	platform PlatformClient

	hubClientSet hubclientset.Interface
	hubInformer  hubinformer.SharedInformerFactory
}

// NewWatcherAPI returns a new WatcherPortal API.
func NewWatcherAPI(client PlatformClient, hubClientSet hubclientset.Interface, hubInformer hubinformer.SharedInformerFactory, apiSyncInterval time.Duration) *WatcherAPI {
	return &WatcherAPI{
		apiSyncInterval: apiSyncInterval,
		platform:        client,

		hubClientSet: hubClientSet,
		hubInformer:  hubInformer,
	}
}

// Run runs WatcherPortal.
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

	clusterAPIsByName := map[string]*hubv1alpha1.API{}
	for _, api := range clusterAPIs {
		clusterAPIsByName[api.Name] = api
	}

	for _, api := range platformAPIs {
		platformAPI := api

		clusterAPI, found := clusterAPIsByName[platformAPI.Name]

		// APIs that will remain in the map will be deleted.
		delete(clusterAPIsByName, platformAPI.Name)

		if !found {
			if err = w.createAPI(ctx, &platformAPI); err != nil {
				log.Error().Err(err).
					Str("name", platformAPI.Name).
					Msg("Unable to create API")
			}
			continue
		}

		if err = w.updateAPI(ctx, clusterAPI, &platformAPI); err != nil {
			log.Error().Err(err).
				Str("name", platformAPI.Name).
				Msg("Unable to update API")
		}
	}

	w.cleanPortals(ctx, clusterAPIsByName)
}

func (w *WatcherAPI) createAPI(ctx context.Context, api *API) error {
	obj, err := api.Resource()
	if err != nil {
		return fmt.Errorf("build API resource: %w", err)
	}

	obj, err = w.hubClientSet.HubV1alpha1().APIs(api.Namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating API: %w", err)
	}

	log.Debug().
		Str("name", obj.Name).
		Msg("API created")

	return nil
}

func (w *WatcherAPI) updateAPI(ctx context.Context, oldAPI *hubv1alpha1.API, newAPI *API) error {
	obj, err := newAPI.Resource()
	if err != nil {
		return fmt.Errorf("build API resource: %w", err)
	}

	obj.ObjectMeta = oldAPI.ObjectMeta

	if obj.Status.Version != oldAPI.Status.Version {
		obj, err = w.hubClientSet.HubV1alpha1().APIs(obj.Namespace).Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating API: %w", err)
		}

		log.Debug().
			Str("name", obj.Name).
			Msg("API updated")
	}

	return nil
}

func (w *WatcherAPI) cleanPortals(ctx context.Context, apis map[string]*hubv1alpha1.API) {
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
			Msg("API deleted")
	}
}
