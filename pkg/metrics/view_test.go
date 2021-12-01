package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataPointView_FindByIngressAndService(t *testing.T) {
	now := time.Date(2021, 1, 1, 8, 21, 43, 0, time.UTC)

	type input struct {
		table   string
		ingress string
		service string
		from    time.Time
		to      time.Time
	}

	type expected struct {
		points DataPoints
		err    bool
	}

	tests := []struct {
		desc     string
		groups   []DataPointGroup
		input    input
		expected expected
	}{
		{
			desc: "unknown table",
			input: input{
				table:   "unknown",
				ingress: "ingress-1",
				service: "service-1",
				from:    now.Add(-10 * time.Minute),
				to:      now.Add(-time.Minute),
			},
			expected: expected{},
		},
		{
			desc: "data points found for service via ingress",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-2",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-4*time.Minute), 60, 60, 15, 15, 6, 60),
						genPoint(now.Add(-3*time.Minute), 60, 120, 30, 30, 12, 120),
						genPoint(now.Add(-2*time.Minute), 60, 240, 60, 60, 24, 240),
						genPoint(now.Add(-time.Minute), 60, 480, 120, 120, 48, 480),
						genPoint(now, 60, 960, 240, 240, 96, 960),
					},
				},
				{
					Ingress: "ingress-1",
					Service: "service-2",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
				{
					Ingress: "",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
				{
					Ingress: "ingress-1",
					Service: "",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
				{
					Ingress: "",
					Service: "",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
			},
			input: input{
				table:   "1m",
				ingress: "ingress-1",
				service: "service-1",
				from:    now.Add(-3 * time.Minute),
				to:      now.Add(-2 * time.Minute),
			},
			expected: expected{points: DataPoints{
				{
					Timestamp: now.Add(-3 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          120,
					RequestErrs:       30,
					RequestClientErrs: 30,
					ResponseTimeSum:   12,
					ResponseTimeCount: 120,

					ReqPerS:                 2,
					RequestErrPerS:          0.5,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    0.5,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
				{
					Timestamp: now.Add(-2 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          240,
					RequestErrs:       60,
					RequestClientErrs: 60,
					ResponseTimeSum:   24,
					ResponseTimeCount: 240,

					ReqPerS:                 4,
					RequestErrPerS:          1,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    1,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
			}},
		},
		{
			desc: "data point group found but no data points",
			groups: []DataPointGroup{
				{
					Ingress:    "ingress-1",
					Service:    "service-1",
					DataPoints: DataPoints{},
				},
			},
			input: input{
				table:   "1m",
				ingress: "ingress-1",
				service: "service-1",
				from:    now.Add(-10 * time.Minute),
				to:      now.Add(-time.Minute),
			},
			expected: expected{},
		},
		{
			desc: "data point group found but no data points in range",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-12*time.Minute), 60, 60, 15, 15, 6, 60),
						genPoint(now.Add(-3*time.Minute), 60, 60, 15, 15, 6, 60),
					},
				},
			},
			input: input{
				table:   "1m",
				ingress: "ingress-1",
				service: "service-1",
				from:    now.Add(-10 * time.Minute),
				to:      now.Add(-5 * time.Minute),
			},
			expected: expected{},
		},
		{
			desc: "more than one data point group found",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
						genPoint(now.Add(-2*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
						genPoint(now.Add(-2*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
			},
			input: input{
				table:   "1m",
				ingress: "ingress-1",
				service: "service-1",
				from:    now.Add(-10 * time.Minute),
				to:      now.Add(-time.Minute),
			},
			expected: expected{err: true},
		},
		{
			desc: "to is before from",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
			},
			input: input{
				table:   "1m",
				ingress: "ingress-1",
				service: "service-1",
				from:    now.Add(-time.Minute),
				to:      now.Add(-10 * time.Minute),
			},
			expected: expected{},
		},
		{
			desc: "from equals to",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
			},
			input: input{
				table:   "1m",
				ingress: "ingress-1",
				service: "service-1",
				from:    now.Add(-time.Minute),
				to:      now.Add(-time.Minute),
			},
			expected: expected{},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			store := &storeMock{forEach: func(table string, fn ForEachFunc) {
				require.Equal(t, test.input.table, table)

				for _, group := range test.groups {
					fn(group.Ingress, group.Service, group.DataPoints)
				}
			}}

			view := DataPointView{store: store, nowFunc: func() time.Time { return now }}
			gotPoints, err := view.FindByIngressAndService(test.input.table,
				test.input.ingress, test.input.service,
				test.input.from, test.input.to)

			if test.expected.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, test.expected.points, gotPoints)
		})
	}
}

