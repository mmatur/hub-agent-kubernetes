package alerting

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent/pkg/metrics"
)

func TestThresholdProcessor_NoMatchingRule(t *testing.T) {
	now := time.Date(2021, 1, 1, 8, 21, 43, 0, time.UTC)
	store := mockThresholdStore{
		group: generateData(t, now, 3, 10, 3),
	}
	logs := mockLogProvider{}

	threshProc := NewThresholdProcessor(store, logs)
	threshProc.nowFunc = func() time.Time { return now }

	got, err := threshProc.Process(context.Background(), &Rule{
		ID:      "123",
		Ingress: "ing@ns",
		Service: "svc2@ns",
		Threshold: &Threshold{
			Metric: "requestsPerSecond",
			Condition: ThresholdCondition{
				Above: true,
				Value: 10,
			},
			Occurrence: 3,
			TimeRange:  10 * time.Minute,
		},
	})
	require.NoError(t, err)

	assert.Nil(t, got)
}

func TestThresholdProcessor_NotEnoughPoints(t *testing.T) {
	now := time.Date(2021, 1, 1, 8, 21, 43, 0, time.UTC)
	store := mockThresholdStore{
		group: generateData(t, now, 2, 10, 2),
	}
	logs := mockLogProvider{}

	threshProc := NewThresholdProcessor(store, logs)
	threshProc.nowFunc = func() time.Time { return now }

	got, err := threshProc.Process(context.Background(), &Rule{
		ID:      "123",
		Ingress: "ing@ns",
		Service: "svc2@ns",
		Threshold: &Threshold{
			Metric: "requestsPerSecond",
			Condition: ThresholdCondition{
				Above: true,
				Value: 10,
			},
			Occurrence: 3,
			TimeRange:  10 * time.Minute,
		},
	})
	require.NoError(t, err)

	assert.Nil(t, got)
}

func TestThresholdProcessor_NoAlert(t *testing.T) {
	now := time.Date(2021, 1, 1, 8, 21, 43, 0, time.UTC)
	store := mockThresholdStore{
		group: generateData(t, now, 3, 10, 2),
	}
	logs := mockLogProvider{}

	threshProc := NewThresholdProcessor(store, logs)
	threshProc.nowFunc = func() time.Time { return now }

	got, err := threshProc.Process(context.Background(), &Rule{
		ID:      "123",
		Ingress: "ing@ns",
		Service: "svc2@ns",
		Threshold: &Threshold{
			Metric: "requestsPerSecond",
			Condition: ThresholdCondition{
				Above: true,
				Value: 10,
			},
			Occurrence: 3,
			TimeRange:  10 * time.Minute,
		},
	})
	require.NoError(t, err)

	assert.Nil(t, got)
}

func TestThresholdProcessor_NoAlert10Minute(t *testing.T) {
	now := time.Date(2021, 1, 1, 8, 21, 43, 0, time.UTC)
	data := metrics.DataPoints{
		{
			Timestamp: time.Date(2021, 1, 1, 8, 10, 0, 0, time.UTC).Unix(),
			ReqPerS:   15.78,
		},
		{
			Timestamp: time.Date(2021, 1, 1, 8, 0, 0, 0, time.UTC).Unix(),
			ReqPerS:   9.78,
		},
		{
			Timestamp: time.Date(2021, 1, 1, 7, 50, 0, 0, time.UTC).Unix(),
			ReqPerS:   15.78,
		},
		{
			Timestamp: time.Date(2021, 1, 1, 7, 40, 0, 0, time.UTC).Unix(),
			ReqPerS:   9.78,
		},
		{
			Timestamp: time.Date(2021, 1, 1, 7, 30, 0, 0, time.UTC).Unix(),
			ReqPerS:   15.78,
		},
	}
	store := mockThresholdStore{
		group: metrics.DataPointGroup{
			Ingress:    "ing",
			Service:    "svc",
			DataPoints: data,
		},
	}
	logs := mockLogProvider{}

	threshProc := NewThresholdProcessor(store, logs)
	threshProc.nowFunc = func() time.Time { return now }

	got, err := threshProc.Process(context.Background(), &Rule{
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
			TimeRange:  40 * time.Minute,
		},
	})

	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestThresholdProcessor_Alert(t *testing.T) {
	now := time.Date(2021, 1, 1, 8, 21, 43, 0, time.UTC)
	metricsData := generateData(t, now, 3, 15, 3)
	store := mockThresholdStore{
		group: metricsData,
	}
	logs := mockLogProvider{
		getServiceLogsFn: func(namespace, name string, lines, maxLen int) ([]byte, error) {
			assert.Equal(t, "svc", name)
			assert.Equal(t, "ns", namespace)

			return []byte("fake logs"), nil
		},
	}

	threshProc := NewThresholdProcessor(store, logs)
	threshProc.nowFunc = func() time.Time { return now }

	got, err := threshProc.Process(context.Background(), &Rule{
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
	})
	require.NoError(t, err)

	newPnts := make([]Point, len(metricsData.DataPoints))
	for i, pnt := range metricsData.DataPoints {
		newPnts[i] = Point{Timestamp: pnt.Timestamp, Value: pnt.ReqPerS}
	}
	logBytes, err := compress([]byte("fake logs"))
	require.NoError(t, err)

	want := &Alert{
		RuleID:  "123",
		Ingress: "ing@ns",
		Service: "svc@ns",
		Points:  newPnts,
		Logs:    logBytes,
		State:   stateCritical,
		Threshold: &Threshold{
			Metric: "requestsPerSecond",
			Condition: ThresholdCondition{
				Above: true,
				Value: 10,
			},
			Occurrence: 3,
			TimeRange:  10 * time.Minute,
		},
	}
	assert.Equal(t, want, got)
}

