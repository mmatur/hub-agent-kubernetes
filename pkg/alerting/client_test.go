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
	"github.com/traefik/hub-agent-kubernetes/pkg/alerting"
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

	var rulesCallCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rulesCallCount++
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
	assert.Equal(t, 1, rulesCallCount)

	assert.Equal(t, want, got)
}

type descriptor struct {
	ID      int    `json:"id"`
	RuleID  string `json:"ruleId"`
	Ingress string `json:"ingress"`
	Service string `json:"service"`
}

func TestClient_PreflightAlerts(t *testing.T) {
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
		{
			RuleID:  "456",
			Ingress: "ing1",
			Service: "svc2",
			Points: []alerting.Point{
				{
					Timestamp: time.Now().Unix(),
					Value:     42,
				},
			},
		},
	}

	var preflightCallCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		preflightCallCount++
		assert.Equal(t, "/preflight", r.URL.Path)
		assert.Equal(t, "Bearer some_test_token", r.Header.Get("Authorization"))

		var alerts []descriptor
		err := json.NewDecoder(r.Body).Decode(&alerts)
		require.NoError(t, err)

		want := []descriptor{
			{
				ID:      0,
				RuleID:  "123",
				Ingress: "ing",
				Service: "svc",
			},
			{
				ID:      1,
				RuleID:  "456",
				Ingress: "ing1",
				Service: "svc2",
			},
		}
		assert.Equal(t, want, alerts)

		err = json.NewEncoder(w).Encode([]int{1})
		require.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	client, err := alerting.NewClient(http.DefaultClient, srv.URL, "some_test_token")
	require.NoError(t, err)

	got, err := client.PreflightAlerts(context.Background(), data)
	assert.NoError(t, err)
	assert.Equal(t, 1, preflightCallCount)

	want := []alerting.Alert{data[1]}
	assert.Equal(t, want, got)
}

func TestClient_PreflightAlertsHandlesBadAlertID(t *testing.T) {
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
		err := json.NewEncoder(w).Encode([]int{42})
		require.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	client, err := alerting.NewClient(http.DefaultClient, srv.URL, "some_test_token")
	require.NoError(t, err)

	_, err = client.PreflightAlerts(context.Background(), data)

	assert.Error(t, err)
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

	var notifyCallCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notifyCallCount++
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

	err = client.SendAlerts(context.Background(), data)
	assert.NoError(t, err)
	assert.Equal(t, 1, notifyCallCount)
}
