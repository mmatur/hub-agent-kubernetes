package metrics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/traefik/neo-agent/pkg/metrics"
)

func TestDataPoints_Get(t *testing.T) {
	pnts := metrics.DataPoints{
		{Timestamp: 123},
		{Timestamp: 456},
		{Timestamp: 789},
	}

	gotIdx, gotPnt := pnts.Get(456)

	assert.Equal(t, 1, gotIdx)
	assert.Equal(t, int64(456), gotPnt.Timestamp)
}

func TestDataPoints_GetHandlesNotFound(t *testing.T) {
	pnts := metrics.DataPoints{
		{Timestamp: 123},
		{Timestamp: 456},
		{Timestamp: 789},
	}

	gotIdx, gotPnt := pnts.Get(111)

	assert.Equal(t, -1, gotIdx)
	assert.Equal(t, int64(0), gotPnt.Timestamp)
}

func TestDataPoints_Aggregate(t *testing.T) {
	pnts := metrics.DataPoints{
		{
			ReqPerS:             10,
			RequestErrPer:       10,
			RequestClientErrPer: 10,
			AvgResponseTime:     1,
			Requests:            10,
			RequestErrs:         1,
			RequestClientErrs:   1,
			ResponseTimeSum:     10,
			ResponseTimeCount:   1,
		},
		{
			ReqPerS:             20,
			RequestErrPer:       10,
			RequestClientErrPer: 10,
			AvgResponseTime:     1,
			Requests:            20,
			RequestErrs:         2,
			RequestClientErrs:   2,
			ResponseTimeSum:     20,
			ResponseTimeCount:   2,
		},
		{
			ReqPerS:             30,
			RequestErrPer:       10,
			RequestClientErrPer: 10,
			AvgResponseTime:     1,
			Requests:            30,
			RequestErrs:         3,
			RequestClientErrs:   3,
			ResponseTimeSum:     30,
			ResponseTimeCount:   3,
		},
	}

	got := pnts.Aggregate()

	assert.Equal(t, float64(20), got.ReqPerS)
	assert.Equal(t, 0.1, got.RequestErrPer)
	assert.Equal(t, 0.1, got.RequestClientErrPer)
	assert.Equal(t, float64(10), got.AvgResponseTime)
	assert.Equal(t, int64(60), got.Requests)
	assert.Equal(t, int64(6), got.RequestErrs)
	assert.Equal(t, int64(6), got.RequestClientErrs)
	assert.Equal(t, float64(60), got.ResponseTimeSum)
	assert.Equal(t, int64(6), got.ResponseTimeCount)
}

func TestAggregator_Aggregate(t *testing.T) {
	ms := []metrics.Metric{
		&metrics.Counter{Name: metrics.MetricRequests, Service: "default/whoami", Value: 12},
		&metrics.Counter{Name: metrics.MetricRequests, Service: "default/whoami", Value: 14},
		&metrics.Counter{Name: metrics.MetricRequestClientErrors, Service: "default/whoami", Value: 14},
		&metrics.Counter{Name: metrics.MetricRequests, Service: "default/whoami2", Value: 16},
		&metrics.Counter{Name: metrics.MetricRequestErrors, Service: "default/whoami2", Value: 16},
		&metrics.Histogram{
			Name:    metrics.MetricRequestDuration,
			Service: "default/whoami",
			Sum:     0.041072671000000005,
			Count:   26,
		},
		&metrics.Histogram{
			Name:    metrics.MetricRequestDuration,
			Service: "default/whoami2",
			Sum:     0.021072671000000005,
			Count:   16,
		},
		&metrics.Histogram{
			Name:    metrics.MetricRequestDuration,
			Service: "default/whoami2",
			Sum:     0.021072671000000005,
			Count:   16,
		},
	}

	svcs := metrics.Aggregate(ms)

	if !assert.Len(t, svcs, 2) {
		return
	}
	assert.Equal(t, svcs["default/whoami"], metrics.Service{
		Requests:            26,
		RequestErrors:       0,
		RequestClientErrors: 14,
		RequestDuration: metrics.ServiceHistogram{
			Sum:   0.041072671000000005,
			Count: 26,
		},
	})
	assert.Equal(t, svcs["default/whoami2"], metrics.Service{
		Requests:            16,
		RequestErrors:       16,
		RequestClientErrors: 0,
		RequestDuration: metrics.ServiceHistogram{
			Sum:   0.04214534200000001,
			Count: 32,
		},
	})
}
