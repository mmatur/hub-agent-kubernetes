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

package version

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-github/v47/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChecker_check(t *testing.T) {
	tests := []struct {
		desc string

		version  string
		upToDate bool
	}{
		{
			desc:     "Notify Hub platform when cluster is up to date",
			version:  "v0.5.0",
			upToDate: true,
		},
		{
			desc:    "Notify Hub platform when cluster is outdated",
			version: "v0.4.0",
		},
		{
			desc:    "Notify Hub platform when cluster not use a tag",
			version: "8712d4f",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			latestVersion := "v0.5.0"

			h := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				assert.Equal(t, version, req.Header.Get("Traefik-Hub-Agent-Version"))
				assert.Equal(t, "kubernetes", req.Header.Get("Traefik-Hub-Agent-Platform"))

				b, err := json.Marshal([]*github.RepositoryTag{{Name: &latestVersion}})
				if err != nil {
					http.Error(rw, err.Error(), http.StatusInternalServerError)
					return
				}

				_, _ = rw.Write(b)
			})

			srv := httptest.NewServer(h)
			updateURL, err := url.Parse(srv.URL + "/")
			require.NoError(t, err)

			cluster := newClusterServiceMock(t).
				OnSetVersionStatus(Status{
					UpToDate:       test.upToDate,
					CurrentVersion: test.version,
					LatestVersion:  latestVersion,
				}).TypedReturns(nil).Once().
				Parent

			c := NewChecker(cluster)
			c.github = newGitHubClient(updateURL)
			c.version = test.version

			err = c.check(context.Background())

			if test.upToDate {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
		})
	}
}
