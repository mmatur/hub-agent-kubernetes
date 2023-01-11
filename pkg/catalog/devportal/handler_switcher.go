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
	"net/http"
	"sync"
)

// HTTPHandlerSwitcher allows hot switching of http.ServeMux.
type HTTPHandlerSwitcher struct {
	handlerMu sync.RWMutex
	handler   http.Handler
}

// NewHandlerSwitcher builds a new instance of HTTPHandlerSwitcher.
func NewHandlerSwitcher() *HTTPHandlerSwitcher {
	return &HTTPHandlerSwitcher{
		handler: http.NotFoundHandler(),
	}
}

func (h *HTTPHandlerSwitcher) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	h.handlerMu.RLock()
	handler := h.handler
	h.handlerMu.RUnlock()

	handler.ServeHTTP(rw, req)
}

// UpdateHandler safely updates the current http.ServeMux with a new one.
func (h *HTTPHandlerSwitcher) UpdateHandler(handler http.Handler) {
	if handler == nil {
		return
	}

	h.handlerMu.Lock()
	h.handler = handler
	h.handlerMu.Unlock()
}
