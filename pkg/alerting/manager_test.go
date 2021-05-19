package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/neo-agent/pkg/metrics"
)

func TestManager_SendsAlerts(t *testing.T) {
	sentCh := make(chan struct{})
	srv := platformServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(sentCh)
	}), stateOK)

	client, err := NewClient(http.DefaultClient, srv.URL, "test-token")
	require.NoError(t, err)

	now := time.Now().UTC()

	store := mockThresholdStore{
		group: metrics.DataPointGroup{
			Ingress: "ing",
			Service: "svc",
			DataPoints: []metrics.DataPoint{
				{
					Timestamp: now.Add(-3 * time.Minute).Unix(),
					ReqPerS:   11.1,
				},
				{
					Timestamp: now.Add(-2 * time.Minute).Unix(),
					ReqPerS:   12.3,
				},
				{
					Timestamp: now.Add(-time.Minute).Unix(),
					ReqPerS:   11.7,
				},
			},
		},
	}
	mgr := NewManager(client, map[string]Processor{
		ThresholdType: NewThresholdProcessor(store),
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
	})

	go func() {
		err = mgr.Run(ctx, 10*time.Minute, time.Second)
		assert.NoError(t, err)
	}()

	select {
	case <-sentCh:
		return
	case <-time.After(5 * time.Second):
		t.Errorf("alert send expected but not received")
	}
}

func TestManager_IgnoresStateUnchangedOK(t *testing.T) {
	sentCh := make(chan struct{})
	srv := platformServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(sentCh)
	}), stateOK)

	client, err := NewClient(http.DefaultClient, srv.URL, "test-token")
	require.NoError(t, err)

	now := time.Now().UTC()

	store := mockThresholdStore{
		group: metrics.DataPointGroup{
			Ingress: "ing",
			Service: "svc",
			DataPoints: []metrics.DataPoint{
				{
					Timestamp: now.Add(-3 * time.Minute).Unix(),
					ReqPerS:   9.9,
				},
				{
					Timestamp: now.Add(-2 * time.Minute).Unix(),
					ReqPerS:   12.3,
				},
				{
					Timestamp: now.Add(-time.Minute).Unix(),
					ReqPerS:   11.7,
				},
			},
		},
	}
	mgr := NewManager(client, map[string]Processor{
		ThresholdType: NewThresholdProcessor(store),
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
	})

	go func() {
		err = mgr.Run(ctx, 10*time.Minute, time.Second)
		assert.NoError(t, err)
	}()

	select {
	case <-sentCh:
		t.Errorf("alert send not expected but not received")
	case <-time.After(5 * time.Second):
		return
	}
}

func TestManager_IgnoresStateUnchangedCritical(t *testing.T) {
	sentCh := make(chan struct{})
	srv := platformServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(sentCh)
	}), stateCritical)

	client, err := NewClient(http.DefaultClient, srv.URL, "test-token")
	require.NoError(t, err)

	now := time.Now().UTC()

	store := mockThresholdStore{
		group: metrics.DataPointGroup{
			Ingress: "ing",
			Service: "svc",
			DataPoints: []metrics.DataPoint{
				{
					Timestamp: now.Add(-3 * time.Minute).Unix(),
					ReqPerS:   11.7,
				},
				{
					Timestamp: now.Add(-2 * time.Minute).Unix(),
					ReqPerS:   12.3,
				},
				{
					Timestamp: now.Add(-time.Minute).Unix(),
					ReqPerS:   11.7,
				},
			},
		},
	}
	mgr := NewManager(client, map[string]Processor{
		ThresholdType: NewThresholdProcessor(store),
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
	})

	go func() {
		err = mgr.Run(ctx, 10*time.Minute, time.Second)
		assert.NoError(t, err)
	}()

	select {
	case <-sentCh:
		t.Errorf("alert send not expected but not received")
	case <-time.After(5 * time.Second):
		return
	}
}

func platformServer(t *testing.T, handler http.Handler, state int) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/rules", func(w http.ResponseWriter, r *http.Request) {
		rules := []Rule{
			{
				ID:      "123",
				Ingress: "ing",
				Service: "svc",
				Threshold: &Threshold{
					Metric: "requestsPerSecond",
					Condition: ThresholdCondition{
						Above: true,
						Value: 10,
					},
					Occurrence: 3,
					TimeRange:  10 * time.Minute,
				},
				State: state,
			},
		}

		err := json.NewEncoder(w).Encode(rules)
		require.NoError(t, err)
	})

	if handler != nil {
		mux.Handle("/notify", handler)
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}
