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

package alerting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent-kubernetes/pkg/metrics"
)

func TestThresholdProcessor_Process(t *testing.T) {
	now := time.Date(2021, 1, 1, 8, 21, 43, 0, time.UTC)
	serviceLogs := []byte("here are my logs")
	serviceCompressedLogs := []byte{
		0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xca, 0x48, 0x2d, 0x4a, 0x55, 0x48,
		0x2c, 0x4a, 0x55, 0xc8, 0xad, 0x54, 0xc8, 0xc9, 0x4f, 0x2f, 0x06, 0x04, 0x00, 0x00, 0xff, 0xff,
		0x20, 0x9a, 0x9e, 0x8d, 0x10, 0x00, 0x00, 0x00,
	}

	type expected struct {
		alert      *Alert
		requireErr require.ErrorAssertionFunc
	}

	tests := []struct {
		desc           string
		rule           *Rule
		dataPointsMock func(testing.TB) *dataPointsFinderMock
		logsMock       func(testing.TB) *logProviderMock
		expected       expected
	}{
		{
			desc: "No alert: Rule with no service and ingress",
			rule: &Rule{
				ID: "rule-1",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: true, Value: 100},
					Occurrence: 1,
					TimeRange:  5 * time.Minute,
				},
			},
			dataPointsMock: newDataPointsFinderMock,
			logsMock:       newLogProviderMock,
			expected:       expected{requireErr: require.Error},
		},
		{
			desc: "Alert: Rule with service needs 1 occurrence: rule matches 1 data point",
			rule: &Rule{
				ID:      "rule-1",
				Service: "service-1@myns",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: true, Value: 100},
					Occurrence: 1,
					TimeRange:  5 * time.Minute,
				},
			},
			dataPointsMock: func(tb testing.TB) *dataPointsFinderMock {
				tb.Helper()

				view := newDataPointsFinderMock(tb)
				view.
					OnFindByService("1m", "service-1@myns",
						time.Date(2021, 1, 1, 8, 15, 0, 0, time.UTC),
						time.Date(2021, 1, 1, 8, 20, 0, 0, time.UTC),
					).
					TypedReturns(metrics.DataPoints{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), ReqPerS: 90},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), ReqPerS: 120},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), ReqPerS: 80},
					}).
					Once()

				return view
			},
			logsMock: func(tb testing.TB) *logProviderMock {
				tb.Helper()

				logs := newLogProviderMock(tb)
				logs.
					OnGetServiceLogs("myns", "service-1", logLines, logMaxLineLength).
					TypedReturns(serviceLogs, nil).
					Once()

				return logs
			},
			expected: expected{
				requireErr: require.NoError,
				alert: &Alert{
					RuleID:  "rule-1",
					Ingress: "",
					Service: "service-1@myns",
					Points: []Point{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), Value: 90},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), Value: 120},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), Value: 80},
					},
					Logs: serviceCompressedLogs,
					Threshold: &Threshold{
						Metric:     "requestsPerSecond",
						Condition:  ThresholdCondition{Above: true, Value: 100},
						Occurrence: 1,
						TimeRange:  5 * time.Minute,
					},
				},
			},
		},
		{
			desc: "Alert: Rule with service needs 1 occurrence: rule matches 2 data point",
			rule: &Rule{
				ID:      "rule-1",
				Service: "service-1@myns",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: true, Value: 100},
					Occurrence: 1,
					TimeRange:  5 * time.Minute,
				},
			},
			dataPointsMock: func(tb testing.TB) *dataPointsFinderMock {
				tb.Helper()

				view := newDataPointsFinderMock(tb)
				view.
					OnFindByService("1m", "service-1@myns",
						time.Date(2021, 1, 1, 8, 15, 0, 0, time.UTC),
						time.Date(2021, 1, 1, 8, 20, 0, 0, time.UTC),
					).
					TypedReturns(metrics.DataPoints{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), ReqPerS: 90},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), ReqPerS: 101},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), ReqPerS: 110},
					}).
					Once()

				return view
			},
			logsMock: func(tb testing.TB) *logProviderMock {
				tb.Helper()

				logs := newLogProviderMock(tb)
				logs.
					OnGetServiceLogs("myns", "service-1", logLines, logMaxLineLength).
					TypedReturns(serviceLogs, nil).
					Once()

				return logs
			},
			expected: expected{
				requireErr: require.NoError,
				alert: &Alert{
					RuleID:  "rule-1",
					Ingress: "",
					Service: "service-1@myns",
					Points: []Point{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), Value: 90},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), Value: 101},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), Value: 110},
					},
					Logs: serviceCompressedLogs,
					Threshold: &Threshold{
						Metric:     "requestsPerSecond",
						Condition:  ThresholdCondition{Above: true, Value: 100},
						Occurrence: 1,
						TimeRange:  5 * time.Minute,
					},
				},
			},
		},
		{
			desc: "No Alert: Rule with service: rule matches 0 data point",
			rule: &Rule{
				ID:      "rule-1",
				Service: "service-1@myns",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: true, Value: 100},
					Occurrence: 1,
					TimeRange:  5 * time.Minute,
				},
			},
			dataPointsMock: func(tb testing.TB) *dataPointsFinderMock {
				tb.Helper()

				view := newDataPointsFinderMock(tb)
				view.
					OnFindByService("1m", "service-1@myns",
						time.Date(2021, 1, 1, 8, 15, 0, 0, time.UTC),
						time.Date(2021, 1, 1, 8, 20, 0, 0, time.UTC),
					).
					TypedReturns(metrics.DataPoints{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), ReqPerS: 90},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), ReqPerS: 90},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), ReqPerS: 80},
					}).
					Once()

				return view
			},
			logsMock: newLogProviderMock,
			expected: expected{
				requireErr: require.NoError,
			},
		},
		{
			desc: "Alert: Rule with service needs 2 occurrences: rule matches 2 data point",
			rule: &Rule{
				ID:      "rule-1",
				Service: "service-1@myns",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: true, Value: 100},
					Occurrence: 2,
					TimeRange:  5 * time.Minute,
				},
			},
			dataPointsMock: func(tb testing.TB) *dataPointsFinderMock {
				tb.Helper()

				view := newDataPointsFinderMock(tb)
				view.
					OnFindByService("1m", "service-1@myns",
						time.Date(2021, 1, 1, 8, 15, 0, 0, time.UTC),
						time.Date(2021, 1, 1, 8, 20, 0, 0, time.UTC),
					).
					TypedReturns(metrics.DataPoints{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), ReqPerS: 90},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), ReqPerS: 101},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), ReqPerS: 110},
					}).
					Once()

				return view
			},
			logsMock: func(tb testing.TB) *logProviderMock {
				tb.Helper()

				logs := newLogProviderMock(tb)
				logs.
					OnGetServiceLogs("myns", "service-1", logLines, logMaxLineLength).
					TypedReturns(serviceLogs, nil).
					Once()

				return logs
			},
			expected: expected{
				requireErr: require.NoError,
				alert: &Alert{
					RuleID:  "rule-1",
					Ingress: "",
					Service: "service-1@myns",
					Points: []Point{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), Value: 90},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), Value: 101},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), Value: 110},
					},
					Logs: serviceCompressedLogs,
					Threshold: &Threshold{
						Metric:     "requestsPerSecond",
						Condition:  ThresholdCondition{Above: true, Value: 100},
						Occurrence: 2,
						TimeRange:  5 * time.Minute,
					},
				},
			},
		},
		{
			desc: "No Alert: Rule with service needs 2 occurrences: rule matches 1 data point",
			rule: &Rule{
				ID:      "rule-1",
				Service: "service-1@myns",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: true, Value: 100},
					Occurrence: 2,
					TimeRange:  5 * time.Minute,
				},
			},
			dataPointsMock: func(tb testing.TB) *dataPointsFinderMock {
				tb.Helper()

				view := newDataPointsFinderMock(tb)
				view.
					OnFindByService("1m", "service-1@myns",
						time.Date(2021, 1, 1, 8, 15, 0, 0, time.UTC),
						time.Date(2021, 1, 1, 8, 20, 0, 0, time.UTC),
					).
					TypedReturns(metrics.DataPoints{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), ReqPerS: 90},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), ReqPerS: 110},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), ReqPerS: 80},
					}).
					Once()

				return view
			},
			logsMock: newLogProviderMock,
			expected: expected{
				requireErr: require.NoError,
			},
		},
		{
			desc: "Alert: Rule with service needs 1 occurrences (below): rule matches 1 data point",
			rule: &Rule{
				ID:      "rule-1",
				Service: "service-1@myns",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: false, Value: 100},
					Occurrence: 1,
					TimeRange:  5 * time.Minute,
				},
			},
			dataPointsMock: func(tb testing.TB) *dataPointsFinderMock {
				tb.Helper()

				view := newDataPointsFinderMock(tb)
				view.
					OnFindByService("1m", "service-1@myns",
						time.Date(2021, 1, 1, 8, 15, 0, 0, time.UTC),
						time.Date(2021, 1, 1, 8, 20, 0, 0, time.UTC),
					).
					TypedReturns(metrics.DataPoints{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), ReqPerS: 110},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), ReqPerS: 80},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), ReqPerS: 110},
					}).
					Once()

				return view
			},
			logsMock: func(tb testing.TB) *logProviderMock {
				tb.Helper()

				logs := newLogProviderMock(tb)
				logs.
					OnGetServiceLogs("myns", "service-1", logLines, logMaxLineLength).
					TypedReturns(serviceLogs, nil).
					Once()

				return logs
			},
			expected: expected{
				requireErr: require.NoError,
				alert: &Alert{
					RuleID:  "rule-1",
					Ingress: "",
					Service: "service-1@myns",
					Points: []Point{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), Value: 110},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), Value: 80},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), Value: 110},
					},
					Logs: serviceCompressedLogs,
					Threshold: &Threshold{
						Metric:     "requestsPerSecond",
						Condition:  ThresholdCondition{Above: false, Value: 100},
						Occurrence: 1,
						TimeRange:  5 * time.Minute,
					},
				},
			},
		},
		{
			desc: "No Alert: Rule with service needs 2 occurrences (below): rule matches 1 data point",
			rule: &Rule{
				ID:      "rule-1",
				Service: "service-1@myns",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: false, Value: 100},
					Occurrence: 2,
					TimeRange:  5 * time.Minute,
				},
			},
			dataPointsMock: func(tb testing.TB) *dataPointsFinderMock {
				tb.Helper()

				view := newDataPointsFinderMock(tb)
				view.
					OnFindByService("1m", "service-1@myns",
						time.Date(2021, 1, 1, 8, 15, 0, 0, time.UTC),
						time.Date(2021, 1, 1, 8, 20, 0, 0, time.UTC),
					).
					TypedReturns(metrics.DataPoints{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), ReqPerS: 110},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), ReqPerS: 80},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), ReqPerS: 110},
					}).
					Once()

				return view
			},
			logsMock: newLogProviderMock,
			expected: expected{
				requireErr: require.NoError,
			},
		},
		{
			desc: "Alert: Rule with service needs 2 occurrences (below): rule matches 2 data point",
			rule: &Rule{
				ID:      "rule-1",
				Service: "service-1@myns",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: false, Value: 100},
					Occurrence: 2,
					TimeRange:  5 * time.Minute,
				},
			},
			dataPointsMock: func(tb testing.TB) *dataPointsFinderMock {
				tb.Helper()

				view := newDataPointsFinderMock(tb)
				view.
					OnFindByService("1m", "service-1@myns",
						time.Date(2021, 1, 1, 8, 15, 0, 0, time.UTC),
						time.Date(2021, 1, 1, 8, 20, 0, 0, time.UTC),
					).
					TypedReturns(metrics.DataPoints{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), ReqPerS: 0},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), ReqPerS: 80},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), ReqPerS: 110},
					}).
					Once()

				return view
			},
			logsMock: func(tb testing.TB) *logProviderMock {
				tb.Helper()

				logs := newLogProviderMock(tb)
				logs.
					OnGetServiceLogs("myns", "service-1", logLines, logMaxLineLength).
					TypedReturns(serviceLogs, nil).
					Once()

				return logs
			},
			expected: expected{
				requireErr: require.NoError,
				alert: &Alert{
					RuleID:  "rule-1",
					Ingress: "",
					Service: "service-1@myns",
					Points: []Point{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), Value: 0},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), Value: 80},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), Value: 110},
					},
					Logs: serviceCompressedLogs,
					Threshold: &Threshold{
						Metric:     "requestsPerSecond",
						Condition:  ThresholdCondition{Above: false, Value: 100},
						Occurrence: 2,
						TimeRange:  5 * time.Minute,
					},
				},
			},
		},
		{
			desc: "Alert: Rule with service but unable to get logs",
			rule: &Rule{
				ID:      "rule-1",
				Service: "service-1@myns",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: false, Value: 100},
					Occurrence: 1,
					TimeRange:  5 * time.Minute,
				},
			},
			dataPointsMock: func(tb testing.TB) *dataPointsFinderMock {
				tb.Helper()

				view := newDataPointsFinderMock(tb)
				view.
					OnFindByService("1m", "service-1@myns",
						time.Date(2021, 1, 1, 8, 15, 0, 0, time.UTC),
						time.Date(2021, 1, 1, 8, 20, 0, 0, time.UTC),
					).
					TypedReturns(metrics.DataPoints{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), ReqPerS: 90},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), ReqPerS: 110},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), ReqPerS: 80},
					}).
					Once()

				return view
			},
			logsMock: func(tb testing.TB) *logProviderMock {
				tb.Helper()

				logs := newLogProviderMock(tb)
				logs.
					OnGetServiceLogs("myns", "service-1", logLines, logMaxLineLength).
					TypedReturns([]byte{}, errors.New("boom")).
					Once()

				return logs
			},
			expected: expected{
				requireErr: require.NoError,
				alert: &Alert{
					RuleID:  "rule-1",
					Ingress: "",
					Service: "service-1@myns",
					Points: []Point{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), Value: 90},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), Value: 110},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), Value: 80},
					},
					Threshold: &Threshold{
						Metric:     "requestsPerSecond",
						Condition:  ThresholdCondition{Above: false, Value: 100},
						Occurrence: 1,
						TimeRange:  5 * time.Minute,
					},
				},
			},
		},
		{
			desc: "Alert: Rule with ingress",
			rule: &Rule{
				ID:      "rule-1",
				Ingress: "ingress-1@myns",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: true, Value: 100},
					Occurrence: 1,
					TimeRange:  5 * time.Minute,
				},
			},
			dataPointsMock: func(tb testing.TB) *dataPointsFinderMock {
				tb.Helper()

				view := newDataPointsFinderMock(tb)
				view.
					OnFindByIngress("1m", "ingress-1@myns",
						time.Date(2021, 1, 1, 8, 15, 0, 0, time.UTC),
						time.Date(2021, 1, 1, 8, 20, 0, 0, time.UTC),
					).
					TypedReturns(metrics.DataPoints{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), ReqPerS: 90},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), ReqPerS: 120},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), ReqPerS: 80},
					}).
					Once()

				return view
			},
			logsMock: newLogProviderMock,
			expected: expected{
				requireErr: require.NoError,
				alert: &Alert{
					RuleID:  "rule-1",
					Ingress: "ingress-1@myns",
					Service: "",
					Points: []Point{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), Value: 90},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), Value: 120},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), Value: 80},
					},
					Threshold: &Threshold{
						Metric:     "requestsPerSecond",
						Condition:  ThresholdCondition{Above: true, Value: 100},
						Occurrence: 1,
						TimeRange:  5 * time.Minute,
					},
				},
			},
		},
		{
			desc: "Alert: Rule with service and ingress",
			rule: &Rule{
				ID:      "rule-1",
				Ingress: "ingress-1@myns",
				Service: "service-1@myns",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: true, Value: 100},
					Occurrence: 1,
					TimeRange:  5 * time.Minute,
				},
			},
			dataPointsMock: func(tb testing.TB) *dataPointsFinderMock {
				tb.Helper()

				view := newDataPointsFinderMock(tb)
				view.
					OnFindByIngressAndService("1m", "ingress-1@myns", "service-1@myns",
						time.Date(2021, 1, 1, 8, 15, 0, 0, time.UTC),
						time.Date(2021, 1, 1, 8, 20, 0, 0, time.UTC),
					).
					TypedReturns(metrics.DataPoints{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), ReqPerS: 90},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), ReqPerS: 120},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), ReqPerS: 80},
					}, nil).
					Once()

				return view
			},
			logsMock: func(tb testing.TB) *logProviderMock {
				tb.Helper()

				logs := newLogProviderMock(tb)
				logs.
					OnGetServiceLogs("myns", "service-1", logLines, logMaxLineLength).
					TypedReturns([]byte{}, errors.New("boom")).
					Once()

				return logs
			},
			expected: expected{
				requireErr: require.NoError,
				alert: &Alert{
					RuleID:  "rule-1",
					Ingress: "ingress-1@myns",
					Service: "service-1@myns",
					Points: []Point{
						{Timestamp: now.Add(-4 * time.Minute).Unix(), Value: 90},
						{Timestamp: now.Add(-3 * time.Minute).Unix(), Value: 120},
						{Timestamp: now.Add(-2 * time.Minute).Unix(), Value: 80},
					},
					Threshold: &Threshold{
						Metric:     "requestsPerSecond",
						Condition:  ThresholdCondition{Above: true, Value: 100},
						Occurrence: 1,
						TimeRange:  5 * time.Minute,
					},
				},
			},
		},
		{
			desc: "Alert: Rule with threshold time range > 24h: use 1d table and 24h granularity",
			rule: &Rule{
				ID:      "rule-1",
				Ingress: "ingress-1@myns",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: true, Value: 100},
					Occurrence: 1,
					TimeRange:  48 * time.Hour,
				},
			},
			dataPointsMock: func(tb testing.TB) *dataPointsFinderMock {
				tb.Helper()

				view := newDataPointsFinderMock(tb)
				view.
					OnFindByIngress("1d", "ingress-1@myns",
						time.Date(2020, 12, 29, 0, 0, 0, 0, time.UTC),
						time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC),
					).
					TypedReturns(metrics.DataPoints{
						{Timestamp: time.Date(2020, 12, 30, 0, 0, 0, 0, time.UTC).Unix(), ReqPerS: 90},
						{Timestamp: time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC).Unix(), ReqPerS: 120},
						{Timestamp: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC).Unix(), ReqPerS: 80},
					}).
					Once()

				return view
			},
			logsMock: newLogProviderMock,
			expected: expected{
				requireErr: require.NoError,
				alert: &Alert{
					RuleID:  "rule-1",
					Ingress: "ingress-1@myns",
					Service: "",
					Points: []Point{
						{Timestamp: time.Date(2020, 12, 30, 0, 0, 0, 0, time.UTC).Unix(), Value: 90},
						{Timestamp: time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC).Unix(), Value: 120},
						{Timestamp: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC).Unix(), Value: 80},
					},
					Threshold: &Threshold{
						Metric:     "requestsPerSecond",
						Condition:  ThresholdCondition{Above: true, Value: 100},
						Occurrence: 1,
						TimeRange:  48 * time.Hour,
					},
				},
			},
		},
		{
			desc: "Alert: Rule with threshold time range > 1h: use 1h table and 1h granularity",
			rule: &Rule{
				ID:      "rule-1",
				Ingress: "ingress-1@myns",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: true, Value: 100},
					Occurrence: 1,
					TimeRange:  10 * time.Hour,
				},
			},
			dataPointsMock: func(tb testing.TB) *dataPointsFinderMock {
				tb.Helper()

				view := newDataPointsFinderMock(tb)
				view.
					OnFindByIngress("1h", "ingress-1@myns",
						time.Date(2020, 12, 31, 21, 0, 0, 0, time.UTC),
						time.Date(2021, 1, 1, 7, 0, 0, 0, time.UTC),
					).
					TypedReturns(metrics.DataPoints{
						{Timestamp: time.Date(2021, 1, 1, 2, 0, 0, 0, time.UTC).Unix(), ReqPerS: 120},
					}).
					Once()

				return view
			},
			logsMock: newLogProviderMock,
			expected: expected{
				requireErr: require.NoError,
				alert: &Alert{
					RuleID:  "rule-1",
					Ingress: "ingress-1@myns",
					Service: "",
					Points: []Point{
						{Timestamp: time.Date(2021, 1, 1, 2, 0, 0, 0, time.UTC).Unix(), Value: 120},
					},
					Threshold: &Threshold{
						Metric:     "requestsPerSecond",
						Condition:  ThresholdCondition{Above: true, Value: 100},
						Occurrence: 1,
						TimeRange:  10 * time.Hour,
					},
				},
			},
		},
		{
			desc: "Alert: Rule with threshold time range > 10m: use 10m table and 10m granularity",
			rule: &Rule{
				ID:      "rule-1",
				Ingress: "ingress-1@myns",
				Threshold: &Threshold{
					Metric:     "requestsPerSecond",
					Condition:  ThresholdCondition{Above: true, Value: 100},
					Occurrence: 1,
					TimeRange:  30 * time.Minute,
				},
			},
			dataPointsMock: func(tb testing.TB) *dataPointsFinderMock {
				tb.Helper()

				view := newDataPointsFinderMock(tb)
				view.
					OnFindByIngress("10m", "ingress-1@myns",
						time.Date(2021, 1, 1, 7, 40, 0, 0, time.UTC),
						time.Date(2021, 1, 1, 8, 10, 0, 0, time.UTC),
					).
					TypedReturns(metrics.DataPoints{
						{Timestamp: time.Date(2021, 1, 1, 7, 50, 0, 0, time.UTC).Unix(), ReqPerS: 120},
					}).
					Once()

				return view
			},
			logsMock: newLogProviderMock,
			expected: expected{
				requireErr: require.NoError,
				alert: &Alert{
					RuleID:  "rule-1",
					Ingress: "ingress-1@myns",
					Service: "",
					Points: []Point{
						{Timestamp: time.Date(2021, 1, 1, 7, 50, 0, 0, time.UTC).Unix(), Value: 120},
					},
					Threshold: &Threshold{
						Metric:     "requestsPerSecond",
						Condition:  ThresholdCondition{Above: true, Value: 100},
						Occurrence: 1,
						TimeRange:  30 * time.Minute,
					},
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			logs := test.logsMock(t)
			view := test.dataPointsMock(t)

			threshProc := NewThresholdProcessor(view, logs)
			threshProc.nowFunc = func() time.Time { return now }

			alert, err := threshProc.Process(context.Background(), test.rule)
			test.expected.requireErr(t, err)

			assert.Equal(t, test.expected.alert, alert)
		})
	}
}

