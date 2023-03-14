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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	logwrapper "github.com/traefik/hub-agent-kubernetes/pkg/logger"
)

// PortalAPI is a handler that exposes APIPortal information.
type PortalAPI struct {
	router     chi.Router
	httpClient *http.Client

	portal       *portal
	listAPIsResp []byte
}

// NewPortalAPI creates a new PortalAPI handler.
func NewPortalAPI(portal *portal) (*PortalAPI, error) {
	client := retryablehttp.NewClient()
	client.RetryMax = 4
	client.Logger = logwrapper.NewRetryableHTTPWrapper(log.Logger.With().
		Str("component", "portal_api").
		Logger())

	listAPIsResp, err := json.Marshal(buildListResp(portal))
	if err != nil {
		return nil, fmt.Errorf("marshal list APIs response: %w", err)
	}

	p := &PortalAPI{
		router:       chi.NewRouter(),
		httpClient:   client.StandardClient(),
		portal:       portal,
		listAPIsResp: listAPIsResp,
	}

	p.router.Get("/apis", p.handleListAPIs)
	p.router.Get("/apis/{api}", p.handleGetAPISpec)
	p.router.Get("/collections/{collection}/apis/{api}", p.handleGetCollectionAPISpec)

	return p, nil
}

// ServeHTTP serves HTTP requests.
func (p *PortalAPI) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	p.router.ServeHTTP(rw, req)
}

func (p *PortalAPI) handleListAPIs(rw http.ResponseWriter, _ *http.Request) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)

	if _, err := rw.Write(p.listAPIsResp); err != nil {
		log.Error().Err(err).
			Str("portal_name", p.portal.Name).
			Msg("Write list APIs response")
	}
}

func (p *PortalAPI) handleGetAPISpec(rw http.ResponseWriter, r *http.Request) {
	apiNameNamespace := chi.URLParam(r, "api")

	logger := log.With().
		Str("portal_name", p.portal.Name).
		Str("api_name", apiNameNamespace).
		Logger()

	a, ok := p.portal.Gateway.APIs[apiNameNamespace]
	if !ok {
		logger.Debug().Msg("API not found")
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	p.serveAPISpec(logger.WithContext(r.Context()), rw, &p.portal.Gateway, nil, &a)
}

func (p *PortalAPI) handleGetCollectionAPISpec(rw http.ResponseWriter, r *http.Request) {
	collectionName := chi.URLParam(r, "collection")
	apiNameNamespace := chi.URLParam(r, "api")

	logger := log.With().
		Str("portal_name", p.portal.Name).
		Str("collection_name", collectionName).
		Str("api_name", apiNameNamespace).
		Logger()

	c, ok := p.portal.Gateway.Collections[collectionName]
	if !ok {
		logger.Debug().Msg("APICollection not found")
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	a, ok := c.APIs[apiNameNamespace]
	if !ok {
		logger.Debug().Msg("API not found")
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	p.serveAPISpec(logger.WithContext(r.Context()), rw, &p.portal.Gateway, &c, &a)
}

func (p *PortalAPI) serveAPISpec(ctx context.Context, rw http.ResponseWriter, g *gateway, c *collection, a *hubv1alpha1.API) {
	logger := log.Ctx(ctx)

	spec, err := p.getOpenAPISpec(ctx, a)
	if err != nil {
		logger.Error().Err(err).Msg("Unable to fetch OpenAPI spec")
		rw.WriteHeader(http.StatusBadGateway)

		return
	}

	var pathPrefix string
	if c != nil {
		pathPrefix = c.Spec.PathPrefix
	}
	pathPrefix = path.Join(pathPrefix, a.Spec.PathPrefix)

	// As soon as a CustomDomain is provided on the Gateway, the API is no longer accessible through the HubDomain.
	domains := g.Status.CustomDomains
	if len(domains) == 0 {
		domains = []string{g.Status.HubDomain}
	}

	if err = overrideServersAndSecurity(spec, domains, pathPrefix); err != nil {
		logger.Error().Err(err).Msg("Unable to adapt OpenAPI spec server and security configurations")
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)

	if err = json.NewEncoder(rw).Encode(spec); err != nil {
		logger.Error().Msg("Unable to serve OpenAPI spec")
	}
}

func (p *PortalAPI) getOpenAPISpec(ctx context.Context, a *hubv1alpha1.API) (*openapi3.T, error) {
	svc := a.Spec.Service

	var openapiURL *url.URL
	switch {
	case svc.OpenAPISpec.URL != "":
		u, err := url.Parse(svc.OpenAPISpec.URL)
		if err != nil {
			return nil, fmt.Errorf("parse OpenAPI URL %q: %w", svc.OpenAPISpec.URL, err)
		}
		openapiURL = u

	case svc.Port.Number != 0 || svc.OpenAPISpec.Port != nil && svc.OpenAPISpec.Port.Number != 0:
		protocol := svc.OpenAPISpec.Protocol
		if svc.OpenAPISpec.Protocol == "" {
			protocol = "http"
		}

		port := svc.Port.Number
		if svc.OpenAPISpec.Port != nil {
			port = svc.OpenAPISpec.Port.Number
		}

		namespace := a.Namespace
		if namespace == "" {
			namespace = "default"
		}

		openapiURL = &url.URL{
			Scheme: protocol,
			Host:   fmt.Sprint(svc.Name, ".", namespace, ":", port),
			Path:   svc.OpenAPISpec.Path,
		}
	default:
		return nil, errors.New("no spec endpoint specified")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openapiURL.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request %q: %w", openapiURL.String(), err)
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Accept", "application/yaml")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request %q: %w", openapiURL.String(), err)
	}

	rawSpec, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read spec %q: %w", openapiURL.String(), err)
	}

	// A new loader must be created each time. LoadFromData mutates the internal state of Loader.
	// LoadFromURI doesn't take a context, therefore, we must do the call ourselves.
	spec, err := openapi3.NewLoader().LoadFromData(rawSpec)
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI spec: %w", err)
	}

	return spec, nil
}

