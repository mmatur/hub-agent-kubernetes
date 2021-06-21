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
	"github.com/traefik/hub-agent/pkg/metrics"
)

func TestManager_SendsAlerts(t *testing.T) {
	sentCh := make(chan struct{})
	srv := platformServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(sentCh)
	}))

	client, err := NewClient(http.DefaultClient, srv.URL, "test-token")
	require.NoError(t, err)

	now := time.Now().UTC()

	store := mockThresholdStore{
		group: metrics.DataPointGroup{
			Ingress: "ing@ns",
			Service: "svc@ns",
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
	logs := mockLogProvider{
		getServiceLogsFn: func(namespace, name string, lines, maxLen int) ([]byte, error) {
			assert.Equal(t, "svc", name)
			assert.Equal(t, "ns", namespace)

			return []byte("fake logs"), nil
		},
	}
	mgr := NewManager(client, map[string]Processor{
		ThresholdType: NewThresholdProcessor(store, logs),
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

func platformServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/rules", func(w http.ResponseWriter, r *http.Request) {
		rules := []Rule{
			{
				ID:      "123",
				Ingress: "ing@ns",
				Service: "svc@ns",
				Threshold: &Threshold{
					Metric: "requestsPerSecond",
					Condition: ThresholdCondition{
						Above: true,
						Value: 10,
					},
					Occurrence: 3,
					TimeRange:  10 * time.Minute,
				},
			},
		}

		err := json.NewEncoder(w).Encode(rules)
		require.NoError(t, err)
	})
	mux.HandleFunc("/preflight", func(w http.ResponseWriter, r *http.Request) {
		var data []struct {
			ID int `json:"id"`
		}
		err := json.NewDecoder(r.Body).Decode(&data)
		require.NoError(t, err)

		var ids []int
		for _, alert := range data {
			ids = append(ids, alert.ID)
		}

		err = json.NewEncoder(w).Encode(ids)
		require.NoError(t, err)
	})

	if handler != nil {
		mux.Handle("/notify", handler)
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}
