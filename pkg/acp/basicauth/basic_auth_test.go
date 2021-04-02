package basicauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicAuthFail(t *testing.T) {
	cfg := &Config{
		Users: []string{"test"},
	}
	_, err := NewHandler(cfg, "authName")
	require.Error(t, err)

	auth2 := Config{
		Users: []string{"test:test"},
	}
	handler, err := NewHandler(&auth2, "acp@my-ns")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.SetBasicAuth("test", "test")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestBasicAuthUserHeader(t *testing.T) {
	cfg := &Config{
		Users:                 []string{"test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/"},
		ForwardUsernameHeader: "User",
	}
	handler, err := NewHandler(cfg, "acp@my-ns")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.SetBasicAuth("test", "test")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "test", rec.Header().Get("User"))
}
