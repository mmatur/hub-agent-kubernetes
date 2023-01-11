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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWatcher_OnAdd(t *testing.T) {
	oasSpec, err := os.ReadFile("./fixtures/openapi-spec-read.yaml")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := rw.Write(oasSpec)
		require.NoError(t, err)
	}))

	switcher := NewHandlerSwitcher()
	watcher := NewWatcher(switcher)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	t.Cleanup(cancel)

	go watcher.Run(ctx)
	watcher.OnAdd(createCatalog("my-catalog", srv.URL))

	time.Sleep(10 * time.Millisecond)

	testCases := []struct {
		desc   string
		path   string
		jwt    string
		status int
		body   []byte
	}{
		{
			desc:   "list services",
			path:   "/my-catalog/services",
			status: http.StatusOK,
			// json lib add a new line in the end.
			body: []byte("[\"svc@ns\"]\n"),
		},
		{
			desc:   "get Open API spec",
			path:   "/my-catalog/services/svc@ns",
			status: http.StatusOK,
			body:   oasSpec,
		},
		{
			desc:   "not found",
			path:   "/my-catalog/services/unknown@ns",
			status: http.StatusNotFound,
			body:   []byte{},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			rw := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://localhost"+test.path, nil)

			switcher.ServeHTTP(rw, req)

			resp, err := io.ReadAll(rw.Body)
			require.NoError(t, err)

			assert.Equal(t, test.status, rw.Code)
			assert.Equal(t, test.body, resp)
		})
	}
}

func createCatalog(name, openAPISpecURL string) *hubv1alpha1.Catalog {
	catalog := &hubv1alpha1.Catalog{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: hubv1alpha1.CatalogSpec{
			Services: []hubv1alpha1.CatalogService{
				{
					Name:           "svc",
					Namespace:      "ns",
					Port:           80,
					PathPrefix:     "",
					OpenAPISpecURL: openAPISpecURL,
				},
			},
			CustomDomains: nil,
		},
	}
	return catalog
}

func TestWatcher_OnUpdate(t *testing.T) {
	oasSpec, err := os.ReadFile("./fixtures/openapi-spec-read.yaml")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := rw.Write(oasSpec)
		require.NoError(t, err)
	}))

	switcher := NewHandlerSwitcher()
	watcher := NewWatcher(switcher)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	t.Cleanup(cancel)

	go watcher.Run(ctx)

	catalog := createCatalog("my-catalog", srv.URL)
	watcher.OnAdd(catalog)

	time.Sleep(10 * time.Millisecond)

	catalog.Spec.Services = append(catalog.Spec.Services, hubv1alpha1.CatalogService{
		Name:           "svc2",
		Namespace:      "ns",
		Port:           80,
		OpenAPISpecURL: srv.URL,
	})

	watcher.OnUpdate(nil, catalog)

	time.Sleep(10 * time.Millisecond)

	testCases := []struct {
		desc   string
		path   string
		jwt    string
		status int
		body   []byte
	}{
		{
			desc:   "list services",
			path:   "/my-catalog/services",
			status: http.StatusOK,
			body:   []byte("[\"svc@ns\",\"svc2@ns\"]\n"),
		},
		{
			desc:   "get Open API spec",
			path:   "/my-catalog/services/svc@ns",
			status: http.StatusOK,
			body:   oasSpec,
		},
		{
			desc:   "get Open API spec",
			path:   "/my-catalog/services/svc2@ns",
			status: http.StatusOK,
			body:   oasSpec,
		},
		{
			desc:   "not found",
			path:   "/my-catalog/services/unknown@ns",
			status: http.StatusNotFound,
			body:   []byte{},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			rw := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://localhost"+test.path, nil)

			switcher.ServeHTTP(rw, req)

			resp, err := io.ReadAll(rw.Body)
			require.NoError(t, err)

			assert.Equal(t, test.status, rw.Code)
			assert.Equal(t, test.body, resp)
		})
	}
}

func TestWatcher_OnDelete(t *testing.T) {
	oasSpec, err := os.ReadFile("./fixtures/openapi-spec-read.yaml")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := rw.Write(oasSpec)
		require.NoError(t, err)
	}))

	switcher := NewHandlerSwitcher()
	watcher := NewWatcher(switcher)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	t.Cleanup(cancel)

	go watcher.Run(ctx)

	catalog := createCatalog("my-catalog", srv.URL)
	watcher.OnAdd(catalog)

	time.Sleep(10 * time.Millisecond)

	watcher.OnDelete(catalog)

	time.Sleep(10 * time.Millisecond)

	testCases := []struct {
		desc   string
		path   string
		jwt    string
		status int
		body   []byte
	}{
		{
			desc:   "list services",
			path:   "/my-catalog/services",
			status: http.StatusNotFound,
		},
		{
			desc:   "get Open API spec",
			path:   "/my-catalog/services/svc@ns",
			status: http.StatusNotFound,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			rw := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://localhost"+test.path, nil)

			switcher.ServeHTTP(rw, req)

			assert.Equal(t, test.status, rw.Code)
		})
	}
}
