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

package oauthintro

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent-kubernetes/pkg/httpclient"
	"github.com/traefik/hub-agent-kubernetes/pkg/optional"
)

func TestNewHandler(t *testing.T) {
	tests := []struct {
		desc    string
		cfg     Config
		wantErr bool
	}{
		{
			desc: "missing client config URL",
			cfg: Config{
				ClientConfig: ClientConfig{
					URL: "",
				},
			},
			wantErr: true,
		},
		{
			desc: "missing client config auth kind",
			cfg: Config{
				ClientConfig: ClientConfig{
					URL: "https://idp.example.com",
					Auth: ClientConfigAuth{
						Kind: "",
					},
				},
			},
			wantErr: true,
		},
		{
			desc: "invalid client config auth kind",
			cfg: Config{
				ClientConfig: ClientConfig{
					URL: "https://idp.example.com",
					Auth: ClientConfigAuth{
						Kind: "invalid",
					},
				},
			},
			wantErr: true,
		},
		{
			desc: "missing client config auth secret name",
			cfg: Config{
				ClientConfig: ClientConfig{
					URL: "https://idp.example.com",
					Auth: ClientConfigAuth{
						Kind: "Bearer",
						Secret: SecretReference{
							Name: "",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			desc: "missing client config auth secret namespace",
			cfg: Config{
				ClientConfig: ClientConfig{
					URL: "https://idp.example.com",
					Auth: ClientConfigAuth{
						Kind: "Bearer",
						Secret: SecretReference{
							Name: "name",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			desc: "missing valid token sorce",
			cfg: Config{
				ClientConfig: ClientConfig{
					URL: "https://idp.example.com",
					Auth: ClientConfigAuth{
						Kind: "Bearer",
						Secret: SecretReference{
							Name:      "name",
							Namespace: "namespace",
						},
					},
				},
				TokenSource: TokenSource{
					Header: "",
					Query:  "",
					Cookie: "",
				},
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			_, err := NewHandler(&test.cfg, "oauth-intro")

			if test.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestOAuthIntro_GetsTokenFromHeader(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		require.NoError(t, r.ParseForm())
		assert.Equal(t, "abc", r.Form.Get("token"))

		_, _ = w.Write([]byte(`{"active": true}`))
	}))
	defer srv.Close()

	cfg := Config{
		ClientConfig: ClientConfig{
			URL: srv.URL,
			Auth: ClientConfigAuth{
				Kind: "Bearer",
				Secret: SecretReference{
					Name:      "name",
					Namespace: "namespace",
				},
				Key:   "Authorization",
				Value: "Bearer token",
			},
		},
		TokenSource: TokenSource{
			Header:           "Authorization",
			HeaderAuthScheme: "Bearer",
		},
	}
	handler, err := NewHandler(&cfg, "oauth-intro")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer  abc")

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
	assert.Equal(t, 1, callCount)
}

func TestOAuthIntro_GetsTokenFromQuery(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		require.NoError(t, r.ParseForm())
		assert.Equal(t, "abc", r.Form.Get("token"))

		_, _ = w.Write([]byte(`{"active": true}`))
	}))
	defer srv.Close()

	cfg := Config{
		ClientConfig: ClientConfig{
			URL: srv.URL,
			Auth: ClientConfigAuth{
				Kind: "Bearer",
				Secret: SecretReference{
					Name:      "name",
					Namespace: "namespace",
				},
				Key:   "Authorization",
				Value: "Bearer token",
			},
		},
		TokenSource: TokenSource{
			Query: "tok",
		},
	}
	handler, err := NewHandler(&cfg, "oauth-intro")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
	require.NoError(t, err)

	req.Header.Set("X-Forwarded-Uri", "/?tok=abc")

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
	assert.Equal(t, 1, callCount)
}

func TestOAuthIntro_GetsTokenFromCookie(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		require.NoError(t, r.ParseForm())
		assert.Equal(t, "abc", r.Form.Get("token"))

		_, _ = w.Write([]byte(`{"active": true}`))
	}))
	defer srv.Close()

	cfg := Config{
		ClientConfig: ClientConfig{
			URL: srv.URL,
			Auth: ClientConfigAuth{
				Kind: "Bearer",
				Secret: SecretReference{
					Name:      "name",
					Namespace: "namespace",
				},
				Key:   "Authorization",
				Value: "Bearer token",
			},
		},
		TokenSource: TokenSource{
			Cookie: "tok",
		},
	}
	handler, err := NewHandler(&cfg, "oauth-intro")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
	require.NoError(t, err)

	req.Header.Set("Cookie", "tok=abc")

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
	assert.Equal(t, 1, callCount)
}

func TestOAuthIntro_FailsIfMissingToken(t *testing.T) {
	cfg := Config{
		ClientConfig: ClientConfig{
			URL: "https://idp.example.com",
			Auth: ClientConfigAuth{
				Kind: "Bearer",
				Secret: SecretReference{
					Name:      "name",
					Namespace: "namespace",
				},
				Key:   "Authorization",
				Value: "Bearer token",
			},
		},
		TokenSource: TokenSource{
			Header:           "Authorization",
			HeaderAuthScheme: "Bearer",
		},
	}
	handler, err := NewHandler(&cfg, "oauth-intro")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
	require.NoError(t, err)

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Result().StatusCode)
}

func TestOAuthIntro_AuthenticatesWithHeader(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		assert.Equal(t, "Bearer intro-token", r.Header.Get("Authorization"))

		require.NoError(t, r.ParseForm())
		assert.Equal(t, "abc", r.Form.Get("token"))

		_, _ = w.Write([]byte(`{"active": true}`))
	}))
	defer srv.Close()

	cfg := Config{
		ClientConfig: ClientConfig{
			URL: srv.URL,
			Auth: ClientConfigAuth{
				Kind: "Bearer",
				Secret: SecretReference{
					Name:      "name",
					Namespace: "namespace",
				},
				Key:   "Authorization",
				Value: "Bearer intro-token",
			},
		},
		TokenSource: TokenSource{
			Header: "Token",
		},
	}
	handler, err := NewHandler(&cfg, "oauth-intro")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
	require.NoError(t, err)

	req.Header.Set("Token", "abc")

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
	assert.Equal(t, 1, callCount)
}

