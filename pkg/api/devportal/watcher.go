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

package devportal

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/listers/hub/v1alpha1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type portal struct {
	hubv1alpha1.APIPortal

	Gateway gateway
}

type gateway struct {
	hubv1alpha1.APIGateway

	Collections map[string]collection
	APIs        map[string]hubv1alpha1.API
}

type collection struct {
	hubv1alpha1.APICollection

	APIs map[string]hubv1alpha1.API
}

// UpdatableHandler is an updatable HTTP handler for serving dev portals.
type UpdatableHandler interface {
	Update(portals []portal) error
}

// Watcher watches APIPortals resources and builds configurations out of them.
type Watcher struct {
	portals     v1alpha1.APIPortalLister
	gateways    v1alpha1.APIGatewayLister
	apis        v1alpha1.APILister
	collections v1alpha1.APICollectionLister
	accesses    v1alpha1.APIAccessLister

	refresh          chan struct{}
	debounceDelay    time.Duration
	maxDebounceDelay time.Duration

	handler UpdatableHandler
}

// NewWatcher returns a new watcher to track API management resources. It calls the given UpdatableHandler when
// a resource is modified.
func NewWatcher(handler UpdatableHandler,
	portals v1alpha1.APIPortalLister,
	gateways v1alpha1.APIGatewayLister,
	apis v1alpha1.APILister,
	collections v1alpha1.APICollectionLister,
	accesses v1alpha1.APIAccessLister,
) *Watcher {
	return &Watcher{
		portals:     portals,
		gateways:    gateways,
		apis:        apis,
		collections: collections,
		accesses:    accesses,

		refresh:          make(chan struct{}, 1),
		debounceDelay:    2 * time.Second,
		maxDebounceDelay: 10 * time.Second,

		handler: handler,
	}
}

// Run starts listening for changes on the cluster.
func (w *Watcher) Run(ctx context.Context) {
	refresh := debounce(ctx, w.refresh, w.debounceDelay, w.maxDebounceDelay)

	for {
		select {
		case <-refresh:
			portals, err := w.getPortals()
			if err != nil {
				log.Error().Err(err).Msg("Unable to get portals")
				continue
			}

			if err = w.handler.Update(portals); err != nil {
				log.Error().Err(err).Msg("Unable to update handler")
				continue
			}
		case <-ctx.Done():
			return
		}
	}
}

// debounce listen for events on the source chan and emit an event on the debounced channel after waiting for
// the given `delay` duration. Each additional event will wait an additional `delay` duration until it reach
// the `maxDelay`.
func debounce(ctx context.Context, sourceCh <-chan struct{}, delay, maxDelay time.Duration) <-chan struct{} {
	debouncedCh := make(chan struct{})
	var (
		delayCh    <-chan time.Time
		maxDelayCh <-chan time.Time
	)

	go func() {
		for {
			select {
			case <-sourceCh:
				delayCh = time.After(delay)
				if maxDelayCh == nil {
					maxDelayCh = time.After(maxDelay)
				}

			case <-delayCh:
				delayCh, maxDelayCh = nil, nil
				debouncedCh <- struct{}{}
			case <-maxDelayCh:
				delayCh, maxDelayCh = nil, nil
				debouncedCh <- struct{}{}

			case <-ctx.Done():
				return
			}
		}
	}()

	return debouncedCh
}

// OnAdd implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *Watcher) OnAdd(obj interface{}) {
	switch obj.(type) {
	case *hubv1alpha1.APIPortal:
	case *hubv1alpha1.APIGateway:
	case *hubv1alpha1.API:
	case *hubv1alpha1.APICollection:
	case *hubv1alpha1.APIAccess:

	default:
		log.Error().
			Str("component", "api_portal_watcher").
			Str("type", fmt.Sprintf("%T", obj)).
			Msg("Received add event of unknown type")
		return
	}

	select {
	case w.refresh <- struct{}{}:
	default:
	}
}

// OnUpdate implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *Watcher) OnUpdate(oldObj, newObj interface{}) {
	logger := log.With().
		Str("component", "api_portal_watcher").
		Str("type", fmt.Sprintf("%T", newObj)).
		Logger()

	switch v := newObj.(type) {
	case *hubv1alpha1.APIPortal:
		if oldObj.(*hubv1alpha1.APIPortal).Status.Hash == v.Status.Hash &&
			oldObj.(*hubv1alpha1.APIPortal).Status.URLs == v.Status.URLs {
			logger.Debug().Msg("No change detected on APIPortal, skipping")
			return
		}
	case *hubv1alpha1.APIGateway:
		if oldObj.(*hubv1alpha1.APIGateway).Status.Hash == v.Status.Hash &&
			oldObj.(*hubv1alpha1.APIGateway).Status.URLs == v.Status.URLs {
			logger.Debug().Msg("No change detected on APIGateway, skipping")
			return
		}
	case *hubv1alpha1.API:
		if oldObj.(*hubv1alpha1.API).Status.Hash == v.Status.Hash {
			logger.Debug().Msg("No change detected on API, skipping")
			return
		}
	case *hubv1alpha1.APICollection:
		if oldObj.(*hubv1alpha1.APICollection).Status.Hash == v.Status.Hash {
			logger.Debug().Msg("No change detected on APICollection, skipping")
			return
		}
	case *hubv1alpha1.APIAccess:
		if oldObj.(*hubv1alpha1.APIAccess).Status.Hash == v.Status.Hash {
			logger.Debug().Msg("No change detected on APIAccess, skipping")
			return
		}
	default:
		logger.Error().Msg("Received update event of unknown type")
		return
	}

	select {
	case w.refresh <- struct{}{}:
	default:
	}
}

