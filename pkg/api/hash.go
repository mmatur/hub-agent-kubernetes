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

package api

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"hash/fnv"
	"sort"

	"golang.org/x/exp/constraints"
)

// sortedMap is a map sorted by key. This map can safely be used for computing a hash.
type sortedMap[T constraints.Ordered] []keyValue[T]

type keyValue[T constraints.Ordered] struct {
	Key   T
	Value any
}

// newSortedMap creates a new sorted version of the given map.
func newSortedMap[T constraints.Ordered](source map[T]string) sortedMap[T] {
	var keyValues sortedMap[T]
	for key, value := range source {
		keyValues = append(keyValues, keyValue[T]{Key: key, Value: value})
	}

	sort.Slice(keyValues, func(i, j int) bool {
		return keyValues[i].Key < keyValues[j].Key
	})

	return keyValues
}

// sum returns the version of the provided data.
func sum(a any) ([]byte, error) {
	var buff bytes.Buffer
	encoder := gob.NewEncoder(&buff)

	if err := encoder.Encode(a); err != nil {
		return nil, fmt.Errorf("encode data: %w", err)
	}

	hash := fnv.New128a()
	hash.Write(buff.Bytes())

	return hash.Sum(nil), nil
}
