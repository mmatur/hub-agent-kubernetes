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

package catalog

import (
	"context"
	"fmt"
	"net/url"
	"sync"

	"github.com/traefik/hub-agent-kubernetes/pkg/topology/state"
)

// ServiceRegistry is a registry of Service OpenAPI Spec URLs.
type ServiceRegistry struct {
	oasServiceURLsMu sync.RWMutex
	oasServiceURLs   map[string]string

	updated chan struct{}
}

// NewServiceRegistry creates a new ServiceRegistry.
func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		oasServiceURLs: make(map[string]string),
		updated:        make(chan struct{}, 1),
	}
}

// GetURL gets the OpenAPI Spec URL for the given service.
func (r *ServiceRegistry) GetURL(name, namespace string) string {
	r.oasServiceURLsMu.RLock()
	defer r.oasServiceURLsMu.RUnlock()

	return r.oasServiceURLs[objectKey(name, namespace)]
}

// Updated returns a chan which gets triggered whenever a OpenAPI Spec URL changes.
func (r *ServiceRegistry) Updated() <-chan struct{} {
	return r.updated
}

// TopologyStateChanged is called every time the topology state changes.
func (r *ServiceRegistry) TopologyStateChanged(_ context.Context, st *state.Cluster) {
	if st == nil {
		return
	}

	r.oasServiceURLsMu.RLock()

	var changed bool
	oasServiceURLs := make(map[string]string)
	for _, svc := range st.Services {
		if svc.OpenAPISpecLocation == nil {
			continue
		}

		u := url.URL{
			Scheme: "http",
			Host:   fmt.Sprintf("%s.%s.svc:%d", svc.Name, svc.Namespace, svc.OpenAPISpecLocation.Port),
			Path:   svc.OpenAPISpecLocation.Path,
		}
		specURL := u.String()

		key := objectKey(svc.Name, svc.Namespace)
		oasServiceURLs[key] = specURL
		if specURL != r.oasServiceURLs[key] {
			changed = true
		}
	}

	// It's considered changed if at least one URL has been added/modified or if a URL has been removed.
	changed = changed || len(r.oasServiceURLs) != len(oasServiceURLs)

	r.oasServiceURLsMu.RUnlock()

	if !changed {
		return
	}

	r.oasServiceURLsMu.Lock()
	r.oasServiceURLs = oasServiceURLs
	r.oasServiceURLsMu.Unlock()

	select {
	case r.updated <- struct{}{}:
	default:
	}
}

func objectKey(name, ns string) string {
	return name + "@" + ns
}
