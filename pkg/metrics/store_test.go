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

package metrics

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStore_PopulateAndForEach(t *testing.T) {
	pnts := []DataPoint{
		{
			Timestamp:         42,
			ReqPerS:           42.42,
			RequestErrs:       42,
			RequestClientErrs: 69,
			AvgResponseTime:   42.69,
			ResponseTimeSum:   2000,
			ResponseTimeCount: 10,
		},
		{
			Timestamp:         41,
			ReqPerS:           42.42,
			RequestErrs:       42,
			RequestClientErrs: 69,
			AvgResponseTime:   42.69,
			ResponseTimeSum:   2000,
			ResponseTimeCount: 10,
		},
	}

	store := NewStore()

	err := store.Populate("1m", []DataPointGroup{
		{
			Ingress:    "bar",
			Service:    "baz",
			DataPoints: pnts,
		},
	})

	assert.NoError(t, err)
}

func TestStore_Insert(t *testing.T) {
	datapoint := DataPoint{
		Timestamp:         42,
		ReqPerS:           42.42,
		RequestErrs:       42,
		RequestClientErrs: 69,
		AvgResponseTime:   42.69,
		ResponseTimeSum:   2000,
		ResponseTimeCount: 10,
	}

	store := NewStore()

	store.Insert(map[SetKey]DataPoint{
		{Ingress: "foo", Service: "bar"}: datapoint,
	})

	var got []DataPoint
	store.ForEach("1m", func(_, ingr, svc string, pnts DataPoints) {
		got = append(got, pnts...)
	})

	if assert.Len(t, got, 1) {
		assert.Equal(t, datapoint, got[0])
	}
}

func TestStore_RollUp(t *testing.T) {
	now := time.Now().Truncate(time.Hour)

	store := NewStore()
	store.nowFunc = func() time.Time {
		return now
	}

	numPnts := 103

	_ = store.Populate("1m", []DataPointGroup{
		{
			Ingress:    "bar",
			Service:    "baz",
			DataPoints: genDataPoints(t, now, numPnts, time.Minute),
		},
	})
	_ = store.Populate("10m", []DataPointGroup{
		{
			Ingress:    "bar",
			Service:    "baz",
			DataPoints: genDataPoints(t, now, 10, 10*time.Minute),
		},
	})

	store.RollUp()

	store.ForEach("1m", func(_, ingr, svc string, pnts DataPoints) {
		assert.Len(t, pnts, numPnts)
	})

	store.ForEach("10m", func(_, ingr, svc string, pnts DataPoints) {
		assert.Len(t, pnts, 11)
	})

	store.ForEach("1h", func(_, ingr, svc string, pnts DataPoints) {
		assert.Len(t, pnts, 2)
	})
}

func TestStore_Cleanup(t *testing.T) {
	now := time.Now().Truncate(time.Hour).Add(-1 * time.Minute)

	store := NewStore()
	store.nowFunc = func() time.Time {
		return now
	}

	numPnts := 103

	_ = store.Populate("1m", []DataPointGroup{
		{
			Ingress:    "bar",
			Service:    "baz",
			DataPoints: genDataPoints(t, now, numPnts, time.Minute),
		},
	})
	_ = store.Populate("10m", []DataPointGroup{
		{
			Ingress:    "bar",
			Service:    "baz",
			DataPoints: genDataPoints(t, now, 10, 10*time.Minute),
		},
	})

	store.Cleanup()

	store.ForEach("1m", func(_, ingr, svc string, pnts DataPoints) {
		assert.Len(t, pnts, 10)
	})

	store.ForEach("10m", func(_, ingr, svc string, pnts DataPoints) {
		assert.Len(t, pnts, 6)
	})
}

func TestStore_CleanupDoesntRemoveUnmarked(t *testing.T) {
	now := time.Now().Truncate(time.Hour).Add(-1 * time.Minute)

	store := NewStore()
	store.nowFunc = func() time.Time {
		return now
	}

	pnts := genDataPoints(t, now, 103, time.Minute)
	for _, pnt := range pnts {
		store.Insert(map[SetKey]DataPoint{
			{Ingress: "foo", Service: "bar"}: pnt,
		})
	}

	store.Cleanup()

	store.ForEach("1m", func(_, ingr, svc string, pnts DataPoints) {
		assert.Len(t, pnts, 103)
	})
}

func genDataPoints(t *testing.T, now time.Time, n int, gran time.Duration) []DataPoint {
	t.Helper()

	start := now.Truncate(gran).Add(-1 * time.Duration(n) * gran)

	var pnts []DataPoint
	for i := 0; i < n; i++ {
		d := DataPoint{
			Timestamp:   start.Add(time.Duration(i) * gran).Unix(),
			ReqPerS:     rand.Float64(), //nolint:gosec // No need to crypto randomness in this test.
			RequestErrs: 1,
		}
		pnts = append(pnts, d)
	}

	return pnts
}