func TestGetValue(t *testing.T) {
	type expected struct {
		value float64
		err   bool
	}

	tests := []struct {
		desc     string
		metric   string
		point    metrics.DataPoint
		expected expected
	}{
		{
			desc:     "with requests per second metric",
			metric:   "requestsPerSecond",
			point:    metrics.DataPoint{ReqPerS: 100},
			expected: expected{value: 100},
		},
		{
			desc:     "with request errors per second metric",
			metric:   "requestErrorsPerSecond",
			point:    metrics.DataPoint{RequestErrPerS: 100},
			expected: expected{value: 100},
		},
		{
			desc:     "with request client errors per second metric",
			metric:   "requestClientErrorsPerSecond",
			point:    metrics.DataPoint{RequestClientErrPerS: 100},
			expected: expected{value: 100},
		},
		{
			desc:     "with average response time metric",
			metric:   "averageResponseTime",
			point:    metrics.DataPoint{AvgResponseTime: 100},
			expected: expected{value: 100},
		},
		{
			desc:   "with unknown metric",
			metric: "requestsPerPotatoes",
			point: metrics.DataPoint{
				Timestamp:               1,
				ReqPerS:                 2,
				RequestErrPerS:          3,
				RequestErrPercent:       4,
				RequestClientErrPerS:    5,
				RequestClientErrPercent: 6,
				AvgResponseTime:         7,
				Seconds:                 8,
				Requests:                9,
				RequestErrs:             10,
				RequestClientErrs:       11,
				ResponseTimeSum:         12,
				ResponseTimeCount:       13,
			},
			expected: expected{err: true},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			value, err := getValue(test.metric, test.point)
			if test.expected.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, test.expected.value, value)
		})
	}
}
