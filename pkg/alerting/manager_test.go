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
)

const (
	alertRefreshInterval   = 10 * time.Minute
	alertSchedulerInterval = time.Minute
)

// alertSchedulerInterval is the interval at which the scheduler runs rule checks.

func TestManager_refreshRules(t *testing.T) {
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
		{
			ID:      "234",
			Service: "svc@ns",
			Threshold: &Threshold{
				Metric: "averageResponseTime",
				Condition: ThresholdCondition{
					Above: true,
					Value: 100,
				},
				Occurrence: 2,
				TimeRange:  1 * time.Hour,
			},
		},
		{
			ID:      "234",
			Ingress: "ing@ns",
			Threshold: &Threshold{
				Metric: "requestClientErrorsPerSecond",
				Condition: ThresholdCondition{
					Above: true,
					Value: 5,
				},
				Occurrence: 3,
				TimeRange:  10 * time.Minute,
			},
		},
	}

	backend := newBackendMock(t)
	backend.OnGetRules().TypedReturns(rules, nil).Once()

	mgr := NewManager(backend, nil, alertRefreshInterval, alertSchedulerInterval)

	err := mgr.refreshRules(context.Background())
	require.NoError(t, err)

	assert.Equal(t, rules, mgr.rules)
}

func TestManager_refreshRules_handlesClientError(t *testing.T) {
	backend := newBackendMock(t)
	backend.
		OnGetRules().
		TypedReturns(nil, errors.New("boom")).
		Once()

	mgr := NewManager(backend, nil, alertRefreshInterval, alertSchedulerInterval)

	err := mgr.refreshRules(context.Background())
	require.Error(t, err)
}

