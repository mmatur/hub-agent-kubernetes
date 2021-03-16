package store

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchConfig(t *testing.T) {
	authToken := "token1"

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		require.Equal(t, "/config", req.URL.Path)
		require.Equal(t, "Bearer "+authToken, req.Header.Get("Authorization"))

		err := json.NewEncoder(rw).Encode(&config{GitRepo: "gitRepo"})
		if err != nil {
			panic(err)
		}
	}))

	defer server.Close()

	cfg, err := fetchConfig(context.Background(), authToken, server.URL)
	require.NoError(t, err)

	assert.Equal(t, &config{GitRepo: "gitRepo"}, cfg)
}
