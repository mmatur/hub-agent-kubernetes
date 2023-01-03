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
