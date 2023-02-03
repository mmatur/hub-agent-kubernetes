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

package apikey

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHandler(t *testing.T) {
	tests := []struct {
		desc    string
		cfg     Config
		wantErr bool
	}{
		{
			desc:    "missing token source",
			cfg:     Config{},
			wantErr: true,
		},
		{
			desc: "missing keys",
			cfg: Config{
				Header: "Api-Key",
			},
			wantErr: true,
		},
		{
			desc: "ill-formed key, id cannot be empty",
			cfg: Config{
				Header: "Api-Key",
				Keys: []Key{
					{
						ID:    "",
						Value: "17fa993d5eecbd361f30baf0b9b2329ad053bb6d5fec2228eca55e9b4914fface3af69bcc9a6b5f7ff093aa9a0d00811d0b2a3ee67eac60c57e79d2fd99bbde0",
					},
				},
			},
			wantErr: true,
		},
		{
			desc: "ill-formed key, value cannot be empty",
			cfg: Config{
				Header: "Api-Key",
				Keys: []Key{
					{
						ID:    "id-1",
						Value: "",
					},
				},
			},
			wantErr: true,
		},
		{
			desc: "ill-formed keys, duplicated key ID",
			cfg: Config{
				Header: "Api-Key",
				Keys: []Key{
					{
						ID:    "id",
						Value: "",
					},
					{
						ID:    "id",
						Value: "",
					},
				},
			},
			wantErr: true,
		},
		{
			desc: "ill-formed keys, duplicated key value",
			cfg: Config{
				Header: "Api-Key",
				Keys: []Key{
					{
						ID:    "id-1",
						Value: "value",
					},
					{
						ID:    "id-2",
						Value: "value",
					},
				},
			},
			wantErr: true,
		},
		{
			desc: "ok",
			cfg: Config{
				Header: "Api-Key",
				Keys: []Key{
					{
						ID:    "id-1",
						Value: "17fa993d5eecbd361f30baf0b9b2329ad053bb6d5fec2228eca55e9b4914fface3af69bcc9a6b5f7ff093aa9a0d00811d0b2a3ee67eac60c57e79d2fd99bbde0",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			_, err := NewHandler(&test.cfg, "api-key")

			if test.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

const (
	validAPIKey   = "key"
	invalidAPIKey = "invalid"
)

func TestServeHTTP(t *testing.T) {
	tests := []struct {
		desc         string
		cfg          Config
		header       string
		queryTraefik string
		queryNginx   string
		cookie       string
		wantStatus   int
	}{
		{
			desc: "get API key from header",
			cfg: Config{
				Header: "Api-Key",
				Keys: []Key{
					{
						ID:    "id-1",
						Value: "17fa993d5eecbd361f30baf0b9b2329ad053bb6d5fec2228eca55e9b4914fface3af69bcc9a6b5f7ff093aa9a0d00811d0b2a3ee67eac60c57e79d2fd99bbde0",
					},
				},
			},
			header:     validAPIKey,
			wantStatus: http.StatusOK,
		},
		{
			desc: "get API key from query (Traefik)",
			cfg: Config{
				Query: "api-key",
				Keys: []Key{
					{
						ID:    "id-1",
						Value: "17fa993d5eecbd361f30baf0b9b2329ad053bb6d5fec2228eca55e9b4914fface3af69bcc9a6b5f7ff093aa9a0d00811d0b2a3ee67eac60c57e79d2fd99bbde0",
					},
				},
			},
			queryTraefik: validAPIKey,
			wantStatus:   http.StatusOK,
		},
		{
			desc: "get API key from query (Nginx)",
			cfg: Config{
				Query: "api-key",
				Keys: []Key{
					{
						ID:    "id-1",
						Value: "17fa993d5eecbd361f30baf0b9b2329ad053bb6d5fec2228eca55e9b4914fface3af69bcc9a6b5f7ff093aa9a0d00811d0b2a3ee67eac60c57e79d2fd99bbde0",
					},
				},
			},
			queryNginx: validAPIKey,
			wantStatus: http.StatusOK,
		},
		{
			desc: "get API key from cookie",
			cfg: Config{
				Cookie: "api_key",
				Keys: []Key{
					{
						ID:    "id-1",
						Value: "17fa993d5eecbd361f30baf0b9b2329ad053bb6d5fec2228eca55e9b4914fface3af69bcc9a6b5f7ff093aa9a0d00811d0b2a3ee67eac60c57e79d2fd99bbde0",
					},
				},
			},
			cookie:     validAPIKey,
			wantStatus: http.StatusOK,
		},
		{
			desc: "get API key prioritize header",
			cfg: Config{
				Header: "Api-Key",
				Query:  "api-key",
				Keys: []Key{
					{
						ID:    "id-1",
						Value: "17fa993d5eecbd361f30baf0b9b2329ad053bb6d5fec2228eca55e9b4914fface3af69bcc9a6b5f7ff093aa9a0d00811d0b2a3ee67eac60c57e79d2fd99bbde0",
					},
				},
			},
			header:       validAPIKey,
			queryTraefik: invalidAPIKey,
			wantStatus:   http.StatusOK,
		},
		{
			desc: "can't get API key",
			cfg: Config{
				Header: "Api-Key",
				Keys: []Key{
					{
						ID:    "id-1",
						Value: "17fa993d5eecbd361f30baf0b9b2329ad053bb6d5fec2228eca55e9b4914fface3af69bcc9a6b5f7ff093aa9a0d00811d0b2a3ee67eac60c57e79d2fd99bbde0",
					},
				},
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			desc: "invalid API key",
			cfg: Config{
				Header: "Api-Key",
				Keys: []Key{
					{
						ID:    "id-1",
						Value: "17fa993d5eecbd361f30baf0b9b2329ad053bb6d5fec2228eca55e9b4914fface3af69bcc9a6b5f7ff093aa9a0d00811d0b2a3ee67eac60c57e79d2fd99bbde0",
					},
				},
			},
			header:     invalidAPIKey,
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			apiKey, err := NewHandler(&test.cfg, "api-key")
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
			require.NoError(t, err)

			if test.header != "" {
				req.Header.Set("Api-Key", test.header)
			}

			if test.queryTraefik != "" {
				req.Header.Set("X-Forwarded-Uri", "/?api-key="+test.queryTraefik)
			}

			if test.queryNginx != "" {
				req.Header.Set("X-Original-Url", "https://localhost/?api-key="+test.queryNginx)
			}

			if test.cookie != "" {
				req.AddCookie(&http.Cookie{
					Name:  "api_key",
					Value: test.cookie,
				})
			}

			apiKey.ServeHTTP(rr, req)

			assert.Equal(t, test.wantStatus, rr.Code)
		})
	}
}

func TestServeHTTPForwardsHeader(t *testing.T) {
	tests := []struct {
		desc       string
		cfg        Config
		wantStatus int
		wantHeader map[string]string
	}{
		{
			desc: "ID is forwarded if configured",
			cfg: Config{
				Header: "Api-Key",
				Keys: []Key{
					{
						ID:    "id-1",
						Value: "17fa993d5eecbd361f30baf0b9b2329ad053bb6d5fec2228eca55e9b4914fface3af69bcc9a6b5f7ff093aa9a0d00811d0b2a3ee67eac60c57e79d2fd99bbde0",
						Metadata: map[string]string{
							"group": "group-1",
						},
					},
				},
				ForwardHeaders: map[string]string{
					"Id":    "_id",
					"Group": "group",
				},
			},
			wantStatus: http.StatusOK,
			wantHeader: map[string]string{
				"Id":    "id-1",
				"Group": "group-1",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			apiKey, err := NewHandler(&test.cfg, "api-key")
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
			require.NoError(t, err)

			req.Header.Set("Api-Key", "key")

			apiKey.ServeHTTP(rr, req)

			assert.Equal(t, test.wantStatus, rr.Code)
			for k, v := range test.wantHeader {
				assert.Equal(t, v, rr.Header().Get(k))
			}
		})
	}
}
