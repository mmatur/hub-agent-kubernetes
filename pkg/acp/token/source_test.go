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

package token

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_extractToken(t *testing.T) {
	tests := []struct {
		desc         string
		source       Source
		header       http.Header
		queryTraefik string
		queryNginx   string
		cookie       string
		want         string
		wantErr      string
	}{
		{
			desc: "token extracted from header",
			source: Source{
				Header: "Authorization",
			},
			header: http.Header{
				"Authorization": []string{"token"},
			},
			want: "token",
		},
		{
			desc: "token extracted from query with all token sources set",
			source: Source{
				Header: "Authorization",
				Query:  "tok",
				Cookie: "tok",
			},
			queryTraefik: "tok=token",
			want:         "token",
		},
		{
			desc: "token extracted from cookie with all token sources set",
			source: Source{
				Header: "Authorization",
				Query:  "tok",
				Cookie: "token",
			},
			cookie: "token=token",
			want:   "token",
		},
		{
			desc: "token extracted from header (bearer)",
			source: Source{
				Header:           "Authorization",
				HeaderAuthScheme: "Bearer",
			},
			header: http.Header{
				"Authorization": []string{"Bearer token"},
			},
			want: "token",
		},
		{
			desc: "token extracted from query parameter (Traefik)",
			source: Source{
				Query: "tok",
			},
			queryTraefik: "tok=token",
			want:         "token",
		},
		{
			desc: "token extracted from query parameter (Nginx)",
			source: Source{
				Query: "tok",
			},
			queryNginx: "tok=token",
			want:       "token",
		},
		{
			desc: "token extracted from cookie",
			source: Source{
				Cookie: "token",
			},
			cookie: "token=token",
			want:   "token",
		},
		{
			desc: "prioritize header over query parameter",
			source: Source{
				Header: "Authorization",
				Query:  "tok",
			},
			header: http.Header{
				"Authorization": []string{"token1"},
			},
			queryTraefik: "tok=token2",
			want:         "token1",
		},
		{
			desc: "prioritize query parameter over cookie",
			source: Source{
				Query:  "tok",
				Cookie: "token",
			},
			queryTraefik: "tok=token1",
			cookie:       "token=token2",
			want:         "token1",
		},
		{
			desc: "invalid Authorization header scheme",
			source: Source{
				Header:           "Authorization",
				HeaderAuthScheme: "Bearer",
			},
			header: http.Header{
				"Authorization": []string{"Basic token"},
			},
			want: "",
		},
		{
			desc:    "missing token source",
			wantErr: "missing token source",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "https://localhost", http.NoBody)
			require.NoError(t, err)

			if test.header != nil {
				req.Header = test.header
			}
			if test.queryTraefik != "" {
				req.Header.Set("X-Forwarded-Uri", "https://localhost?"+test.queryTraefik)
			}
			if test.queryNginx != "" {
				req.Header.Set("X-Original-Url", "https://localhost?"+test.queryNginx)
			}
			if test.cookie != "" {
				req.Header.Set("Cookie", test.cookie)
			}

			tok, err := Extract(req, test.source)
			if test.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, test.wantErr, err.Error())
				return
			}

			assert.Equal(t, test.want, tok)
		})
	}
}