func TestDataPointView_FindByService(t *testing.T) {
	now := time.Date(2021, 1, 1, 8, 21, 43, 0, time.UTC)

	type input struct {
		table   string
		service string
		from    time.Time
		to      time.Time
	}

	tests := []struct {
		desc     string
		groups   []DataPointGroup
		input    input
		expected DataPoints
	}{
		{
			desc: "unknown table",
			input: input{
				table:   "unknown",
				service: "service-1",
				from:    now.Add(-10 * time.Minute),
				to:      now.Add(-time.Minute),
			},
		},
		{
			desc: "data points found for service",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-2",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-4*time.Minute), 60, 60, 15, 15, 6, 60),
						genPoint(now.Add(-3*time.Minute), 60, 120, 30, 30, 12, 120),
						genPoint(now.Add(-2*time.Minute), 60, 240, 60, 60, 24, 240),
						genPoint(now.Add(-time.Minute), 60, 480, 120, 120, 48, 480),
						genPoint(now, 60, 960, 240, 240, 96, 960),
					},
				},
				{
					Ingress: "ingress-1",
					Service: "service-3",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
				{
					Ingress: "ingress-1",
					Service: "",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
				{
					Ingress: "",
					Service: "",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
			},
			input: input{
				table:   "1m",
				service: "service-1",
				from:    now.Add(-3 * time.Minute),
				to:      now.Add(-2 * time.Minute),
			},
			expected: DataPoints{
				{
					Timestamp: now.Add(-3 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          120,
					RequestErrs:       30,
					RequestClientErrs: 30,
					ResponseTimeSum:   12,
					ResponseTimeCount: 120,

					ReqPerS:                 2,
					RequestErrPerS:          0.5,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    0.5,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
				{
					Timestamp: now.Add(-2 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          240,
					RequestErrs:       60,
					RequestClientErrs: 60,
					ResponseTimeSum:   24,
					ResponseTimeCount: 240,

					ReqPerS:                 4,
					RequestErrPerS:          1,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    1,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
			},
		},
		{
			desc: "data point group found but no data points",
			groups: []DataPointGroup{
				{
					Ingress:    "ingress-1",
					Service:    "service-1",
					DataPoints: DataPoints{},
				},
			},
			input: input{
				table:   "1m",
				service: "service-1",
				from:    now.Add(-10 * time.Minute),
				to:      now.Add(-time.Minute),
			},
		},
		{
			desc: "data point group found but no data points in range",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-12*time.Minute), 60, 60, 15, 15, 6, 60),
						genPoint(now.Add(-3*time.Minute), 60, 60, 15, 15, 6, 60),
					},
				},
			},
			input: input{
				table:   "1m",
				service: "service-1",
				from:    now.Add(-10 * time.Minute),
				to:      now.Add(-5 * time.Minute),
			},
		},
		{
			desc: "more than one data point group found",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-4*time.Minute), 60, 60, 15, 15, 6, 60),
						genPoint(now.Add(-3*time.Minute), 60, 120, 30, 30, 12, 120),
					},
				},
				{
					Ingress: "ingress-2",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-4*time.Minute), 60, 120, 30, 30, 12, 120),
						genPoint(now.Add(-3*time.Minute), 60, 240, 60, 60, 24, 240),
					},
				},
			},
			input: input{
				table:   "1m",
				service: "service-1",
				from:    now.Add(-10 * time.Minute),
				to:      now.Add(-time.Minute),
			},
			expected: DataPoints{
				{
					Timestamp: now.Add(-4 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          180,
					RequestErrs:       45,
					RequestClientErrs: 45,
					ResponseTimeSum:   18,
					ResponseTimeCount: 180,

					ReqPerS:                 3,
					RequestErrPerS:          0.75,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    0.75,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
				{
					Timestamp: now.Add(-3 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          360,
					RequestErrs:       90,
					RequestClientErrs: 90,
					ResponseTimeSum:   36,
					ResponseTimeCount: 360,

					ReqPerS:                 6,
					RequestErrPerS:          1.5,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    1.5,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
			},
		},
		{
			desc: "more than one data point group found: with missing data points",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-4*time.Minute), 60, 60, 15, 15, 6, 60),
						genPoint(now.Add(-3*time.Minute), 60, 120, 30, 30, 12, 120),
					},
				},
				{
					Ingress: "ingress-2",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 60, 240, 60, 60, 24, 240),
						genPoint(now.Add(-2*time.Minute), 60, 480, 120, 120, 48, 480),
					},
				},
			},
			input: input{
				table:   "1m",
				service: "service-1",
				from:    now.Add(-10 * time.Minute),
				to:      now.Add(-time.Minute),
			},
			expected: DataPoints{
				{
					Timestamp: now.Add(-4 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          60,
					RequestErrs:       15,
					RequestClientErrs: 15,
					ResponseTimeSum:   6,
					ResponseTimeCount: 60,

					ReqPerS:                 1,
					RequestErrPerS:          0.25,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    0.25,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
				{
					Timestamp: now.Add(-3 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          360,
					RequestErrs:       90,
					RequestClientErrs: 90,
					ResponseTimeSum:   36,
					ResponseTimeCount: 360,

					ReqPerS:                 6,
					RequestErrPerS:          1.5,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    1.5,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
				{
					Timestamp: now.Add(-2 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          480,
					RequestErrs:       120,
					RequestClientErrs: 120,
					ResponseTimeSum:   48,
					ResponseTimeCount: 480,

					ReqPerS:                 8,
					RequestErrPerS:          2,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    2,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
			},
		},
		{
			desc: "to is before from",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 60, 60, 15, 15, 6, 60),
					},
				},
			},
			input: input{
				table:   "1m",
				service: "service-1",
				from:    now.Add(-time.Minute),
				to:      now.Add(-10 * time.Minute),
			},
		},
		{
			desc: "from equals to",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 60, 60, 15, 15, 6, 60),
					},
				},
			},
			input: input{
				table:   "1m",
				service: "service-1",
				from:    now.Add(-time.Minute),
				to:      now.Add(-time.Minute),
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			store := &storeMock{forEach: func(table string, fn ForEachFunc) {
				require.Equal(t, test.input.table, table)

				for _, group := range test.groups {
					fn(group.Ingress, group.Service, group.DataPoints)
				}
			}}

			view := DataPointView{store: store, nowFunc: func() time.Time { return now }}
			gotPoints := view.FindByService(test.input.table, test.input.service, test.input.from, test.input.to)

			assert.Equal(t, test.expected, gotPoints)
		})
	}
}

