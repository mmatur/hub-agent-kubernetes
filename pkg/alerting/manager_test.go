package alerting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	alertRefreshInterval   = 10 * time.Minute
	alertSchedulerInterval = time.Minute
)

// alertSchedulerInterval is the interval at which the scheduler
// runs rule checks.

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

	backend := &backendMock{}
	backend.On("GetRules").Return(rules, nil).Once()

	mgr := NewManager(backend, nil, alertRefreshInterval, alertSchedulerInterval)

	err := mgr.refreshRules(context.Background())
	require.NoError(t, err)

	assert.Equal(t, rules, mgr.rules)
}

func TestManager_refreshRules_handlesClientError(t *testing.T) {
	backend := &backendMock{}
	backend.
		On("GetRules").
		Return(nil, errors.New("boom")).
		Once()

	mgr := NewManager(backend, nil, alertRefreshInterval, alertSchedulerInterval)

	err := mgr.refreshRules(context.Background())
	require.Error(t, err)
}

func TestManager_checkAlerts(t *testing.T) {
	tests := []struct {
		desc     string
		rules    []Rule
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
			on: func(rules []Rule, processors map[string]Processor, backend *backendMock) {
				rule := rules[0]
				alert := Alert{
					RuleID:  rule.ID,
					Ingress: rule.Ingress,
					Service: rule.Service,
					Points: []Point{
						{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
						{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
					},
					Logs:      []byte("logs"),
					Threshold: rule.Threshold,
				}

				processor := &processorMock{}
				processor.
					On("Process", &rule).
					Return(&alert, nil).
					Once()

				processors[ThresholdType] = processor

				backend.
					On("PreflightAlerts", []Alert{alert}).
					Return([]Alert{alert}, nil).
					Once()

				backend.
					On("SendAlerts", []Alert{alert}).
					Return(nil).
					Once()
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
			on: func(rules []Rule, processors map[string]Processor, backend *backendMock) {
				rule := rules[0]
				alert := Alert{
					RuleID:  rule.ID,
					Ingress: rule.Ingress,
					Service: rule.Service,
					Points: []Point{
						{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
						{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
					},
					Logs:      []byte("logs"),
					Threshold: rule.Threshold,
				}

				processor := &processorMock{}
				processor.
					On("Process", &rule).
					Return(&alert, nil).
					Once()

				processors[ThresholdType] = processor

				backend.
					On("PreflightAlerts", []Alert{alert}).
					Return([]Alert{}, nil).
					Once()
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
			on: func(rules []Rule, processors map[string]Processor, backend *backendMock) {
				processor := &processorMock{}
				processor.
					On("Process", &rules[0]).
					Return(nil, nil).
					Once()

				processors[ThresholdType] = processor

				var alerts []Alert
				backend.
					On("PreflightAlerts", alerts).
					Return([]Alert{}, nil).
					Once()
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
			on: func(rules []Rule, processors map[string]Processor, backend *backendMock) {
				rule := rules[0]
				alert := Alert{
					RuleID:  rule.ID,
					Ingress: rule.Ingress,
					Service: rule.Service,
					Points: []Point{
						{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
						{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
					},
					Logs:      []byte("logs"),
					Threshold: rule.Threshold,
				}

				processor := &processorMock{}
				processor.
					On("Process", &rule).
					Return(&alert, nil).
					Once()

				processors[ThresholdType] = processor

				backend.
					On("PreflightAlerts", []Alert{alert}).
					Return([]Alert{alert}, nil).
					Once()

				backend.
					On("SendAlerts", []Alert{alert}).
					Return(errors.New("boom")).
					Once()
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
			on: func(rules []Rule, processors map[string]Processor, backend *backendMock) {
				rule := rules[0]
				alert := Alert{
					RuleID:  rule.ID,
					Ingress: rule.Ingress,
					Service: rule.Service,
					Points: []Point{
						{Timestamp: time.Now().Add(-30 * time.Minute).Unix(), Value: 110},
						{Timestamp: time.Now().Add(-20 * time.Minute).Unix(), Value: 100},
					},
					Logs:      []byte("logs"),
					Threshold: rule.Threshold,
				}

				processor := &processorMock{}
				processor.
					On("Process", &rule).
					Return(&alert, nil).
					Once()

				processors[ThresholdType] = processor

				backend.
					On("PreflightAlerts", []Alert{alert}).
					Return(nil, errors.New("boom")).
					Once()
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
			on: func(rules []Rule, processors map[string]Processor, backend *backendMock) {
				processor := &processorMock{}
				processor.
					On("Process", &rules[0]).
					Return(nil, errors.New("boom")).
					Once()

				processors[ThresholdType] = processor

				var alerts []Alert
				backend.
					On("PreflightAlerts", alerts).
					Return([]Alert{}, nil).
					Once()
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
			on: func(rules []Rule, processors map[string]Processor, backend *backendMock) {
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

				processor := &processorMock{}
				processor.
					On("Process", &rules[0]).
					Return(&alerts[0], nil).
					Once()
				processor.
					On("Process", &rules[1]).
					Return(&alerts[1], nil).
					Once()

				processors[ThresholdType] = processor

				backend.
					On("PreflightAlerts", alerts).
					Return(alerts, nil).
					Once()

				backend.
					On("SendAlerts", alerts).
					Return(nil).
					Once()
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
			on: func(rules []Rule, processors map[string]Processor, backend *backendMock) {
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

				processor := &processorMock{}
				processor.
					On("Process", &rules[0]).
					Return(&alert, nil).
					Once()
				processor.
					On("Process", &rules[1]).
					Return(nil, nil).
					Once()

				processors[ThresholdType] = processor

				backend.
					On("PreflightAlerts", []Alert{alert}).
					Return([]Alert{alert}, nil).
					Once()

				backend.
					On("SendAlerts", []Alert{alert}).
					Return(nil).
					Once()
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
			on: func(rules []Rule, processors map[string]Processor, backend *backendMock) {
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

				processor := &processorMock{}
				processor.
					On("Process", &rules[0]).
					Return(&alerts[0], nil).
					Once()
				processor.
					On("Process", &rules[1]).
					Return(&alerts[1], nil).
					Once()

				processors[ThresholdType] = processor

				backend.
					On("PreflightAlerts", alerts).
					Return([]Alert{alerts[0]}, nil).
					Once()

				backend.
					On("SendAlerts", []Alert{alerts[0]}).
					Return(nil).
					Once()
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
			on: func(rules []Rule, processors map[string]Processor, backend *backendMock) {
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

				processor := &processorMock{}
				processor.
					On("Process", &rules[0]).
					Return(nil, errors.New("boom")).
					Once()
				processor.
					On("Process", &rules[1]).
					Return(&alert, nil).
					Once()

				processors[ThresholdType] = processor

				backend.
					On("PreflightAlerts", []Alert{alert}).
					Return([]Alert{alert}, nil).
					Once()

				backend.
					On("SendAlerts", []Alert{alert}).
					Return(nil).
					Once()
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
			on: func(rules []Rule, processors map[string]Processor, backend *backendMock) {
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

				processor := &processorMock{}
				processor.
					On("Process", &rules[1]).
					Return(&alert, nil).
					Once()

				processors[ThresholdType] = processor

				backend.
					On("PreflightAlerts", []Alert{alert}).
					Return([]Alert{alert}, nil).
					Once()

				backend.
					On("SendAlerts", []Alert{alert}).
					Return(nil).
					Once()
			},
			expected: require.NoError,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			backend := &backendMock{}
			processors := make(map[string]Processor)

			test.on(test.rules, processors, backend)

			mgr := NewManager(backend, processors, time.Second, time.Second)
			mgr.rules = test.rules

			err := mgr.checkAlerts(context.Background())
			test.expected(t, err)

			for _, proc := range processors {
				m := proc.(*processorMock)
				m.AssertExpectations(t)
			}
			backend.AssertExpectations(t)
		})
	}
}

type backendMock struct {
	mock.Mock
}

func (b *backendMock) GetRules(_ context.Context) ([]Rule, error) {
	call := b.Called()

	err := call.Error(1)
	if rules := call.Get(0); rules != nil {
		return rules.([]Rule), err
	}

	return nil, err
}

func (b *backendMock) PreflightAlerts(_ context.Context, alerts []Alert) ([]Alert, error) {
	call := b.Called(alerts)

	err := call.Error(1)
	if res := call.Get(0); res != nil {
		return res.([]Alert), err
	}

	return nil, err
}

func (b *backendMock) SendAlerts(_ context.Context, alerts []Alert) error {
	return b.Called(alerts).Error(0)
}

type processorMock struct {
	mock.Mock
}

func (p *processorMock) Process(_ context.Context, rule *Rule) (*Alert, error) {
	call := p.Called(rule)

	err := call.Error(1)
	if alert := call.Get(0); alert != nil {
		return alert.(*Alert), err
	}

	return nil, err
}
