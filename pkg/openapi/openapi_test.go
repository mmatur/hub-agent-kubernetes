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

package openapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoader_Load(t *testing.T) {
	tests := []struct {
		desc       string
		spec       []byte
		statusCode int

		wantErr  require.ErrorAssertionFunc
		wantSpec *Spec
	}{
		{
			desc: "load valid JSON specification",
			spec: []byte(`{
				"openapi": "3.0.1",
				"info": {
					"title": "my-api"
				},
				"paths": {}
			}`),
			statusCode: http.StatusOK,
			wantErr:    require.NoError,
			wantSpec: &Spec{
				OpenAPI: "3.0.1",
			},
		},
		{
			desc: "load valid YAML specification",
			spec: []byte(`
openapi: 3.0.1
info:
  title: my-api
paths: {}
`),
			statusCode: http.StatusOK,
			wantErr:    require.NoError,
			wantSpec: &Spec{
				OpenAPI: "3.0.1",
			},
		},
		{
			desc:       "invalid specification",
			spec:       []byte("invalid specification"),
			statusCode: http.StatusOK,
			wantErr:    require.Error,
		},
		{
			desc:       "specification not found",
			statusCode: http.StatusNotFound,
			wantErr:    require.Error,
		},
		{
			desc:       "unable to fetch specification: Internal Error",
			statusCode: http.StatusInternalServerError,
			wantErr:    require.Error,
		},
	}

	loader := NewLoader()

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			wantPath := "/spec.json"

			mux := http.NewServeMux()
			mux.HandleFunc(wantPath, func(rw http.ResponseWriter, req *http.Request) {
				rw.WriteHeader(test.statusCode)
				_, _ = rw.Write(test.spec)
			})

			srv := httptest.NewServer(mux)
			u, err := url.Parse(srv.URL)
			require.NoError(t, err)

			u = u.JoinPath(wantPath)

			spec, err := loader.Load(context.Background(), u)
			test.wantErr(t, err)

			assert.Equal(t, spec, test.wantSpec)
		})
	}
}

func TestLoader_Load_ServerUnreachable(t *testing.T) {
	loader := NewLoader()

	u, err := url.Parse("http://127.0.0.1:56789/spec.json")
	require.NoError(t, err)

	_, err = loader.Load(context.Background(), u)
	require.Error(t, err)
}

func TestSpec_Validate(t *testing.T) {
	tests := []struct {
		desc    string
		spec    Spec
		wantErr error
	}{
		{
			desc: "OpenAPI version 3.0.0",
			spec: Spec{OpenAPI: "3.0.0"},
		},
		{
			desc: "OpenAPI version 3.0.3",
			spec: Spec{OpenAPI: "3.0.3"},
		},
		{
			desc: "OpenAPI version 3.1.0",
			spec: Spec{OpenAPI: "3.1.0"},
		},
		{
			desc:    "OpenAPI version 4.0.0",
			spec:    Spec{OpenAPI: "4.0.0"},
			wantErr: errors.New(`unsupported version "4.0.0"`),
		},
		{
			desc:    "Swagger version 2.0.0",
			spec:    Spec{Swagger: "2.0.0"},
			wantErr: errors.New(`unsupported version "2.0.0"`),
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			err := test.spec.Validate()
			assert.Equal(t, test.wantErr, err)
		})
	}
}
