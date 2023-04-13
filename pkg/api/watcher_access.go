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

// WatcherAccess watches hub API accesses and sync them with the cluster.
type WatcherAccess struct {
	accessSyncInterval time.Duration

	platform PlatformClient

	kubeClientSet kclientset.Interface

	hubClientSet hubclientset.Interface
	hubInformer  hubinformers.SharedInformerFactory

	eventRecorder record.EventRecorder
}

// NewWatcherAccess returns a new WatcherAccess.
func NewWatcherAccess(client PlatformClient, kubeClientSet kclientset.Interface, hubClientSet hubclientset.Interface, hubInformer hubinformers.SharedInformerFactory, accessSyncInterval time.Duration) *WatcherAccess {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1.EventSinkImpl{Interface: kubeClientSet.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{})

	return &WatcherAccess{
		accessSyncInterval: accessSyncInterval,
		platform:           client,

		kubeClientSet: kubeClientSet,

		hubClientSet: hubClientSet,
		hubInformer:  hubInformer,

		eventRecorder: eventRecorder,
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

		logger := log.With().Str("name", platformAccess.Name).Logger()

		oldClusterAccess, found := clusterAccessesByName[platformAccess.Name]

		// Accesses that will remain in the map will be deleted.
		delete(clusterAccessesByName, platformAccess.Name)

		newClusterAccess, resourceErr := platformAccess.Resource()
		if resourceErr != nil {
			logger.Error().Err(resourceErr).Msg("Unable to build APIAccess resource")
			continue
		}

		if !found {
			if err = w.createAccess(ctx, newClusterAccess); err != nil {
				logger.Error().Err(err).Msg("Unable to create APIAccess")
			}
			continue
		}

		if err = w.updateAccess(ctx, oldClusterAccess, newClusterAccess); err != nil {
			logger.Error().Err(err).Msg("Unable to update APIAccess")
		}
	}

	w.cleanAccesses(ctx, clusterAccessesByName)
}

func (w *WatcherAccess) createAccess(ctx context.Context, access *hubv1alpha1.APIAccess) error {
	createdAccess, err := w.hubClientSet.HubV1alpha1().APIAccesses().Create(ctx, access, metav1.CreateOptions{})
	if err != nil {
		w.eventRecorder.Eventf(access, "Failed", "Syncing", "Unable to synchronize with the Hub platform: %s", err)
		return fmt.Errorf("creating APIAccess: %w", err)
	}

	log.Debug().
		Str("name", createdAccess.Name).
		Msg("APIAccess created")

	w.eventRecorder.Event(createdAccess, corev1.EventTypeNormal, "Synced", "Synced successfully with the Hub platform")

	return nil
}

func (w *WatcherAccess) updateAccess(ctx context.Context, oldAccess, newAccess *hubv1alpha1.APIAccess) error {
	meta := oldAccess.ObjectMeta
	meta.Labels = newAccess.Labels
	newAccess.ObjectMeta = meta

	if newAccess.Status.Version != oldAccess.Status.Version {
		updatedAccess, err := w.hubClientSet.HubV1alpha1().APIAccesses().Update(ctx, newAccess, metav1.UpdateOptions{})
		if err != nil {
			w.eventRecorder.Eventf(newAccess, "Failed", "Syncing", "Unable to synchronize with the Hub platform: %s", err)
			return fmt.Errorf("updating APIAccess: %w", err)
		}

		log.Debug().
			Str("name", updatedAccess.Name).
			Msg("APIAccess updated")

		w.eventRecorder.Event(updatedAccess, corev1.EventTypeNormal, "Synced", "Synced successfully with the Hub platform")
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