func overrideServersAndSecurity(spec *openapi3.T, domains []string, pathPrefix string) error {
	servers, err := overrideServerDomains(spec.Servers, domains, pathPrefix)
	if err != nil {
		return fmt.Errorf("override global server domains: %w", err)
	}
	spec.Servers = servers
	spec.Security = nil

	for p := range spec.Paths {
		spec.Paths[p].Servers, err = overrideServerDomains(spec.Paths[p].Servers, domains, pathPrefix)
		if err != nil {
			return fmt.Errorf("override path %q server domains: %w", p, err)
		}

		for method := range spec.Paths[p].Operations() {
			operation := spec.Paths[p].GetOperation(method)

			if operation == nil || operation.Servers == nil {
				continue
			}

			servers, err = overrideServerDomains(*operation.Servers, domains, pathPrefix)
			if err != nil {
				return fmt.Errorf("override path %q server domains for method %q: %w", p, method, err)
			}
			operation.Servers = &servers
			operation.Security = nil

			spec.Paths[p].SetOperation(method, operation)
		}
	}

	return nil
}

func overrideServerDomains(servers openapi3.Servers, domains []string, pathPrefix string) (openapi3.Servers, error) {
	if len(servers) == 0 || servers[0].URL == "" {
		return servers, nil
	}

	// TODO: Handle variable substitutions before parsing the URL. (e.g. using Servers.BasePath)
	originalServerURL := servers[0]
	serverURL, err := url.Parse(originalServerURL.URL)
	if err != nil {
		return nil, fmt.Errorf("parse server url %q: %w", originalServerURL.URL, err)
	}

	var overriddenServers openapi3.Servers
	for _, domain := range domains {
		s := *originalServerURL
		s.URL = "https://" + domain + path.Join("/", pathPrefix, serverURL.Path)

		overriddenServers = append(overriddenServers, &s)
	}

	return overriddenServers, nil
}

type listResp struct {
	Collections []collectionResp `json:"collections"`
	APIs        []apiResp        `json:"apis"`
}

type collectionResp struct {
	Name string    `json:"name"`
	APIs []apiResp `json:"apis"`
}

type apiResp struct {
	Name     string `json:"name"`
	SpecLink string `json:"specLink"`
}

func buildListResp(p *portal) listResp {
	var resp listResp
	for collectionName, c := range p.Gateway.Collections {
		cr := collectionResp{
			Name: collectionName,
			APIs: make([]apiResp, 0, len(c.APIs)),
		}

		for apiNameNamespace, a := range c.APIs {
			cr.APIs = append(cr.APIs, apiResp{
				Name:     a.Name,
				SpecLink: fmt.Sprintf("/collections/%s/apis/%s", collectionName, apiNameNamespace),
			})
		}
		sortAPIsResp(cr.APIs)

		resp.Collections = append(resp.Collections, cr)
	}
	sortCollectionsResp(resp.Collections)

	for apiNameNamespace, a := range p.Gateway.APIs {
		resp.APIs = append(resp.APIs, apiResp{
			Name:     a.Name,
			SpecLink: fmt.Sprintf("/apis/%s", apiNameNamespace),
		})
	}
	sortAPIsResp(resp.APIs)

	if resp.APIs == nil {
		resp.APIs = make([]apiResp, 0)
	}
	if resp.Collections == nil {
		resp.Collections = make([]collectionResp, 0)
	}

	return resp
}

func sortAPIsResp(apis []apiResp) {
	sort.Slice(apis, func(i, j int) bool {
		return apis[i].Name < apis[j].Name
	})
}

func sortCollectionsResp(collections []collectionResp) {
	sort.Slice(collections, func(i, j int) bool {
		return collections[i].Name < collections[j].Name
	})
}
