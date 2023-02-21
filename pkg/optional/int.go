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

package optional

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Int represents an optional int.
type Int struct {
	i   int
	set bool
}

// NewInt returns a new optional int set to i.
func NewInt(i int) *Int {
	return &Int{
		i:   i,
		set: true,
	}
}

// NewNilInt returns a new optional int set to nil.
func NewNilInt() *Int {
	return &Int{}
}

// Set returns whether the int value has been set.
func (i *Int) Set() bool {
	if i == nil {
		return false
	}
	return i.set
}

// Int returns the integer value when set, otherwise it panics.
func (i *Int) Int() int {
	if !i.Set() {
		panic("value not set")
	}
	return i.i
}

// IntOrDefault returns the integer value when set, otherwise returns the default value.
func (i *Int) IntOrDefault(v int) int {
	if !i.Set() {
		return v
	}
	return i.i
}

// String implements the fmt.Stringer interface.
func (i *Int) String() string {
	if i == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d %t", i.i, i.set)
}

// MarshalJSON implements the json.Marshaler interface.
func (i *Int) MarshalJSON() ([]byte, error) {
	if !i.set {
		return []byte("null"), nil
	}

	return json.Marshal(i.i)
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (i *Int) UnmarshalJSON(data []byte) error {
	var err error
	i.i, err = strconv.Atoi(string(data))
	if err != nil {
		return err
	}

	i.set = true

	return nil
}