func TestOAuthIntro_AuthenticatesWithQuery(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		assert.Equal(t, "intro-token", r.URL.Query().Get("token"))

		require.NoError(t, r.ParseForm())
		assert.Equal(t, "abc", r.Form.Get("token"))

		_, _ = w.Write([]byte(`{"active": true}`))
	}))
	defer srv.Close()

	cfg := Config{
		ClientConfig: ClientConfig{
			URL: srv.URL,
			Auth: ClientConfigAuth{
				Kind: "Query",
				Secret: SecretReference{
					Name:      "name",
					Namespace: "namespace",
				},
				Key:   "token",
				Value: "intro-token",
			},
		},
		TokenSource: TokenSource{
			Header: "Token",
		},
	}
	handler, err := NewHandler(&cfg, "oauth-intro")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
	require.NoError(t, err)

	req.Header.Set("Token", "abc")

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
	assert.Equal(t, 1, callCount)
}

func TestOAuthIntro_SendsCustomHeaders(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))

		require.NoError(t, r.ParseForm())
		assert.Equal(t, "abc", r.Form.Get("token"))

		assert.Equal(t, "bar", r.Header.Get("Foo"))
		assert.Equal(t, "bat", r.Header.Get("Baz"))
		assert.Equal(t, "GET", r.Header.Get("Request-Method"))
		assert.Equal(t, "admin", r.Header.Get("Request-Group"))
		assert.Equal(t, "bar", r.Header.Get("Request-Query-Foo"))

		_, _ = w.Write([]byte(`{"active": true}`))
	}))
	defer srv.Close()

	cfg := Config{
		ClientConfig: ClientConfig{
			URL: srv.URL,
			Auth: ClientConfigAuth{
				Kind: "Bearer",
				Secret: SecretReference{
					Name:      "name",
					Namespace: "namespace",
				},
				Key:   "Authorization",
				Value: "Bearer token",
			},
			Headers: map[string]string{
				"Foo":               "bar",
				"Baz":               "bat",
				"Request-Method":    "{{ .Request.Method }}",
				"Request-Group":     `{{ .Request.Header.Get "Group" }}`,
				"Request-Query-Foo": `{{ .Request.URL.Query.Get "foo" }}`,
			},
		},
		TokenSource: TokenSource{
			Header:           "Authorization",
			HeaderAuthScheme: "Bearer",
		},
	}
	handler, err := NewHandler(&cfg, "oauth-intro")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/?foo=bar", http.NoBody)
	req.Header.Set("Authorization", "Bearer abc")
	req.Header.Set("Group", "admin")
	require.NoError(t, err)

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
	assert.Equal(t, 1, callCount)
}

