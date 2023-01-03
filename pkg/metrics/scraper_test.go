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

package metrics_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent-kubernetes/pkg/metrics"
)

func TestScraper_ScrapeTraefik(t *testing.T) {
	tests := []struct {
		desc    string
		metrics string
		want    []metrics.Metric
	}{
		{
			desc:    "Traefik v2.8+",
			metrics: "testdata/traefik-v2-8-metrics.txt",
			want: []metrics.Metric{
				&metrics.Histogram{Name: metrics.MetricRequestDuration, EdgeIngress: "myIngress@default", Sum: 0.0137623, Count: 1},
				&metrics.Counter{Name: metrics.MetricRequests, EdgeIngress: "myIngress@default", Value: 2},
				// edge cases, TLS/middleware enable on entrypoint
				&metrics.Counter{Name: metrics.MetricRequests, EdgeIngress: "app-obe@whoami", Value: 38},
			},
		},
		{
			desc:    "Traefik older versions",
			metrics: "testdata/traefik-metrics.txt",
			want: []metrics.Metric{
				&metrics.Histogram{Name: metrics.MetricRequestDuration, EdgeIngress: "myIngress@default", Sum: 0.0137623, Count: 1},
				&metrics.Counter{Name: metrics.MetricRequests, EdgeIngress: "myIngress@default", Value: 2},
				// edge cases, TLS/middleware enable on entrypoint
				&metrics.Counter{Name: metrics.MetricRequests, EdgeIngress: "app-obe@whoami", Value: 38},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			srvURL := startServer(t, test.metrics)
			s := metrics.NewScraper(http.DefaultClient)

			got, err := s.Scrape(context.Background(), metrics.ParserTraefik, srvURL, metrics.ScrapeState{
				Ingresses: map[string]struct{}{
					"myIngress@default.ingress.networking.k8s.io": {},
					"app-obe@whoami.ingress.networking.k8s.io":    {},
				},
			})
			require.NoError(t, err)

			assert.ElementsMatch(t, got, test.want)
		})
	}
}

func startServer(t *testing.T, file string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Clean(file))
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)

	return srv.URL
}
