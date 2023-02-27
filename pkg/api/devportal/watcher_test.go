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

const generatedDomain = "generated.hub.domain"

func TestWatcher_OnAdd(t *testing.T) {
	oasSpec, err := os.ReadFile("./fixtures/openapi-spec-read-before.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, writeErr := rw.Write(oasSpec)
		require.NoError(t, writeErr)
	}))

	switcher := NewHandlerSwitcher()
	watcher, err := NewWatcher(switcher)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	t.Cleanup(cancel)

	go watcher.Run(ctx)
	portal := createPortal(srv.URL)
	portal.Status.HubDomain = generatedDomain

	watcher.OnAdd(portal)

	time.Sleep(50 * time.Millisecond)

	testCases := []struct {
		desc   string
		path   string
		jwt    string
		status int
		body   []byte
	}{
		{
			desc:   "list apis",
			path:   "/api/my-portal/apis",
			status: http.StatusOK,
			// json lib add a new line in the end.
			body: []byte("[\"svc@ns\"]\n"),
		},
		{
			desc:   "not found",
			path:   "/api/my-portal/apis/unknown@ns",
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

			assert.Equal(t, test.status, rw.Code)

			// No API for the moment
			// resp, err := io.ReadAll(rw.Body)
			// require.NoError(t, err)
			// assert.Equal(t, test.body, resp)
		})
	}
}

func TestWatcher_OnDelete(t *testing.T) {
	oasSpec, err := os.ReadFile("./fixtures/openapi-spec-read-before.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, writeErr := rw.Write(oasSpec)
		require.NoError(t, writeErr)
	}))

	switcher := NewHandlerSwitcher()
	watcher, err := NewWatcher(switcher)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	t.Cleanup(cancel)

	go watcher.Run(ctx)

	portal := createPortal(srv.URL)
	watcher.OnAdd(portal)

	time.Sleep(10 * time.Millisecond)

	watcher.OnDelete(portal)

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
			path:   "/my-portal/apis",
			status: http.StatusNotFound,
		},
		{
			desc:   "get Open API spec",
			path:   "/my-portal/apis/svc@ns",
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

func createPortal(openAPISpecURL string) *hubv1alpha1.APIPortal {
	_ = openAPISpecURL
	portal := &hubv1alpha1.APIPortal{
		ObjectMeta: metav1.ObjectMeta{Name: "my-portal"},
		Spec:       hubv1alpha1.APIPortalSpec{},
	}
	return portal
}