func TestThresholdProcessor_Alert10Minute(t *testing.T) {
	now := time.Date(2021, 1, 1, 8, 21, 43, 0, time.UTC)
	data := metrics.DataPoints{
		{
			Timestamp: time.Date(2021, 1, 1, 8, 10, 0, 0, time.UTC).Unix(),
			ReqPerS:   15.78,
		},
		{
			Timestamp: time.Date(2021, 1, 1, 8, 0, 0, 0, time.UTC).Unix(),
			ReqPerS:   9.78,
		},
		{
			Timestamp: time.Date(2021, 1, 1, 7, 50, 0, 0, time.UTC).Unix(),
			ReqPerS:   15.78,
		},
		{
			Timestamp: time.Date(2021, 1, 1, 7, 40, 0, 0, time.UTC).Unix(),
			ReqPerS:   15.78,
		},
		{
			Timestamp: time.Date(2021, 1, 1, 7, 30, 0, 0, time.UTC).Unix(),
			ReqPerS:   9.78,
		},
	}
	store := mockThresholdStore{
		group: metrics.DataPointGroup{
			Ingress:    "ing@ns",
			Service:    "svc@ns",
			DataPoints: data,
		},
	}
	logs := mockLogProvider{
		getServiceLogsFn: func(namespace, name string, lines, maxLen int) ([]byte, error) {
			assert.Equal(t, "svc", name)
			assert.Equal(t, "ns", namespace)

			return []byte("fake logs"), nil
		},
	}

	threshProc := NewThresholdProcessor(store, logs)
	threshProc.nowFunc = func() time.Time { return now }

	got, err := threshProc.Process(context.Background(), &Rule{
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
			TimeRange:  40 * time.Minute,
		},
	})

	require.NoError(t, err)

	var newPnts []Point
	for _, pnt := range data[:4] {
		newPnts = append(newPnts, Point{Timestamp: pnt.Timestamp, Value: pnt.ReqPerS})
	}
	logBytes, err := compress([]byte("fake logs"))
	require.NoError(t, err)

	want := &Alert{
		RuleID:  "123",
		Ingress: "ing@ns",
		Service: "svc@ns",
		Points:  newPnts,
		Logs:    logBytes,
		State:   stateCritical,
		Threshold: &Threshold{
			Metric: "requestsPerSecond",
			Condition: ThresholdCondition{
				Above: true,
				Value: 10,
			},
			Occurrence: 3,
			TimeRange:  40 * time.Minute,
		},
	}
	assert.Equal(t, want, got)
}

type mockThresholdStore struct {
	group metrics.DataPointGroup
}

func (t mockThresholdStore) ForEach(table string, fn metrics.ForEachFunc) {
	fn(table, t.group.Ingress, t.group.Service, t.group.DataPoints)
}

type mockLogProvider struct {
	getServiceLogsFn func(namespace, name string, lines, maxLen int) ([]byte, error)
}

func (m mockLogProvider) GetServiceLogs(_ context.Context, namespace, name string, lines, maxLen int) ([]byte, error) {
	return m.getServiceLogsFn(namespace, name, lines, maxLen)
}

func generateData(t *testing.T, now time.Time, points, threshold, occurrences int) metrics.DataPointGroup {
	t.Helper()

	group := metrics.DataPointGroup{
		Ingress:    "ing@ns",
		Service:    "svc@ns",
		DataPoints: metrics.DataPoints{},
	}

	for i := points; i > 0; i-- {
		var val float64
		if occurrences > 0 {
			val = float64(threshold) + rand.Float64()*float64(threshold) //nolint:gosec // No need to crypto randomness in this test.
			occurrences--
		} else {
			val = rand.Float64() * float64(threshold) //nolint:gosec // No need to crypto randomness in this test.
		}

		group.DataPoints = append(group.DataPoints, metrics.DataPoint{
			Timestamp:            now.Add(time.Duration(-i) * time.Minute).Unix(),
			ReqPerS:              val,
			RequestErrPerS:       val,
			RequestClientErrPerS: val,
			AvgResponseTime:      val,
		})
	}

	return group
}
