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
	srvURL := startServer(t, "testdata/traefik-metrics.txt")

	s := metrics.NewScraper(http.DefaultClient)

	got, err := s.Scrape(context.Background(), metrics.ParserTraefik, srvURL, metrics.ScrapeState{
		Ingresses:        map[string]struct{}{"myIngress@default.ingress.networking.k8s.io": {}, "app-obe@whoami.ingress.networking.k8s.io": {}},
		IngressRoutes:    map[string]struct{}{"myIngressRoute@default.ingressroute.traefik.containo.us": {}, "app-traefik@whoami.ingressroute.traefik.containo.us": {}},
		ServiceIngresses: map[string][]string{"whoami@default": {"myIngress@default.ingress.networking.k8s.io"}, "whoami2@default": {"myIngress@default.ingress.networking.k8s.io"}},
		TraefikServiceNames: map[string]string{
			"default-whoami-80":         "whoami@default",
			"default-whoami2-80":        "whoami2@default",
			"default-whoami-sdfsdfsdsd": "whoami@default",
			"default-whoami3-80":        "whoami3@default",
			"whoami-app-traefik":        "whoami@whoami",
		},
	})
	require.NoError(t, err)

	// router
	assert.Contains(t, got, &metrics.Histogram{Name: metrics.MetricRequestDuration, Ingress: "myIngress@default.ingress.networking.k8s.io", Sum: 0.0137623, Count: 1})
	assert.Contains(t, got, &metrics.Histogram{Name: metrics.MetricRequestDuration, Ingress: "myIngressRoute@default.ingressroute.traefik.containo.us", Sum: 0.0216373, Count: 1})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress@default.ingress.networking.k8s.io", Value: 2})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngressRoute@default.ingressroute.traefik.containo.us", Value: 1})
	// edge cases, TLS/middleware enable on entrypoint
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "app-obe@whoami.ingress.networking.k8s.io", Value: 38})

	require.Len(t, got, 5)
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
