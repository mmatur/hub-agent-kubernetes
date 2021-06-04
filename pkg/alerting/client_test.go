package alerting_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent/pkg/alerting"
)

func TestClient_GetRules(t *testing.T) {
	want := []alerting.Rule{
		{
			ID:      "123",
			Ingress: "ing",
			Service: "svc",
			Threshold: &alerting.Threshold{
				Metric: "requestsPerSecond",
				Condition: alerting.ThresholdCondition{
					Above: true,
					Value: 10,
				},
				Occurrence: 3,
				TimeRange:  10 * time.Minute,
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/rules", r.URL.Path)
		assert.Equal(t, "Bearer some_test_token", r.Header.Get("Authorization"))

		err := json.NewEncoder(w).Encode(want)
		require.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	client, err := alerting.NewClient(http.DefaultClient, srv.URL, "some_test_token")
	require.NoError(t, err)

	got, err := client.GetRules(context.Background())
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

func TestClient_SendAlerts(t *testing.T) {
	data := []alerting.Alert{
		{
			RuleID:  "123",
			Ingress: "ing",
			Service: "svc",
			Points: []alerting.Point{
				{
					Timestamp: time.Now().Unix(),
					Value:     42,
				},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/notify", r.URL.Path)
		assert.Equal(t, "Bearer some_test_token", r.Header.Get("Authorization"))

		var alerts []alerting.Alert
		err := json.NewDecoder(r.Body).Decode(&alerts)
		require.NoError(t, err)
		assert.Equal(t, data, alerts)
	}))
	t.Cleanup(srv.Close)

	client, err := alerting.NewClient(http.DefaultClient, srv.URL, "some_test_token")
	require.NoError(t, err)

	err = client.Send(context.Background(), data)
	assert.NoError(t, err)
}
