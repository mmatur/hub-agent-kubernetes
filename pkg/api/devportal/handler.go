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
	"fmt"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
)

// Handler exposes both an API and a UI for a set of APIPortals.
// The handler can be safely updated to support more APIPortals as they come and go.
type Handler struct {
	handlerMu sync.RWMutex
	handler   http.Handler
}

// NewHandler builds a new instance of Handler.
func NewHandler() *Handler {
	return &Handler{
		handler: http.NotFoundHandler(),
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	h.handlerMu.RLock()
	handler := h.handler
	h.handlerMu.RUnlock()

	handler.ServeHTTP(rw, req)
}

// Update safely updates the current http.ServeMux with a new one built for serving the given portals.
func (h *Handler) Update(portals []portal) error {
	router := chi.NewRouter()

	for _, p := range portals {
		p := p

		apiHandler, err := NewPortalAPI(&p)
		if err != nil {
			return fmt.Errorf("create portal %q API handler: %w", p.Name, err)
		}

		router.Mount("/api/"+p.Name, apiHandler)
	}

	uiHandler, err := NewPortalUI(portals)
	if err != nil {
		return fmt.Errorf("create portal UI handler: %w", err)
	}

	router.Mount("/", uiHandler)

	h.handlerMu.Lock()
	h.handler = router
	h.handlerMu.Unlock()

	return nil
}
