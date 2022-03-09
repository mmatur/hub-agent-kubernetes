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
			TunnelID:        "id",
			BrokerEndpoint:  "endpoint",
			ClusterEndpoint: ":443",
		},
		{
			TunnelID:        "id2",
			BrokerEndpoint:  "endpoint2",
			ClusterEndpoint: ":4443",
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
