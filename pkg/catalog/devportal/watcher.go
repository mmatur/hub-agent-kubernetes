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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/catalog"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/logger"
)

// NOTE: if we use the same watcher for all resources, then we need to restart it when new CRDs are
// created/removed like for example when Traefik is installed and IngressRoutes are added.
// Always listening to non-existing resources would cause errors.
// Also, if multiple clients of this watcher are not interested in the same resources
// add a parameter to NewWatcher to subscribe only to a subset of events.

// Watcher watches access control policy resources and builds configurations out of them.
type Watcher struct {
	catalogsMu sync.RWMutex
	catalogs   map[string]catalog.Catalog
	previous   uint64

	refresh chan struct{}

	switcher   *HTTPHandlerSwitcher
	httpClient *http.Client
}

// NewWatcher returns a new watcher to track Catalog resources. It calls the given Updater when a Catalog is modified at most
// once every throttle.
func NewWatcher(switcher *HTTPHandlerSwitcher) *Watcher {
	client := retryablehttp.NewClient()
	client.RetryMax = 4
	client.Logger = logger.NewRetryableHTTPWrapper(log.Logger.With().Str("component", "watcher_catalog_client").Logger())

	return &Watcher{
		catalogs:   make(map[string]catalog.Catalog),
		refresh:    make(chan struct{}, 1),
		switcher:   switcher,
		httpClient: client.StandardClient(),
	}
}

// Run launches listener if the watcher is dirty.
func (w *Watcher) Run(ctx context.Context) {
	for {
		select {
		case <-w.refresh:
			w.catalogsMu.RLock()

			hash, err := hashstructure.Hash(w.catalogs, hashstructure.FormatV2, nil)
			if err != nil {
				log.Error().Err(err).Msg("Unable to hash")
			}

			w.catalogsMu.RUnlock()

			if err == nil && w.previous == hash {
				continue
			}

			w.previous = hash

			log.Debug().Msg("Refreshing Catalog handlers")

			w.switcher.UpdateHandler(w.buildRoutes())

		case <-ctx.Done():
			return
		}
	}
}

// OnAdd implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *Watcher) OnAdd(obj interface{}) {
	switch v := obj.(type) {
	case *hubv1alpha1.Catalog:
		w.updateCatalogsFromCRD(v)

	default:
		log.Error().
			Str("component", "catalog_watcher").
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
func (w *Watcher) OnUpdate(_, newObj interface{}) {
	switch v := newObj.(type) {
	case *hubv1alpha1.Catalog:
		w.updateCatalogsFromCRD(v)

	default:
		log.Error().
			Str("component", "catalog_watcher").
			Str("type", fmt.Sprintf("%T", newObj)).
			Msg("Received update event of unknown type")
		return
	}

	select {
	case w.refresh <- struct{}{}:
	default:
	}
}

// OnDelete implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *Watcher) OnDelete(obj interface{}) {
	switch v := obj.(type) {
	case *hubv1alpha1.Catalog:
		w.catalogsMu.Lock()
		delete(w.catalogs, v.ObjectMeta.Name)
		w.catalogsMu.Unlock()

	default:
		log.Error().
			Str("component", "catalog_watcher").
			Str("type", fmt.Sprintf("%T", obj)).
			Msg("Received delete event of unknown type")
		return
	}

	select {
	case w.refresh <- struct{}{}:
	default:
	}
}

func (w *Watcher) updateCatalogsFromCRD(c *hubv1alpha1.Catalog) {
	w.catalogsMu.Lock()
	defer w.catalogsMu.Unlock()

	var verifiedCustomDomains []catalog.CustomDomain
	for _, customDomain := range c.Status.CustomDomains {
		verifiedCustomDomains = append(verifiedCustomDomains, catalog.CustomDomain{
			Name:     customDomain,
			Verified: true,
		})
	}

	statusSpecURLByName := make(map[string]string)
	for _, svc := range c.Status.Services {
		statusSpecURLByName[svc.Name+"@"+svc.Namespace] = svc.OpenAPISpecURL
	}

	var services []catalog.Service
	for _, svc := range c.Spec.Services {
		svc.OpenAPISpecURL = statusSpecURLByName[svc.Name+"@"+svc.Namespace]

		services = append(services, svc)
	}
	w.catalogs[c.Name] = catalog.Catalog{
		Name:          c.Name,
		Services:      services,
		CustomDomains: verifiedCustomDomains,
	}
}

func (w *Watcher) buildRoutes() http.Handler {
	w.catalogsMu.RLock()
	defer w.catalogsMu.RUnlock()

	router := chi.NewRouter()
	for name, cfg := range w.catalogs {
		cfg := cfg
		path := "/" + name

		router.Mount(path, w.buildRoute(name, &cfg))
	}

	return router
}

func (w *Watcher) buildRoute(name string, c *catalog.Catalog) http.Handler {
	var services []string

	urlByName := map[string]string{}
	for _, service := range c.Services {
		key := service.Name + "@" + service.Namespace
		services = append(services, key)
		urlByName[key] = service.OpenAPISpecURL
	}

	router := chi.NewRouter()
	router.Get("/services", func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(rw).Encode(services); err != nil {
			log.Error().Err(err).
				Str("catalog_name", name).
				Msg("Encode services")
		}
	})
	router.Get("/services/{service}", func(rw http.ResponseWriter, req *http.Request) {
		svcName := chi.URLParam(req, "service")

		u, found := urlByName[svcName]
		if !found {
			log.Debug().
				Str("catalog_name", name).
				Str("service_name", svcName).
				Msg("Service not found")
			rw.WriteHeader(http.StatusNotFound)

			return
		}

		r, err := http.NewRequestWithContext(req.Context(), http.MethodGet, u, http.NoBody)
		if err != nil {
			log.Error().Err(err).
				Str("catalog_name", name).
				Str("service_name", svcName).
				Str("url", u).
				Msg("New request")
			rw.WriteHeader(http.StatusInternalServerError)

			return
		}

		resp, err := w.httpClient.Do(r)
		if err != nil {
			rw.WriteHeader(http.StatusBadGateway)
			log.Error().Err(err).
				Str("catalog_name", name).
				Str("service_name", svcName).
				Str("url", u).
				Msg("Do request")

			return
		}

		if resp.StatusCode < 200 || resp.StatusCode > 300 {
			rw.WriteHeader(http.StatusBadGateway)
			log.Error().Err(err).
				Str("catalog_name", name).
				Str("service_name", svcName).
				Str("url", u).
				Int("status_code", resp.StatusCode).
				Msg("Unexpected status code")

			return
		}

		rw.WriteHeader(http.StatusOK)
		rw.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		if _, err := io.Copy(rw, resp.Body); err != nil {
			log.Error().Err(err).
				Str("catalog_name", name).
				Str("service_name", svcName).
				Str("url", u).
				Msg("Copy content")
		}
	})

	return router
}
