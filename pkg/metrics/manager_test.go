package metrics_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/hamba/avro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/neo-agent/pkg/metrics"
	"github.com/traefik/neo-agent/pkg/metrics/protocol"
)

func TestManager_SendsMetrics(t *testing.T) {
	metricsData, err := os.ReadFile("./testdata/nginx-metrics.txt")
	require.NoError(t, err)

	sentCh := make(chan struct{})
	srv := platformServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(sentCh)
	}))

	podSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(metricsData)
	}))
	t.Cleanup(func() { podSrv.Close() })

	client, err := metrics.NewClient(http.DefaultClient, srv.URL, "test-token")
	require.NoError(t, err)

	store := metrics.NewStore()

	scraper := metrics.NewScraper(http.DefaultClient)

	m := metrics.NewManager(client, store, scraper)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
	})

	go func() {
		err = m.Run(ctx, time.Second, metrics.ParserNginx, "test", []string{podSrv.URL})

		assert.NoError(t, err)
	}()

	select {
	case <-sentCh:
		return
	case <-time.After(30 * time.Second):
		t.Errorf("metrics send expected but not received")
	}
}

func platformServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	schema, err := avro.Parse(protocol.ConfigV1Schema)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		cfg := metrics.Config{
			Interval: time.Second,
			Tables:   []string{"1m", "10m"},
		}

		err := avro.NewEncoderForSchema(schema, w).Encode(cfg)
		require.NoError(t, err)
	})

	if handler != nil {
		mux.Handle("/", handler)
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(func() { srv.Close() })

	return srv
}
