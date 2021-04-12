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
	"github.com/traefik/neo-agent/pkg/metrics"
)

func TestScraper_ScrapeNginx(t *testing.T) {
	srvURL := startServer(t, "testdata/nginx-metrics.txt")

	s := metrics.NewScraper(http.DefaultClient)

	got, err := s.Scrape(context.Background(), metrics.ParserNginx, []string{srvURL}, nil)
	require.NoError(t, err)

	require.Len(t, got, 12)

	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "default/whoami", Value: 20})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "default/whoami", Value: 19})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestClientErrors, Ingress: "default/whoami", Value: 19})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "default/whoami", Value: 18})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestErrors, Ingress: "default/whoami", Value: 18})
	assert.Contains(t, got, &metrics.Histogram{
		Name:    metrics.MetricRequestDuration,
		Ingress: "default/whoami",
		Sum:     0.030000000000000006,
		Count:   20,
	})

	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "default/whoami", Service: "default/whoami", Value: 20})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "default/whoami", Service: "default/whoami", Value: 19})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestClientErrors, Ingress: "default/whoami", Service: "default/whoami", Value: 19})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "default/whoami", Service: "default/whoami2", Value: 18})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestErrors, Ingress: "default/whoami", Service: "default/whoami2", Value: 18})
	assert.Contains(t, got, &metrics.Histogram{
		Name:    metrics.MetricRequestDuration,
		Ingress: "default/whoami",
		Service: "default/whoami",
		Sum:     0.030000000000000006,
		Count:   20,
	})
}

func TestScraper_ScrapeTraefik(t *testing.T) {
	srvURL := startServer(t, "testdata/traefik-metrics.txt")

	s := metrics.NewScraper(http.DefaultClient)

	got, err := s.Scrape(context.Background(), metrics.ParserTraefik, []string{srvURL}, map[string][]string{
		"myIngress": {"default/whoami", "default/whoami2"},
	})
	require.NoError(t, err)

	require.Len(t, got, 6)

	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress", Service: "default/whoami", Value: 12})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress", Service: "default/whoami", Value: 14})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestClientErrors, Ingress: "myIngress", Service: "default/whoami", Value: 14})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress", Service: "default/whoami2", Value: 16})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestErrors, Ingress: "myIngress", Service: "default/whoami2", Value: 16})
	assert.Contains(t, got, &metrics.Histogram{
		Name:    metrics.MetricRequestDuration,
		Ingress: "myIngress",
		Service: "default/whoami",
		Sum:     0.021072671000000005,
		Count:   12,
	})
}

func TestScraper_ScrapeHAProxy(t *testing.T) {
	srvURL := startServer(t, "testdata/haproxy-metrics.txt")

	s := metrics.NewScraper(http.DefaultClient)

	got, err := s.Scrape(context.Background(), metrics.ParserHAProxy, []string{srvURL}, map[string][]string{
		"myIngress": {"default/whoami", "default/whoami2"},
	})
	require.NoError(t, err)

	require.Len(t, got, 6)

	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress", Service: "default/whoami", Value: 12})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress", Service: "default/whoami", Value: 14})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestClientErrors, Ingress: "myIngress", Service: "default/whoami", Value: 14})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress", Service: "default/whoami2", Value: 16})
	assert.Contains(t, got, &metrics.Counter{Name: metrics.MetricRequestErrors, Ingress: "myIngress", Service: "default/whoami2", Value: 16})
	assert.Contains(t, got, &metrics.Histogram{
		Name:     metrics.MetricRequestDuration,
		Relative: true,
		Ingress:  "myIngress",
		Service:  "default/whoami",
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