// OnDelete implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *Watcher) OnDelete(oldObj interface{}) {
	switch oldObj.(type) {
	case *hubv1alpha1.APIPortal:
	case *hubv1alpha1.APIGateway:
	case *hubv1alpha1.API:
	case *hubv1alpha1.APICollection:
	case *hubv1alpha1.APIAccess:

	default:
		log.Error().
			Str("component", "api_portal_watcher").
			Str("type", fmt.Sprintf("%T", oldObj)).
			Msg("Received delete event of unknown type")
		return
	}

	select {
	case w.refresh <- struct{}{}:
	default:
	}
}

func (w *Watcher) getPortals() ([]portal, error) {
	apiPortals, err := w.portals.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("list APIPortals: %w", err)
	}

	apiAccesses, err := w.accesses.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("list APIAccesses: %w", err)
	}
	apiAccessByName := make(map[string]*hubv1alpha1.APIAccess)
	for _, apiAccess := range apiAccesses {
		apiAccessByName[apiAccess.Name] = apiAccess
	}

	var portals []portal
	for _, apiPortal := range apiPortals {
		var apiGateway *hubv1alpha1.APIGateway

		apiGateway, err = w.gateways.Get(apiPortal.Spec.APIGateway)
		if err != nil {
			if kerror.IsNotFound(err) {
				log.Error().
					Str("portal_name", apiPortal.Name).
					Str("gateway_name", apiPortal.Spec.APIGateway).
					Msg("Unable to find APIGateway")

				continue
			}

			return nil, fmt.Errorf("get APIGateway %q: %w", apiPortal.Spec.APIGateway, err)
		}

		g := gateway{
			APIGateway:  *apiGateway,
			Collections: make(map[string]collection),
			APIs:        make(map[string]hubv1alpha1.API),
		}

		for _, apiAccessName := range apiGateway.Spec.APIAccesses {
			apiAccess := apiAccessByName[apiAccessName]
			if apiAccess == nil {
				log.Error().
					Str("api_gateway_name", apiPortal.Spec.APIGateway).
					Str("api_access_name", apiAccessName).
					Msg("Unable to find APIAccess")

				continue
			}

			accessAPIs, err := w.findAPIs(apiAccess.Spec.APISelector)
			if err != nil {
				return nil, fmt.Errorf("find APIAccess %q APIs: %w", apiAccessName, err)
			}

			for k := range accessAPIs {
				g.APIs[k] = accessAPIs[k]
			}

			collectionAPIs, err := w.findCollections(apiAccess.Spec.APICollectionSelector)
			if err != nil {
				return nil, fmt.Errorf("find APIAccess %q APICollections: %w", apiAccessName, err)
			}

			for k := range collectionAPIs {
				g.Collections[k] = collectionAPIs[k]
			}
		}

		portals = append(portals, portal{
			APIPortal: *apiPortal,
			Gateway:   g,
		})
	}

	return portals, nil
}

func (w *Watcher) findAPIs(labelSelector *metav1.LabelSelector) (map[string]hubv1alpha1.API, error) {
	if labelSelector == nil {
		return nil, nil
	}

	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		log.Error().Err(err).
			Str("selector", labelSelector.String()).
			Msg("Invalid selector")
		return nil, nil
	}

	apis, err := w.apis.List(selector)
	if err != nil {
		return nil, fmt.Errorf("list APIs using selector %q: %w", labelSelector.String(), err)
	}

	foundAPIs := make(map[string]hubv1alpha1.API)
	for _, a := range apis {
		namespace := a.Namespace
		if namespace == "" {
			namespace = "default"
		}

		foundAPIs[a.Name+"@"+namespace] = *a
	}

	return foundAPIs, nil
}

func (w *Watcher) findCollections(labelSelector *metav1.LabelSelector) (map[string]collection, error) {
	if labelSelector == nil {
		return nil, nil
	}

	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		log.Error().Err(err).
			Str("selector", selector.String()).
			Msg("Invalid selector")
		return nil, nil
	}

	collections, err := w.collections.List(selector)
	if err != nil {
		return nil, fmt.Errorf("list APICollections using selector %q: %w", labelSelector.String(), err)
	}

	foundCollections := make(map[string]collection)
	for _, c := range collections {
		apis, err := w.findAPIs(&c.Spec.APISelector)
		if err != nil {
			return nil, fmt.Errorf("find APICollection %q APIs: %w", c.Name, err)
		}

		foundCollections[c.Name] = collection{
			APICollection: *c,
			APIs:          apis,
		}
	}

	return foundCollections, nil
}