func TestDataPointView_FindByIngress(t *testing.T) {
	now := time.Date(2021, 1, 1, 8, 21, 43, 0, time.UTC)

	type input struct {
		table   string
		ingress string
		from    time.Time
		to      time.Time
	}

	tests := []struct {
		desc     string
		groups   []DataPointGroup
		input    input
		expected DataPoints
	}{
		{
			desc: "unknown table",
			input: input{
				table:   "unknown",
				ingress: "ingress-1",
				from:    now.Add(-10 * time.Minute),
				to:      now.Add(-time.Minute),
			},
		},
		{
			desc: "data points found for ingress",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-2",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-4*time.Minute), 60, 60, 15, 15, 6, 60),
						genPoint(now.Add(-3*time.Minute), 60, 120, 30, 30, 12, 120),
						genPoint(now.Add(-2*time.Minute), 60, 240, 60, 60, 24, 240),
						genPoint(now.Add(-time.Minute), 60, 480, 120, 120, 48, 480),
						genPoint(now, 60, 960, 240, 240, 96, 960),
					},
				},
				{
					Ingress: "ingress-2",
					Service: "service-3",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
				{
					Ingress: "",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
				{
					Ingress: "",
					Service: "",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 1, 1, 1, 1, 1, 1),
					},
				},
			},
			input: input{
				table:   "1m",
				ingress: "ingress-1",
				from:    now.Add(-3 * time.Minute),
				to:      now.Add(-2 * time.Minute),
			},
			expected: DataPoints{
				{
					Timestamp: now.Add(-3 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          120,
					RequestErrs:       30,
					RequestClientErrs: 30,
					ResponseTimeSum:   12,
					ResponseTimeCount: 120,

					ReqPerS:                 2,
					RequestErrPerS:          0.5,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    0.5,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
				{
					Timestamp: now.Add(-2 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          240,
					RequestErrs:       60,
					RequestClientErrs: 60,
					ResponseTimeSum:   24,
					ResponseTimeCount: 240,

					ReqPerS:                 4,
					RequestErrPerS:          1,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    1,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
			},
		},
		{
			desc: "data point group found but no data points",
			groups: []DataPointGroup{
				{
					Ingress:    "ingress-1",
					Service:    "service-1",
					DataPoints: DataPoints{},
				},
			},
			input: input{
				table:   "1m",
				ingress: "ingress-1",
				from:    now.Add(-10 * time.Minute),
				to:      now.Add(-time.Minute),
			},
		},
		{
			desc: "data point group found but no data points in range",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-12*time.Minute), 60, 60, 15, 15, 6, 60),
						genPoint(now.Add(-3*time.Minute), 60, 60, 15, 15, 6, 60),
					},
				},
			},
			input: input{
				table:   "1m",
				ingress: "ingress-1",
				from:    now.Add(-10 * time.Minute),
				to:      now.Add(-5 * time.Minute),
			},
		},
		{
			desc: "more than one data point group found",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-4*time.Minute), 60, 60, 15, 15, 6, 60),
						genPoint(now.Add(-3*time.Minute), 60, 120, 30, 30, 12, 120),
					},
				},
				{
					Ingress: "ingress-1",
					Service: "service-2",
					DataPoints: DataPoints{
						genPoint(now.Add(-4*time.Minute), 60, 120, 30, 30, 12, 120),
						genPoint(now.Add(-3*time.Minute), 60, 240, 60, 60, 24, 240),
					},
				},
			},
			input: input{
				table:   "1m",
				ingress: "ingress-1",
				from:    now.Add(-10 * time.Minute),
				to:      now.Add(-time.Minute),
			},
			expected: DataPoints{
				{
					Timestamp: now.Add(-4 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          180,
					RequestErrs:       45,
					RequestClientErrs: 45,
					ResponseTimeSum:   18,
					ResponseTimeCount: 180,

					ReqPerS:                 3,
					RequestErrPerS:          0.75,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    0.75,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
				{
					Timestamp: now.Add(-3 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          360,
					RequestErrs:       90,
					RequestClientErrs: 90,
					ResponseTimeSum:   36,
					ResponseTimeCount: 360,

					ReqPerS:                 6,
					RequestErrPerS:          1.5,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    1.5,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
			},
		},
		{
			desc: "more than one data point group found: with missing data points",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-4*time.Minute), 60, 60, 15, 15, 6, 60),
						genPoint(now.Add(-3*time.Minute), 60, 120, 30, 30, 12, 120),
					},
				},
				{
					Ingress: "ingress-1",
					Service: "service-2",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 60, 240, 60, 60, 24, 240),
						genPoint(now.Add(-2*time.Minute), 60, 480, 120, 120, 48, 480),
					},
				},
			},
			input: input{
				table:   "1m",
				ingress: "ingress-1",
				from:    now.Add(-10 * time.Minute),
				to:      now.Add(-time.Minute),
			},
			expected: DataPoints{
				{
					Timestamp: now.Add(-4 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          60,
					RequestErrs:       15,
					RequestClientErrs: 15,
					ResponseTimeSum:   6,
					ResponseTimeCount: 60,

					ReqPerS:                 1,
					RequestErrPerS:          0.25,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    0.25,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
				{
					Timestamp: now.Add(-3 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          360,
					RequestErrs:       90,
					RequestClientErrs: 90,
					ResponseTimeSum:   36,
					ResponseTimeCount: 360,

					ReqPerS:                 6,
					RequestErrPerS:          1.5,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    1.5,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
				{
					Timestamp: now.Add(-2 * time.Minute).Unix(),

					Seconds:           60,
					Requests:          480,
					RequestErrs:       120,
					RequestClientErrs: 120,
					ResponseTimeSum:   48,
					ResponseTimeCount: 480,

					ReqPerS:                 8,
					RequestErrPerS:          2,
					RequestErrPercent:       0.25,
					RequestClientErrPerS:    2,
					RequestClientErrPercent: 0.25,
					AvgResponseTime:         0.1,
				},
			},
		},
		{
			desc: "to is before from",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 60, 60, 15, 15, 6, 60),
					},
				},
			},
			input: input{
				table:   "1m",
				ingress: "ingress-1",
				from:    now.Add(-time.Minute),
				to:      now.Add(-10 * time.Minute),
			},
		},
		{
			desc: "from equals to",
			groups: []DataPointGroup{
				{
					Ingress: "ingress-1",
					Service: "service-1",
					DataPoints: DataPoints{
						genPoint(now.Add(-3*time.Minute), 60, 60, 15, 15, 6, 60),
					},
				},
			},
			input: input{
				table:   "1m",
				ingress: "ingress-1",
				from:    now.Add(-time.Minute),
				to:      now.Add(-time.Minute),
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			store := &storeMock{forEach: func(table string, fn ForEachFunc) {
				require.Equal(t, test.input.table, table)

				for _, group := range test.groups {
					fn(group.Ingress, group.Service, group.DataPoints)
				}
			}}

			view := DataPointView{store: store, nowFunc: func() time.Time { return now }}
			gotPoints := view.FindByIngress(test.input.table, test.input.ingress, test.input.from, test.input.to)

			assert.Equal(t, test.expected, gotPoints)
		})
	}
}

func genPoint(ts time.Time, secs, reqs, reqErrs, reqClientErrs int64, respTimeSum float64, respTimeCount int64) DataPoint {
	return DataPoint{
		Timestamp: ts.Unix(),

		Seconds:           secs,
		Requests:          reqs,
		RequestErrs:       reqErrs,
		RequestClientErrs: reqClientErrs,
		ResponseTimeSum:   respTimeSum,
		ResponseTimeCount: respTimeCount,

		ReqPerS:                 float64(reqs) / float64(secs),
		RequestErrPerS:          float64(reqErrs) / float64(secs),
		RequestErrPercent:       float64(reqErrs) / float64(reqs),
		RequestClientErrPerS:    float64(reqClientErrs) / float64(secs),
		RequestClientErrPercent: float64(reqClientErrs) / float64(reqs),
		AvgResponseTime:         respTimeSum / float64(respTimeCount),
	}
}

type storeMock struct {
	forEach func(table string, fn ForEachFunc)
}

func (s storeMock) ForEach(tbl string, fn ForEachFunc) {
	s.forEach(tbl, fn)
}
