/*
Copyright (C) 2022 Traefik Labs

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

package tunnel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_ListClusterTunnelEndpoints(t *testing.T) {
	wantEndpoints := []Endpoint{
		{
			TunnelID:       "id",
			BrokerEndpoint: "endpoint",
		},
		{
			TunnelID:       "id2",
			BrokerEndpoint: "endpoint2",
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/tunnel-endpoints", func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "Bearer token", req.Header.Get("Authorization"))

		err := json.NewEncoder(rw).Encode(wantEndpoints)
		require.NoError(t, err)
	})
	srv := httptest.NewServer(mux)

	client, err := NewClient(srv.URL, "token")
	require.NoError(t, err)

	endpoints, err := client.ListClusterTunnelEndpoints(context.Background())
	assert.NoError(t, err)

	assert.Equal(t, wantEndpoints, endpoints)
}

func TestClient_ListClusterTunnelEndpoints_handleError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tunnel-endpoints", func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "Bearer token", req.Header.Get("Authorization"))

		rw.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)

	client, err := NewClient(srv.URL, "token")
	require.NoError(t, err)
	// We remove the retryable client to not last too long.
	client.httpClient = http.DefaultClient

	_, err = client.ListClusterTunnelEndpoints(context.Background())
	assert.Error(t, err)
}
