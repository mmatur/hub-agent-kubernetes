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
	"fmt"
	"sort"
	"time"
)

// DataPointGroupIterator is capable of iterating over data point groups.
type DataPointGroupIterator interface {
	ForEach(tbl string, fn ForEachFunc)
}

// DataPointView provides a view for querying data points from a store.
type DataPointView struct {
	store DataPointGroupIterator

	nowFunc func() time.Time
}

// NewDataPointView creates a new DataPointView instance.
func NewDataPointView(s DataPointGroupIterator) *DataPointView {
	return &DataPointView{
		store:   s,
		nowFunc: time.Now,
	}
}

// FindByIngressAndService finds the data points for the traffic on the given service via the given ingress for the
// specified time range (inclusive).
func (v *DataPointView) FindByIngressAndService(table, ingress, service string, from, to time.Time) (DataPoints, error) {
	if to.Before(from) || to == from {
		return nil, nil
	}

	fromTS, toTS := from.Unix(), to.Unix()

	var (
		pointsInRange DataPoints
		groupFound    bool
		err           error
	)
	v.store.ForEach(table, func(_, ingr, svc string, points DataPoints) {
		if ingr != ingress || svc != service {
			return
		}
		if groupFound {
			err = fmt.Errorf("more than one data point group for table %q, ingress %q and service %q",
				table, ingress, service)
			return
		}

		groupFound = true

		// Filter points to only keep those in the given time range.
		for _, point := range points {
			if point.Timestamp < fromTS || point.Timestamp > toTS {
				continue
			}

			pointsInRange = append(pointsInRange, point)
		}
	})

	if err != nil {
		return nil, err
	}
	return pointsInRange, err
}

// FindByService finds the data points for the traffic on the given service for the specified time range (inclusive).
// If the traffic coming this service comes from multiple ingresses, the resulting data points will be an aggregated
// view of all these ingresses.
func (v *DataPointView) FindByService(table, service string, from, to time.Time) DataPoints {
	if to.Before(from) || to == from {
		return nil
	}

	fromTS, toTS := from.Unix(), to.Unix()

	var groups []DataPoints
	v.store.ForEach(table, func(_, _, svc string, points DataPoints) {
		if svc != service {
			return
		}

		// Filter points to only keep those in the given time range.
		var pointsInRange DataPoints
		for _, point := range points {
			if point.Timestamp < fromTS || point.Timestamp > toTS {
				continue
			}

			pointsInRange = append(pointsInRange, point)
		}

		groups = append(groups, pointsInRange)
	})

	return mergeGroups(groups)
}

// FindByIngress finds the data points for the traffic on the given ingress for the specified time range (inclusive).
func (v *DataPointView) FindByIngress(table, ingress string, from, to time.Time) DataPoints {
	if to.Before(from) || to == from {
		return nil
	}

	fromTS, toTS := from.Unix(), to.Unix()

	var groups []DataPoints
	v.store.ForEach(table, func(_, ingr, _ string, points DataPoints) {
		if ingr != ingress {
			return
		}

		// Filter points to only keep those in the given time range.
		var pointsInRange DataPoints
		for _, point := range points {
			if point.Timestamp < fromTS || point.Timestamp > toTS {
				continue
			}

			pointsInRange = append(pointsInRange, point)
		}

		groups = append(groups, pointsInRange)
	})

	return mergeGroups(groups)
}

// mergeGroups merges the data points of the given groups.
func mergeGroups(groups []DataPoints) DataPoints {
	if len(groups) == 0 {
		return nil
	}

	pointSums := make(map[int64]DataPoint)
	counts := make(map[int64]int64)

	for _, points := range groups {
		for _, point := range points {
			sum := pointSums[point.Timestamp]
			sum.Timestamp = point.Timestamp

			sum.Seconds += point.Seconds
			sum.Requests += point.Requests
			sum.RequestErrs += point.RequestErrs
			sum.RequestClientErrs += point.RequestClientErrs
			sum.ResponseTimeSum += point.ResponseTimeSum
			sum.ResponseTimeCount += point.ResponseTimeCount

			pointSums[point.Timestamp] = sum
			counts[point.Timestamp]++
		}
	}

	if len(pointSums) == 0 {
		return nil
	}

	points := make(DataPoints, 0, len(pointSums))
	for ts, point := range pointSums {
		count, ok := counts[ts]
		if !ok {
			continue
		}

		point.Seconds /= count
		point.ReqPerS = float64(point.Requests) / float64(point.Seconds)
		point.RequestErrPerS = float64(point.RequestErrs) / float64(point.Seconds)
		point.RequestClientErrPerS = float64(point.RequestClientErrs) / float64(point.Seconds)

		if point.Requests > 0 {
			point.RequestErrPercent = float64(point.RequestErrs) / float64(point.Requests)
			point.RequestClientErrPercent = float64(point.RequestClientErrs) / float64(point.Requests)
		}

		if point.ResponseTimeCount > 0 {
			point.AvgResponseTime = point.ResponseTimeSum / float64(point.ResponseTimeCount)
		}

		points = append(points, point)
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp < points[j].Timestamp
	})

	return points
}
