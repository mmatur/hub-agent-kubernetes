/*
Copyright (C) 2022 Traefik Labs

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
	"sync"
	"time"
)

type tableInfo struct {
	Name     string
	MinCount int
	RollUp   time.Duration
	Next     string
}

type tableKey struct {
	EdgeIngress string
	Ingress     string
	Service     string
}

func toTableKey(grp DataPointGroup) tableKey {
	return tableKey{
		EdgeIngress: grp.EdgeIngress,
		Ingress:     grp.Ingress,
		Service:     grp.Service,
	}
}

// WaterMarks contain low water marks for a table.
type WaterMarks map[tableKey]int

// Store is a metrics store.
type Store struct {
	tables []tableInfo

	mu    sync.RWMutex
	data  map[string]map[tableKey]DataPoints
	marks map[string]WaterMarks

	// NowFunc is the function used to test time.
	nowFunc func() time.Time
}

// NewStore returns metrics store.
func NewStore() *Store {
	tables := []tableInfo{
		{Name: "1m", MinCount: 10, RollUp: 10 * time.Minute, Next: "10m"},
		{Name: "10m", MinCount: 6, RollUp: time.Hour, Next: "1h"},
		{Name: "1h", MinCount: 24, RollUp: 24 * time.Hour, Next: "1d"},
		{Name: "1d", MinCount: 30, RollUp: 30 * 24 * time.Hour},
	}

	tbls := make(map[string]map[tableKey]DataPoints, len(tables))
	marks := make(map[string]WaterMarks, len(tables))
	for _, info := range tables {
		tbls[info.Name] = map[tableKey]DataPoints{}
		marks[info.Name] = map[tableKey]int{}
	}

	return &Store{
		tables:  tables,
		data:    tbls,
		marks:   marks,
		nowFunc: time.Now,
	}
}

// Populate populates the store with initial data points.
func (s *Store) Populate(tbl string, grps []DataPointGroup) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	table, ok := s.data[tbl]
	if !ok {
		return fmt.Errorf("table %q does not exist", tbl)
	}

	for _, v := range grps {
		key := toTableKey(v)
		if len(v.DataPoints) == 0 {
			continue
		}

		dataPoints := v.DataPoints
		sort.Slice(dataPoints, func(i, j int) bool {
			return dataPoints[i].Timestamp < dataPoints[j].Timestamp
		})
		table[key] = dataPoints
		s.marks[tbl][key] = len(dataPoints)
	}

	return nil
}

// Insert inserts a value for an ingress and service.
func (s *Store) Insert(svcs map[SetKey]DataPoint) {
	s.mu.Lock()
	defer s.mu.Unlock()

	table := s.data["1m"]

	for k, pnt := range svcs {
		key := tableKey(k)
		pnts := table[key]
		pnts = append(pnts, pnt)
		table[key] = pnts
	}
}

// ForEachFunc represents a function that will be called while iterating over a table.
// Each time this function is called, a unique ingress and service will
// be given with their set of points.
type ForEachFunc func(edgeIngr, ingr, svc string, pnts DataPoints)

// ForEach iterates over a table, executing fn for each row.
func (s *Store) ForEach(tbl string, fn ForEachFunc) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	table, ok := s.data[tbl]
	if !ok {
		return
	}

	for k, v := range table {
		fn(k.EdgeIngress, k.Ingress, k.Service, v)
	}
}

// ForEachUnmarked iterates over a table, executing fn for each row that
// has not been marked.
func (s *Store) ForEachUnmarked(tbl string, fn ForEachFunc) WaterMarks {
	s.mu.RLock()
	defer s.mu.RUnlock()

	table, ok := s.data[tbl]
	if !ok {
		return nil
	}

	newMarks := make(WaterMarks)
	for k, v := range table {
		newMarks[k] = len(v)

		mark := s.marks[tbl][k]
		if mark == len(v) {
			continue
		}

		fn(k.EdgeIngress, k.Ingress, k.Service, v[mark:])
	}

	return newMarks
}

// CommitMarks sets the new low water marks for a table.
func (s *Store) CommitMarks(tbl string, marks WaterMarks) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.marks[tbl]
	if !ok {
		return
	}

	s.marks[tbl] = marks
}

// RollUp creates combines data points.
//
// Rollup goes through each table and aggregates the points into complete
// points of the next granularity, if that point does not already exist in the
// next table.
func (s *Store) RollUp() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, tblInfo := range s.tables {
		tbl, gran, dest := tblInfo.Name, tblInfo.RollUp, tblInfo.Next
		if dest == "" {
			continue
		}

		rollUpEnd := s.nowFunc().UTC().Truncate(gran).Unix()

		res := map[tableKey]map[int64]DataPoints{}
		for key, data := range s.data[tbl] {
			destPnts := s.data[dest][key]

			for _, pnt := range data {
				// As data points are in asc order, when the current roll up period is reached, we can stop.
				if pnt.Timestamp >= rollUpEnd {
					continue
				}

				destTS := pnt.Timestamp - (pnt.Timestamp % int64(gran/time.Second))
				// Check if the timestamp exists in the destination table.
				if i, _ := destPnts.Get(destTS); i >= 0 {
					continue
				}

				tsPnts, ok := res[key]
				if !ok {
					tsPnts = map[int64]DataPoints{}
				}
				pnts := tsPnts[destTS]
				pnts = append(pnts, pnt)
				tsPnts[destTS] = pnts
				res[key] = tsPnts
			}
		}

		// Insert new computed points into dest.
		table := s.data[dest]
		for key, tsPnts := range res {
			for ts, pnts := range tsPnts {
				pnt := pnts.Aggregate()
				pnt.Timestamp = ts

				destPnts := table[key]
				destPnts = append(destPnts, pnt)
				table[key] = destPnts
			}
		}
	}
}

// Cleanup removes old data points no longer needed for roll up.
func (s *Store) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, tblInfo := range s.tables {
		tbl, count := tblInfo.Name, tblInfo.MinCount

		for k, data := range s.data[tbl] {
			pnts := data
			idx := len(pnts) - count
			if idx < 1 {
				continue
			}

			mark := s.marks[tbl][k]
			if idx > mark {
				idx = mark
			}

			copy(pnts[0:], pnts[idx:])
			pnts = pnts[0 : len(pnts)-idx]
			s.data[tbl][k] = pnts
			s.marks[tbl][k] = mark - idx
		}
	}
}