func TestOAuthIntro_HandlesTokenInactive(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		_, _ = w.Write([]byte(`{"active": false}`))
	}))
	defer srv.Close()

	cfg := Config{
		ClientConfig: ClientConfig{
			URL: srv.URL,
			Config: httpclient.Config{
				MaxRetries: optional.NewInt(0),
			},
			Auth: ClientConfigAuth{
				Kind: "Bearer",
				Secret: SecretReference{
					Name:      "name",
					Namespace: "namespace",
				},
				Key:   "Authorization",
				Value: "Bearer token",
			},
		},
		TokenSource: TokenSource{
			Header:           "Authorization",
			HeaderAuthScheme: "Bearer",
		},
	}
	handler, err := NewHandler(&cfg, "oauth-intro")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer abc")
	require.NoError(t, err)

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Result().StatusCode)
	assert.Equal(t, 1, callCount)
}

func TestOAuthIntro_HandlesIntrospectionServerError(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := Config{
		ClientConfig: ClientConfig{
			URL: srv.URL,
			Config: httpclient.Config{
				MaxRetries: optional.NewInt(0),
			},
			Auth: ClientConfigAuth{
				Kind: "Bearer",
				Secret: SecretReference{
					Name:      "name",
					Namespace: "namespace",
				},
				Key:   "Authorization",
				Value: "Bearer token",
			},
		},
		TokenSource: TokenSource{
			Header:           "Authorization",
			HeaderAuthScheme: "Bearer",
		},
	}
	handler, err := NewHandler(&cfg, "oauth-intro")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer abc")
	require.NoError(t, err)

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Result().StatusCode)
	assert.Equal(t, 1, callCount)
}

func TestOAuthIntro_HandlesWrongClaims(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))

		_, _ = w.Write([]byte(`{"active": true, "grp": "test"}`))
	}))
	defer srv.Close()

	cfg := Config{
		ClientConfig: ClientConfig{
			URL: srv.URL,
			Config: httpclient.Config{
				MaxRetries: optional.NewInt(0),
			},
			Auth: ClientConfigAuth{
				Kind: "Bearer",
				Secret: SecretReference{
					Name:      "name",
					Namespace: "namespace",
				},
				Key:   "Authorization",
				Value: "Bearer token",
			},
		},
		TokenSource: TokenSource{
			Header:           "Authorization",
			HeaderAuthScheme: "Bearer",
		},
		Claims: "Equals(`grp`, `admin`)",
	}
	handler, err := NewHandler(&cfg, "oauth-intro")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer abc")

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Result().StatusCode)
	assert.Equal(t, 1, callCount)
}

func TestOAuthIntro_ForwardsClaimsHeaders(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		_, _ = w.Write([]byte(`{"active": true, "grp": "test"}`))
	}))
	defer srv.Close()

	cfg := Config{
		ClientConfig: ClientConfig{
			URL: srv.URL,
			Config: httpclient.Config{
				MaxRetries: optional.NewInt(0),
			},
			Auth: ClientConfigAuth{
				Kind: "Bearer",
				Secret: SecretReference{
					Name:      "name",
					Namespace: "namespace",
				},
				Key:   "Authorization",
				Value: "Bearer token",
			},
		},
		TokenSource: TokenSource{
			Header:           "Authorization",
			HeaderAuthScheme: "Bearer",
		},
		ForwardHeaders: map[string]string{"Group": "grp"},
	}
	handler, err := NewHandler(&cfg, "oauth-intro")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer  abc")

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
	assert.Equal(t, "test", rec.Header().Get("Group"))
	assert.Equal(t, 1, callCount)
}
