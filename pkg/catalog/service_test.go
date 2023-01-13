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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/traefik/hub-agent-kubernetes/pkg/topology/state"
)

func TestServiceRegistry_GetURL(t *testing.T) {
	oasServiceURLs := map[string]string{
		"whoami@default": "https://whoami.default.svc:8080/spec.json",
	}
	tests := []struct {
		desc      string
		name      string
		namespace string
		want      string
	}{
		{
			desc:      "URL exists for the service",
			name:      "whoami",
			namespace: "default",
			want:      "https://whoami.default.svc:8080/spec.json",
		},
		{
			desc:      "URL doesn't exist for the service: wrong namespace",
			name:      "whoami",
			namespace: "whoami",
		},
		{
			desc:      "URL doesn't exist for the service: wrong name",
			name:      "default",
			namespace: "default",
		},
		{
			desc:      "empty name",
			namespace: "default",
		},
		{
			desc: "empty namespace",
			name: "whoami",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			registry := NewServiceRegistry()
			registry.oasServiceURLs = oasServiceURLs
			got := registry.GetURL(test.name, test.namespace)

			assert.Equal(t, got, test.want)
		})
	}
}

func TestServiceRegistry_TopologyStateChanged(t *testing.T) {
	registry := NewServiceRegistry()

	registry.TopologyStateChanged(context.Background(), &state.Cluster{
		Services: map[string]*state.Service{
			"svc-1@default": {
				Name:      "svc-1",
				Namespace: "default",
				OpenAPISpecLocation: &state.OpenAPISpecLocation{
					Path: "/spec.json",
					Port: 8080,
				},
			},
			"svc-2@default": {
				Name:      "svc-2",
				Namespace: "default",
			},
		},
	})

	select {
	case <-time.After(10 * time.Millisecond):
		t.Fatalf("timed out waiting for update")
	case <-registry.Updated():
	}

	assert.Equal(t, "http://svc-1.default.svc:8080/spec.json", registry.GetURL("svc-1", "default"))
	assert.Empty(t, registry.GetURL("svc-2", "default"))
	assert.Empty(t, registry.GetURL("unknown", "default"))
}

func TestServiceRegistry_TopologyStateChanged_notChanged(t *testing.T) {
	registry := NewServiceRegistry()
	registry.oasServiceURLs = map[string]string{
		"svc-1@default": "http://svc-1.default.svc:8080/spec.json",
	}

	registry.TopologyStateChanged(context.Background(), &state.Cluster{
		Services: map[string]*state.Service{
			"svc-1@default": {
				Name:      "svc-1",
				Namespace: "default",
				OpenAPISpecLocation: &state.OpenAPISpecLocation{
					Path: "/spec.json",
					Port: 8080,
				},
			},
			"svc-2@default": {
				Name:      "svc-2",
				Namespace: "default",
			},
		},
	})

	select {
	case <-time.After(10 * time.Millisecond):
	case <-registry.Updated():
		t.Fatalf("expected no update")
	}

	assert.Equal(t, "http://svc-1.default.svc:8080/spec.json", registry.GetURL("svc-1", "default"))
	assert.Empty(t, registry.GetURL("svc-2", "default"))
	assert.Empty(t, registry.GetURL("unknown", "default"))
}

func TestServiceRegistry_TopologyStateChanged_serviceRemoved(t *testing.T) {
	registry := NewServiceRegistry()
	registry.oasServiceURLs = map[string]string{
		"svc-1@default": "http://svc-1.default.svc:8080/spec.json",
	}

	registry.TopologyStateChanged(context.Background(), &state.Cluster{
		Services: map[string]*state.Service{
			"svc-2@default": {
				Name:      "svc-2",
				Namespace: "default",
			},
		},
	})

	select {
	case <-time.After(10 * time.Millisecond):
		t.Fatalf("timed out waiting for update")
	case <-registry.Updated():
	}

	assert.Empty(t, registry.GetURL("svc-1", "default"))
	assert.Empty(t, registry.GetURL("svc-2", "default"))
	assert.Empty(t, registry.GetURL("unknown", "default"))
}