func TestManager_checkAlerts(t *testing.T) {
	tests := []struct {
		desc     string
		rules    []Rule
		setup    func(t *testing.T, rules []Rule) (map[string]Processor, *backendMock)
		on       func(rules []Rule, processor map[string]Processor, backend *backendMock)
		expected require.ErrorAssertionFunc
	}{
		{
			desc: "one threshold rule, alert triggered and sent",
			rules: []Rule{
				{
					ID:      "123",
					Ingress: "web@myns",
					Service: "whoami@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
			},
			setup: func(t *testing.T, rules []Rule) (map[string]Processor, *backendMock) {
				t.Helper()

				alert := Alert{
					RuleID:  rules[0].ID,
					Ingress: rules[0].Ingress,
					Service: rules[0].Service,
					Points: []Point{
						{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
						{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
					},
					Logs:      []byte("logs"),
					Threshold: rules[0].Threshold,
				}

				processor := newProcessorMock(t).
					OnProcess(&rules[0]).TypedReturns(&alert, nil).Once()

				backend := newBackendMock(t).
					OnPreflightAlerts([]Alert{alert}).TypedReturns([]Alert{alert}, nil).Once().
					OnSendAlerts([]Alert{alert}).TypedReturns(nil).Once()

				return map[string]Processor{ThresholdType: processor.Parent}, backend.Parent
			},
			expected: require.NoError,
		},
		{
			desc: "one threshold rule, alert triggered but don't need to be sent",
			rules: []Rule{
				{
					ID:      "123",
					Ingress: "web@myns",
					Service: "whoami@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
			},
			setup: func(t *testing.T, rules []Rule) (map[string]Processor, *backendMock) {
				t.Helper()

				alert := Alert{
					RuleID:  rules[0].ID,
					Ingress: rules[0].Ingress,
					Service: rules[0].Service,
					Points: []Point{
						{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
						{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
					},
					Logs:      []byte("logs"),
					Threshold: rules[0].Threshold,
				}

				processor := newProcessorMock(t).
					OnProcess(&rules[0]).TypedReturns(&alert, nil).Once()

				backend := newBackendMock(t).
					OnPreflightAlerts([]Alert{alert}).TypedReturns([]Alert{}, nil).Once()

				return map[string]Processor{ThresholdType: processor.Parent}, backend.Parent
			},
			expected: require.NoError,
		},
		{
			desc: "one threshold rule, alert is not triggered",
			rules: []Rule{
				{
					ID:      "123",
					Ingress: "web@myns",
					Service: "whoami@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
			},
			setup: func(t *testing.T, rules []Rule) (map[string]Processor, *backendMock) {
				t.Helper()

				processor := newProcessorMock(t).
					OnProcess(&rules[0]).TypedReturns(nil, nil).Once()

				var alerts []Alert

				backend := newBackendMock(t).
					OnPreflightAlerts(alerts).TypedReturns([]Alert{}, nil).Once()

				return map[string]Processor{ThresholdType: processor.Parent}, backend.Parent
			},
			expected: require.NoError,
		},
		{
			desc: "failed to send alert",
			rules: []Rule{
				{
					ID:      "123",
					Ingress: "web@myns",
					Service: "whoami@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
			},
			setup: func(t *testing.T, rules []Rule) (map[string]Processor, *backendMock) {
				t.Helper()

				alert := Alert{
					RuleID:  rules[0].ID,
					Ingress: rules[0].Ingress,
					Service: rules[0].Service,
					Points: []Point{
						{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
						{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
					},
					Logs:      []byte("logs"),
					Threshold: rules[0].Threshold,
				}

				processor := newProcessorMock(t).
					OnProcess(&rules[0]).TypedReturns(&alert, nil).Once()

				backend := newBackendMock(t).
					OnPreflightAlerts([]Alert{alert}).TypedReturns([]Alert{alert}, nil).Once().
					OnSendAlerts([]Alert{alert}).TypedReturns(errors.New("boom")).Once()

				return map[string]Processor{ThresholdType: processor.Parent}, backend.Parent
			},
			expected: require.Error,
		},
		{
			desc: "failed to check alert, no alert has to be sent",
			rules: []Rule{
				{
					ID:      "123",
					Ingress: "web@myns",
					Service: "whoami@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
			},
			setup: func(t *testing.T, rules []Rule) (map[string]Processor, *backendMock) {
				t.Helper()

				alert := Alert{
					RuleID:  rules[0].ID,
					Ingress: rules[0].Ingress,
					Service: rules[0].Service,
					Points: []Point{
						{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
						{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
					},
					Logs:      []byte("logs"),
					Threshold: rules[0].Threshold,
				}

				processor := newProcessorMock(t).
					OnProcess(&rules[0]).TypedReturns(&alert, nil).Once()

				backend := newBackendMock(t).
					OnPreflightAlerts([]Alert{alert}).TypedReturns(nil, errors.New("boom")).Once()

				return map[string]Processor{ThresholdType: processor.Parent}, backend.Parent
			},
			expected: require.Error,
		},
		{
			desc: "failed to process rule",
			rules: []Rule{
				{
					ID:      "123",
					Ingress: "web@myns",
					Service: "whoami@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
			},
			setup: func(t *testing.T, rules []Rule) (map[string]Processor, *backendMock) {
				t.Helper()

				processor := newProcessorMock(t).
					OnProcess(&rules[0]).TypedReturns(nil, errors.New("boom")).Once()

				var alerts []Alert

				backend := newBackendMock(t).
					OnPreflightAlerts(alerts).TypedReturns([]Alert{}, nil).Once()

				return map[string]Processor{ThresholdType: processor.Parent}, backend.Parent
			},
			expected: require.NoError,
		},
		{
			desc: "two threshold rule, two alerts triggered and sent",
			rules: []Rule{
				{
					ID:      "123",
					Ingress: "web@myns",
					Service: "whoami@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
				{
					ID:      "234",
					Ingress: "web@myns",
					Service: "api@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
			},
			setup: func(t *testing.T, rules []Rule) (map[string]Processor, *backendMock) {
				t.Helper()

				alerts := []Alert{
					{
						RuleID:  rules[0].ID,
						Ingress: rules[0].Ingress,
						Service: rules[0].Service,
						Points: []Point{
							{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
							{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
						},
						Logs:      []byte("logs 1"),
						Threshold: rules[0].Threshold,
					},
					{
						RuleID:  rules[1].ID,
						Ingress: rules[1].Ingress,
						Service: rules[1].Service,
						Points: []Point{
							{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
							{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
						},
						Logs:      []byte("logs 2"),
						Threshold: rules[1].Threshold,
					},
				}

				processor := newProcessorMock(t).
					OnProcess(&rules[0]).TypedReturns(&alerts[0], nil).Once().
					OnProcess(&rules[1]).TypedReturns(&alerts[1], nil).Once()

				backend := newBackendMock(t).
					OnPreflightAlerts(alerts).TypedReturns(alerts, nil).Once().
					OnSendAlerts(alerts).TypedReturns(nil).Once()

				return map[string]Processor{ThresholdType: processor.Parent}, backend.Parent
			},
			expected: require.NoError,
		},
		{
			desc: "two threshold rule, one alert triggered and sent",
			rules: []Rule{
				{
					ID:      "123",
					Ingress: "web@myns",
					Service: "whoami@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
				{
					ID:      "234",
					Ingress: "web@myns",
					Service: "api@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
			},
			setup: func(t *testing.T, rules []Rule) (map[string]Processor, *backendMock) {
				t.Helper()

				alert := Alert{
					RuleID:  rules[0].ID,
					Ingress: rules[0].Ingress,
					Service: rules[0].Service,
					Points: []Point{
						{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
						{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
					},
					Logs:      []byte("logs"),
					Threshold: rules[0].Threshold,
				}

				processor := newProcessorMock(t).
					OnProcess(&rules[0]).TypedReturns(&alert, nil).Once().
					OnProcess(&rules[1]).TypedReturns(nil, nil).Once()

				backend := newBackendMock(t).
					OnPreflightAlerts([]Alert{alert}).TypedReturns([]Alert{alert}, nil).Once().
					OnSendAlerts([]Alert{alert}).TypedReturns(nil).Once()

				return map[string]Processor{ThresholdType: processor.Parent}, backend.Parent
			},
			expected: require.NoError,
		},
		{
			desc: "two threshold rule, only one needs to be sent",
			rules: []Rule{
				{
					ID:      "123",
					Ingress: "web@myns",
					Service: "whoami@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
				{
					ID:      "234",
					Ingress: "web@myns",
					Service: "api@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
			},
			setup: func(t *testing.T, rules []Rule) (map[string]Processor, *backendMock) {
				t.Helper()

				alerts := []Alert{
					{
						RuleID:  rules[0].ID,
						Ingress: rules[0].Ingress,
						Service: rules[0].Service,
						Points: []Point{
							{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
							{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
						},
						Logs:      []byte("logs 1"),
						Threshold: rules[0].Threshold,
					},
					{
						RuleID:  rules[1].ID,
						Ingress: rules[1].Ingress,
						Service: rules[1].Service,
						Points: []Point{
							{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
							{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
						},
						Logs:      []byte("logs 2"),
						Threshold: rules[1].Threshold,
					},
				}

				processor := newProcessorMock(t).
					OnProcess(&rules[0]).TypedReturns(&alerts[0], nil).Once().
					OnProcess(&rules[1]).TypedReturns(&alerts[1], nil).Once()

				backend := newBackendMock(t).
					OnPreflightAlerts(alerts).TypedReturns([]Alert{alerts[0]}, nil).Once().
					OnSendAlerts([]Alert{alerts[0]}).TypedReturns(nil).Once()

				return map[string]Processor{ThresholdType: processor.Parent}, backend.Parent
			},
			expected: require.NoError,
		},
		{
			desc: "two threshold rule, one failed to be processed the other is sent",
			rules: []Rule{
				{
					ID:      "123",
					Ingress: "web@myns",
					Service: "whoami@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
				{
					ID:      "234",
					Ingress: "web@myns",
					Service: "api@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
			},
			setup: func(t *testing.T, rules []Rule) (map[string]Processor, *backendMock) {
				t.Helper()

				alert := Alert{
					RuleID:  rules[1].ID,
					Ingress: rules[1].Ingress,
					Service: rules[1].Service,
					Points: []Point{
						{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
						{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
					},
					Logs:      []byte("logs"),
					Threshold: rules[1].Threshold,
				}

				processor := newProcessorMock(t).
					OnProcess(&rules[0]).TypedReturns(nil, errors.New("boom")).Once().
					OnProcess(&rules[1]).TypedReturns(&alert, nil).Once()

				backend := newBackendMock(t).
					OnPreflightAlerts([]Alert{alert}).TypedReturns([]Alert{alert}, nil).Once().
					OnSendAlerts([]Alert{alert}).TypedReturns(nil).Once()

				return map[string]Processor{ThresholdType: processor.Parent}, backend.Parent
			},
			expected: require.NoError,
		},
		{
			desc: "one rule type is unknown, the other is sent",
			rules: []Rule{
				{
					ID:      "123",
					Ingress: "web@myns",
					Service: "whoami@myns",
				},
				{
					ID:      "234",
					Ingress: "web@myns",
					Service: "api@myns",
					Threshold: &Threshold{
						Metric: "requestsPerSecond",
						Condition: ThresholdCondition{
							Above: true,
							Value: 100,
						},
						Occurrence: 2,
						TimeRange:  time.Hour,
					},
				},
			},
			setup: func(t *testing.T, rules []Rule) (map[string]Processor, *backendMock) {
				t.Helper()

				alert := Alert{
					RuleID:  rules[1].ID,
					Ingress: rules[1].Ingress,
					Service: rules[1].Service,
					Points: []Point{
						{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
						{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
					},
					Logs:      []byte("logs"),
					Threshold: rules[1].Threshold,
				}

				processor := newProcessorMock(t).
					OnProcess(&rules[1]).TypedReturns(&alert, nil).Once()

				backend := newBackendMock(t).
					OnPreflightAlerts([]Alert{alert}).TypedReturns([]Alert{alert}, nil).Once().
					OnSendAlerts([]Alert{alert}).TypedReturns(nil).Once()

				return map[string]Processor{ThresholdType: processor.Parent}, backend.Parent
			},
			expected: require.NoError,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			processors, backend := test.setup(t, test.rules)

			mgr := NewManager(backend, processors, time.Second, time.Second)
			mgr.rules = test.rules

			err := mgr.checkAlerts(context.Background())
			test.expected(t, err)
		})
	}
}
