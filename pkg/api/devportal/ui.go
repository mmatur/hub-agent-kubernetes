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
	"bytes"
	"fmt"
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
	portalui "github.com/traefik/hub-agent-kubernetes/portal"
)

// PortalUI is a handler for exposing APIPortals' UI.
type PortalUI struct {
	router chi.Router

	templatedIndexes map[string][]byte
}

type portalIndexData struct {
	Name        string
	Description string
}

// NewPortalUI creates a new PortalUI handler.
func NewPortalUI(portals []portal) (*PortalUI, error) {
	indexTemplate, err := template.ParseFS(portalui.WebUI, "index.html")
	if err != nil {
		return nil, fmt.Errorf("parse index.html template: %w", err)
	}

	templatedIndexes, err := templatePortalIndexes(indexTemplate, portals)
	if err != nil {
		return nil, fmt.Errorf("template portal indexes: %w", err)
	}

	h := &PortalUI{
		router:           chi.NewRouter(),
		templatedIndexes: templatedIndexes,
	}

	h.router.Get("/", h.handleIndex)
	h.router.Method(http.MethodGet, "/*", http.FileServer(http.FS(portalui.WebUI)))

	return h, nil
}

// ServeHTTP serves HTTP requests.
func (p *PortalUI) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	p.router.ServeHTTP(rw, req)
}

func (p *PortalUI) handleIndex(rw http.ResponseWriter, req *http.Request) {
	index, ok := p.templatedIndexes[req.Host]
	if !ok {
		log.Debug().Str("host", req.Host).Msg("APIPortal not found for host")
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	rw.Header().Set("X-Frame-Options", "SAMEORIGIN")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")

	if _, err := rw.Write(index); err != nil {
		log.Error().Err(err).Msg("Unable to serve APIPortal UI index")
	}
}

func templatePortalIndexes(indexTemplate *template.Template, portals []portal) (map[string][]byte, error) {
	indexes := make(map[string][]byte)
	for _, p := range portals {
		data := portalIndexData{
			Name:        p.Name,
			Description: p.Spec.Description,
		}

		var buff bytes.Buffer
		if err := indexTemplate.Execute(&buff, data); err != nil {
			return nil, fmt.Errorf("template portal %q index: %w", p.Name, err)
		}

		// As soon as a CustomDomain is provided on the Portal, the UI is no longer accessible through the HubDomain.
		templated := buff.Bytes()
		for _, customDomain := range p.Status.CustomDomains {
			indexes[customDomain] = templated
		}
		if len(p.Status.CustomDomains) == 0 {
			indexes[p.Status.HubDomain] = templated
		}
	}

	return indexes, nil
}
