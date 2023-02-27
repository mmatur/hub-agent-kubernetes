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
	"html/template"
	"net/http"
	"net/url"
	"path"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/api"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/logger"
	"github.com/traefik/hub-agent-kubernetes/portal"
)

// NOTE: if we use the same watcher for all resources, then we need to restart it when new CRDs are
// created/removed like for example when Traefik is installed and IngressRoutes are added.
// Always listening to non-existing resources would cause errors.
// Also, if multiple clients of this watcher are not interested in the same resources
// add a parameter to NewWatcher to subscribe only to a subset of events.

// Watcher watches APIPortals resources and builds configurations out of them.
type Watcher struct {
	portalsMu   sync.RWMutex
	portals     map[string]api.Portal
	hostPortals map[string]*api.Portal
	previous    uint64

	refresh chan struct{}

	switcher   *HTTPHandlerSwitcher
	httpClient *http.Client

	indexTemplate *template.Template
}

// NewWatcher returns a new watcher to track APIPortal resources. It calls the given Updater when an APIPortal is modified at most
// once every throttle.
func NewWatcher(switcher *HTTPHandlerSwitcher) (*Watcher, error) {
	client := retryablehttp.NewClient()
	client.RetryMax = 4
	client.Logger = logger.NewRetryableHTTPWrapper(log.Logger.With().Str("component", "watcher_portal_client").Logger())

	indexTemplate, err := template.ParseFS(portal.WebUI, "index.html")
	if err != nil {
		return nil, fmt.Errorf("parse index.html template: %w", err)
	}

	return &Watcher{
		portals:       make(map[string]api.Portal),
		hostPortals:   make(map[string]*api.Portal),
		refresh:       make(chan struct{}, 1),
		switcher:      switcher,
		httpClient:    client.StandardClient(),
		indexTemplate: indexTemplate,
	}, nil
}

// Run launches listener if the watcher is dirty.
func (w *Watcher) Run(ctx context.Context) {
	for {
		select {
		case <-w.refresh:
			w.portalsMu.RLock()

			hash, err := hashstructure.Hash(w.portals, hashstructure.FormatV2, nil)
			if err != nil {
				log.Error().Err(err).Msg("Unable to hash")
			}

			w.portalsMu.RUnlock()

			if err == nil && w.previous == hash {
				continue
			}

			w.previous = hash

			log.Debug().Msg("Refreshing APIPortal handlers")

			w.switcher.UpdateHandler(w.buildRoutes())

		case <-ctx.Done():
			return
		}
	}
}

