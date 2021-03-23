package auth

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
