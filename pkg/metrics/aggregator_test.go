package metrics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			ReqPerS:                 10,
			RequestErrPerS:          10,
			RequestErrPercent:       10,
			RequestClientErrPerS:    10,
			RequestClientErrPercent: 10,
			AvgResponseTime:         1,
			Seconds:                 60,
			Requests:                10,
			RequestErrs:             1,
			RequestClientErrs:       2,
			ResponseTimeSum:         10,
			ResponseTimeCount:       1,
		},
		{
			ReqPerS:                 20,
			RequestErrPerS:          20,
			RequestErrPercent:       10,
			RequestClientErrPerS:    20,
			RequestClientErrPercent: 10,
			AvgResponseTime:         1,
			Seconds:                 60,
			Requests:                20,
			RequestErrs:             2,
			RequestClientErrs:       3,
			ResponseTimeSum:         20,
			ResponseTimeCount:       2,
		},
		{
			ReqPerS:                 30,
			RequestErrPerS:          30,
			RequestErrPercent:       10,
			RequestClientErrPerS:    30,
			RequestClientErrPercent: 10,
			AvgResponseTime:         1,
			Seconds:                 60,
			Requests:                30,
			RequestErrs:             3,
			RequestClientErrs:       4,
			ResponseTimeSum:         30,
			ResponseTimeCount:       3,
		},
	}

	got := pnts.Aggregate()

	assert.Equal(t, 0.3333333333333333, got.ReqPerS)
	assert.Equal(t, 0.03333333333333333, got.RequestErrPerS)
	assert.Equal(t, 0.1, got.RequestErrPercent)
	assert.Equal(t, 0.05, got.RequestClientErrPerS)
	assert.Equal(t, 0.15, got.RequestClientErrPercent)
	assert.Equal(t, float64(10), got.AvgResponseTime)
	assert.Equal(t, int64(180), got.Seconds)
	assert.Equal(t, int64(60), got.Requests)
	assert.Equal(t, int64(6), got.RequestErrs)
	assert.Equal(t, int64(9), got.RequestClientErrs)
	assert.Equal(t, float64(60), got.ResponseTimeSum)
	assert.Equal(t, int64(6), got.ResponseTimeCount)
}

func TestAggregator_Aggregate(t *testing.T) {
	ms := []metrics.Metric{
		&metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress", Service: "whoami@default", Value: 12},
		&metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress", Service: "whoami@default", Value: 14},
		&metrics.Counter{Name: metrics.MetricRequestClientErrors, Ingress: "myIngress", Service: "whoami@default", Value: 14},
		&metrics.Counter{Name: metrics.MetricRequests, Ingress: "myIngress", Service: "whoami2@default", Value: 16},
		&metrics.Counter{Name: metrics.MetricRequestErrors, Ingress: "myIngress", Service: "whoami2@default", Value: 16},
		&metrics.Histogram{
			Name:    metrics.MetricRequestDuration,
			Ingress: "myIngress",
			Service: "whoami@default",
			Sum:     0.041072671000000005,
			Count:   26,
		},
		&metrics.Histogram{
			Name:    metrics.MetricRequestDuration,
			Ingress: "myIngress",
			Service: "whoami2@default",
			Sum:     0.021072671000000005,
			Count:   16,
		},
		&metrics.Histogram{
			Name:    metrics.MetricRequestDuration,
			Ingress: "myIngress",
			Service: "whoami2@default",
			Sum:     0.021072671000000005,
			Count:   16,
		},
	}

	svcs := metrics.Aggregate(ms)

	require.Len(t, svcs, 2)

	assert.Equal(t, svcs[metrics.SetKey{Ingress: "myIngress", Service: "whoami@default"}], metrics.MetricSet{
		Requests:            26,
		RequestErrors:       0,
		RequestClientErrors: 14,
		RequestDuration: metrics.ServiceHistogram{
			Sum:   0.041072671000000005,
			Count: 26,
		},
	})
	assert.Equal(t, svcs[metrics.SetKey{Ingress: "myIngress", Service: "whoami2@default"}], metrics.MetricSet{
		Requests:            16,
		RequestErrors:       16,
		RequestClientErrors: 0,
		RequestDuration: metrics.ServiceHistogram{
			Sum:   0.04214534200000001,
			Count: 32,
		},
	})
}
