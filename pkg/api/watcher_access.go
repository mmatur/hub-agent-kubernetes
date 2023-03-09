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

// WatcherAccess watches hub API accesses and sync them with the cluster.
type WatcherAccess struct {
	accessSyncInterval time.Duration

	platform PlatformClient

	hubClientSet hubclientset.Interface
	hubInformer  hubinformer.SharedInformerFactory
}

// NewWatcherAccess returns a new WatcherAccess.
func NewWatcherAccess(client PlatformClient, hubClientSet hubclientset.Interface, hubInformer hubinformer.SharedInformerFactory, accessSyncInterval time.Duration) *WatcherAccess {
	return &WatcherAccess{
		accessSyncInterval: accessSyncInterval,
		platform:           client,

		hubClientSet: hubClientSet,
		hubInformer:  hubInformer,
	}
}

// Run runs WatcherAccess.
func (w *WatcherAccess) Run(ctx context.Context) {
	t := time.NewTicker(w.accessSyncInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping API access watcher")
			return

		case <-t.C:
			ctxSync, cancel := context.WithTimeout(ctx, 20*time.Second)
			w.syncAccesses(ctxSync)
			cancel()
		}
	}
}

func (w *WatcherAccess) syncAccesses(ctx context.Context) {
	platformAccesses, err := w.platform.GetAccesses(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Unable to fetch APIAccesses")
		return
	}

	clusterAccesses, err := w.hubInformer.Hub().V1alpha1().APIAccesses().Lister().List(labels.Everything())
	if err != nil {
		log.Error().Err(err).Msg("Unable to obtain APIAccesses")
		return
	}

	clusterAccessesByName := map[string]*hubv1alpha1.APIAccess{}
	for _, access := range clusterAccesses {
		clusterAccessesByName[access.Name] = access
	}

	for _, access := range platformAccesses {
		platformAccess := access
		clusterAccess, found := clusterAccessesByName[platformAccess.Name]

		// Accesses that will remain in the map will be deleted.
		delete(clusterAccessesByName, platformAccess.Name)

		if !found {
			if err = w.createAccess(ctx, &platformAccess); err != nil {
				log.Error().Err(err).
					Str("name", platformAccess.Name).
					Msg("Unable to create APIAccess")
			}
			continue
		}

		if err = w.updateAccess(ctx, clusterAccess, &platformAccess); err != nil {
			log.Error().Err(err).
				Str("name", platformAccess.Name).
				Msg("Unable to update APIAccess")
		}
	}

	w.cleanAccesses(ctx, clusterAccessesByName)
}

func (w *WatcherAccess) createAccess(ctx context.Context, access *Access) error {
	obj, err := access.Resource()
	if err != nil {
		return fmt.Errorf("build APIAccess resource: %w", err)
	}

	obj, err = w.hubClientSet.HubV1alpha1().APIAccesses().Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating APIAccess: %w", err)
	}

	log.Debug().
		Str("name", obj.Name).
		Msg("APIAccess created")

	return nil
}

func (w *WatcherAccess) updateAccess(ctx context.Context, oldAccess *hubv1alpha1.APIAccess, newAccess *Access) error {
	obj, err := newAccess.Resource()
	if err != nil {
		return fmt.Errorf("build APIAccess resource: %w", err)
	}

	obj.ObjectMeta = oldAccess.ObjectMeta
	obj.ObjectMeta.Labels = newAccess.Labels

	if obj.Status.Version != oldAccess.Status.Version {
		obj, err = w.hubClientSet.HubV1alpha1().APIAccesses().Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating APIAccess: %w", err)
		}

		log.Debug().
			Str("name", obj.Name).
			Msg("APIAccess updated")
	}

	return nil
}

func (w *WatcherAccess) cleanAccesses(ctx context.Context, accesses map[string]*hubv1alpha1.APIAccess) {
	for _, access := range accesses {
		// Foreground propagation allow us to delete all resources owned by the APIAccess.
		policy := metav1.DeletePropagationForeground

		opts := metav1.DeleteOptions{
			PropagationPolicy: &policy,
		}
		err := w.hubClientSet.HubV1alpha1().APIAccesses().Delete(ctx, access.Name, opts)
		if err != nil {
			log.Error().Err(err).Msg("Unable to delete APIAccess")

			continue
		}

		log.Debug().
			Str("name", access.Name).
			Msg("APIAccess deleted")
	}
}