// OnAdd implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *Watcher) OnAdd(obj interface{}) {
	switch v := obj.(type) {
	case *hubv1alpha1.APIPortal:
		w.updatePortalsFromCRD(v)

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
func (w *Watcher) OnUpdate(_, newObj interface{}) {
	switch v := newObj.(type) {
	case *hubv1alpha1.APIPortal:
		w.updatePortalsFromCRD(v)

	default:
		log.Error().
			Str("component", "api_portal_watcher").
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
	case *hubv1alpha1.APIPortal:
		w.deletePortal(v.Name)

	default:
		log.Error().
			Str("component", "api_portal_watcher").
			Str("type", fmt.Sprintf("%T", obj)).
			Msg("Received delete event of unknown type")
		return
	}

	select {
	case w.refresh <- struct{}{}:
	default:
	}
}

func (w *Watcher) updatePortalsFromCRD(p *hubv1alpha1.APIPortal) {
	w.portalsMu.Lock()
	defer w.portalsMu.Unlock()

	var verifiedCustomDomains []api.CustomDomain
	for _, customDomain := range p.Status.CustomDomains {
		verifiedCustomDomains = append(verifiedCustomDomains, api.CustomDomain{
			Name:     customDomain,
			Verified: true,
		})
	}

	var verifiedAPICustomDomains []api.CustomDomain
	for _, customDomain := range p.Status.APICustomDomains {
		verifiedAPICustomDomains = append(verifiedAPICustomDomains, api.CustomDomain{
			Name:     customDomain,
			Verified: true,
		})
	}

	hubDomain := p.Status.HubDomain

	pc := api.Portal{
		Name:             p.Name,
		Description:      p.Spec.Description,
		HubDomain:        hubDomain,
		APIHubDomain:     p.Status.HubDomain,
		CustomDomains:    verifiedCustomDomains,
		APICustomDomains: verifiedAPICustomDomains,
	}

	w.portals[p.Name] = pc
	w.hostPortals[hubDomain] = &pc
}

func (w *Watcher) deletePortal(name string) {
	w.portalsMu.Lock()
	defer w.portalsMu.Unlock()

	c, ok := w.portals[name]
	if !ok {
		return
	}

	delete(w.hostPortals, c.HubDomain)
	delete(w.portals, c.Name)
}

func (w *Watcher) buildRoutes() http.Handler {
	w.portalsMu.RLock()
	defer w.portalsMu.RUnlock()

	router := chi.NewRouter()
	for name, cfg := range w.portals {
		cfg := cfg
		router.Mount("/api/"+name, w.buildRoute(name, &cfg))
	}

	router.Get("/", func(rw http.ResponseWriter, req *http.Request) {
		w.portalsMu.RLock()
		c, ok := w.hostPortals[req.Host]
		w.portalsMu.RUnlock()

		if !ok {
			log.Debug().Str("host", req.Host).Msg("APIPortal not found")
			rw.WriteHeader(http.StatusNotFound)
			return
		}

		data := struct {
			Name        string
			Description string
		}{
			Name:        c.Name,
			Description: c.Description,
		}

		if err := w.indexTemplate.Execute(rw, data); err != nil {
			log.Error().Err(err).
				Str("host", req.Host).
				Str("api_portal_name", data.Name).
				Msg("Unable to execute index template")

			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
	})

	router.Method(http.MethodGet, "/*", http.FileServer(http.FS(portal.WebUI)))

	return router
}

func (w *Watcher) buildRoute(name string, p *api.Portal) http.Handler {
	var apis []string

	urlByName := map[string]string{}
	pathPrefixByName := map[string]string{}
	// TODO: fill apis, urlByName, pathPrefixByName and oasBasePathByName from portal APIs

	router := chi.NewRouter()
	router.Get("/apis", func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(rw).Encode(apis); err != nil {
			log.Error().Err(err).
				Str("api_portal_name", name).
				Msg("Encode APIs")
		}
	})
	router.Get("/apis/{api}", func(rw http.ResponseWriter, req *http.Request) {
		apiName := chi.URLParam(req, "api")

		u, found := urlByName[apiName]
		if !found {
			log.Debug().
				Str("api_portal_name", name).
				Str("api_name", apiName).
				Msg("API not found")
			rw.WriteHeader(http.StatusNotFound)

			return
		}

		r, err := http.NewRequestWithContext(req.Context(), http.MethodGet, u, http.NoBody)
		if err != nil {
			log.Error().Err(err).
				Str("api_portal_name", name).
				Str("api_name", apiName).
				Str("url", u).
				Msg("New request")
			rw.WriteHeader(http.StatusInternalServerError)

			return
		}

		resp, err := w.httpClient.Do(r)
		if err != nil {
			rw.WriteHeader(http.StatusBadGateway)
			log.Error().Err(err).
				Str("api_portal_name", name).
				Str("api_name", apiName).
				Str("url", u).
				Msg("Do request")

			return
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode < 200 || resp.StatusCode > 300 {
			rw.WriteHeader(http.StatusBadGateway)
			log.Error().Err(err).
				Str("api_portal_name", name).
				Str("api_name", apiName).
				Str("url", u).
				Int("status_code", resp.StatusCode).
				Msg("Unexpected status code")

			return
		}

		var oas openapi3.T
		if err := json.NewDecoder(resp.Body).Decode(&oas); err != nil {
			log.Error().Err(err).
				Str("api_portal_name", name).
				Str("api_name", apiName).
				Str("url", u).
				Msg("Decode open api spec")

			return
		}

		if err := overrideServersAndSecurity(&oas, p, pathPrefixByName[apiName]); err != nil {
			log.Error().Err(err).
				Str("api_portal_name", name).
				Str("api_name", apiName).
				Str("url", u).
				Msg("Override servers and security")

			return
		}

		rw.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		rw.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(rw).Encode(oas); err != nil {
			log.Error().Err(err).
				Str("api_portal_name", name).
				Str("api_name", apiName).
				Str("url", u).
				Msg("Write content")
		}
	})

	return router
}

func overrideServersAndSecurity(oas *openapi3.T, p *api.Portal, apiPathPrefix string) error {
	var domains []string
	for _, domain := range p.APICustomDomains {
		domains = append(domains, domain.Name)
	}

	if len(domains) == 0 {
		domains = append(domains, p.APIHubDomain)
	}

	var err error
	oas.Servers, err = serversWithDomains(oas.Servers, domains, apiPathPrefix)
	if err != nil {
		return fmt.Errorf("unable to build oas servers: %w", err)
	}
	oas.Security = nil

	for i := range oas.Paths {
		if len(oas.Paths[i].Servers) > 0 {
			oas.Paths[i].Servers, err = serversWithDomains(oas.Paths[i].Servers, domains, apiPathPrefix)
			if err != nil {
				return fmt.Errorf("unable to build path servers: %w", err)
			}
		}

		for method := range oas.Paths[i].Operations() {
			operation := oas.Paths[i].GetOperation(method)

			if len(*operation.Servers) > 0 {
				servers, err := serversWithDomains(*operation.Servers, domains, apiPathPrefix)
				if err != nil {
					return fmt.Errorf("unable to build operation servers: %w", err)
				}
				operation.Servers = &servers
			}
			operation.Security = nil

			oas.Paths[i].SetOperation(method, operation)
		}
	}

	return nil
}

func serversWithDomains(servers openapi3.Servers, domains []string, prefix string) (openapi3.Servers, error) {
	baseServer := &openapi3.Server{}
	if len(servers) != 0 {
		baseServer = servers[0]
	}

	pathS, err := pathServers(servers)
	if err != nil {
		return nil, fmt.Errorf("unable to get path servers: %w", err)
	}

	var mergedServers openapi3.Servers
	for _, domain := range domains {
		s := *baseServer
		s.URL = "https://" + domain + path.Join(prefix, pathS)

		mergedServers = append(mergedServers, &s)
	}

	return mergedServers, nil
}

func pathServers(servers openapi3.Servers) (string, error) {
	if len(servers) == 0 {
		return "", nil
	}

	if servers[0].URL == "" {
		return "", nil
	}

	u, err := url.Parse(servers[0].URL)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}

	return u.Path, nil
}
