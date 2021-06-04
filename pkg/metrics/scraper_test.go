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
	"github.com/traefik/hub-agent/pkg/metrics"
)

func TestScraper_ScrapeNginx(t *testing.T) {
	srvURL := startServer(t, "testdata/nginx-metrics.txt")

	s := metrics.NewScraper(http.DefaultClient)

	got, err := s.Scrape(context.Background(), metrics.ParserNginx, []string{srvURL}, metrics.ScrapeState{})
	require.NoError(t, err)

	require.Len(t, got, 12)

	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "whoami@default.ingress.networking.k8s.io", Value: 20})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "whoami@default.ingress.networking.k8s.io", Value: 19})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestClientErrors, Ingress: "whoami@default.ingress.networking.k8s.io", Value: 19})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "whoami@default.ingress.networking.k8s.io", Value: 18})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestErrors, Ingress: "whoami@default.ingress.networking.k8s.io", Value: 18})
	assert.Contains(t, got, &metrics.Histogram{
		Name:    metrics.MetricRequestDuration,
		Ingress: "whoami@default.ingress.networking.k8s.io",
		Sum:     0.030000000000000006,
		Count:   20,
	})

	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "whoami@default.ingress.networking.k8s.io", Service: "whoami@default", Value: 20})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "whoami@default.ingress.networking.k8s.io", Service: "whoami@default", Value: 19})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestClientErrors, Ingress: "whoami@default.ingress.networking.k8s.io", Service: "whoami@default", Value: 19})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "whoami@default.ingress.networking.k8s.io", Service: "whoami2@default", Value: 18})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestErrors, Ingress: "whoami@default.ingress.networking.k8s.io", Service: "whoami2@default", Value: 18})
	assert.Contains(t, got, &metrics.Histogram{
		Name:    metrics.MetricRequestDuration,
		Ingress: "whoami@default.ingress.networking.k8s.io",
		Service: "whoami@default",
		Sum:     0.030000000000000006,
		Count:   20,
	})
}

func TestScraper_ScrapeTraefik(t *testing.T) {
	srvURL := startServer(t, "testdata/traefik-metrics.txt")

	s := metrics.NewScraper(http.DefaultClient)

	got, err := s.Scrape(context.Background(), metrics.ParserTraefik, []string{srvURL}, metrics.ScrapeState{
		ServiceIngresses:     map[string][]string{"whoami@default": {"myIngress@default.ingress.networking.k8s.io"}, "whoami2@default": {"myIngress@default.ingress.networking.k8s.io"}},
		ServiceIngressRoutes: map[string][]string{"whoami3@default": {"myIngressRoute@default.ingressroute.traefik.containo.us"}},
		TraefikServiceNames:  map[string]string{"default-whoami-80": "whoami@default", "default-whoami2-80": "whoami2@default", "default-whoami-sdfsdfsdsd": "whoami@default", "default-whoami3-80": "whoami3@default"},
	})
	require.NoError(t, err)

	require.Len(t, got, 10)

	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress@default.ingress.networking.k8s.io", Service: "whoami@default", Value: 12})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress@default.ingress.networking.k8s.io", Service: "whoami@default", Value: 14})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestClientErrors, Ingress: "myIngress@default.ingress.networking.k8s.io", Service: "whoami@default", Value: 14})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress@default.ingress.networking.k8s.io", Service: "whoami2@default", Value: 16})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestErrors, Ingress: "myIngress@default.ingress.networking.k8s.io", Service: "whoami2@default", Value: 16})
	assert.Contains(t, got, &metrics.Histogram{
		Name:    metrics.MetricRequestDuration,
		Ingress: "myIngress@default.ingress.networking.k8s.io",
		Service: "whoami@default",
		Sum:     0.021072671000000005,
		Count:   12,
	})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngressRoute@default.ingressroute.traefik.containo.us", Service: "whoami3@default", Value: 15})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngressRoute@default.ingressroute.traefik.containo.us", Service: "whoami3@default", Value: 17})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestErrors, Ingress: "myIngressRoute@default.ingressroute.traefik.containo.us", Service: "whoami3@default", Value: 15})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestErrors, Ingress: "myIngressRoute@default.ingressroute.traefik.containo.us", Service: "whoami3@default", Value: 17})
}

func TestScraper_ScrapeHAProxy(t *testing.T) {
	srvURL := startServer(t, "testdata/haproxy-metrics.txt")

	s := metrics.NewScraper(http.DefaultClient)

	got, err := s.Scrape(context.Background(), metrics.ParserHAProxy, []string{srvURL}, metrics.ScrapeState{
		ServiceIngresses:     map[string][]string{"whoami@default": {"myIngress@default.ingress.networking.k8s.io"}, "whoami2@default": {"myIngress@default.ingress.networking.k8s.io"}},
		ServiceIngressRoutes: nil,
		TraefikServiceNames:  nil,
	})
	require.NoError(t, err)

	require.Len(t, got, 6)

	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress@default.ingress.networking.k8s.io", Service: "whoami@default", Value: 12})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress@default.ingress.networking.k8s.io", Service: "whoami@default", Value: 14})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestClientErrors, Ingress: "myIngress@default.ingress.networking.k8s.io", Service: "whoami@default", Value: 14})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress@default.ingress.networking.k8s.io", Service: "whoami2@default", Value: 16})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestErrors, Ingress: "myIngress@default.ingress.networking.k8s.io", Service: "whoami2@default", Value: 16})
	assert.Contains(t, got, &metrics.Histogram{
		Name:     metrics.MetricRequestDuration,
		Relative: true,
		Ingress:  "myIngress@default.ingress.networking.k8s.io",
		Service:  "whoami@default",
		Sum:      1.263616,
		Count:    1024,
	})
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
