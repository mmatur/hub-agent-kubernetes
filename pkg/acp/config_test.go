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

package acp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildClaims(t *testing.T) {
	testCases := []struct {
		desc     string
		emails   []string
		expected string
	}{
		{
			desc: "empty",
		},
		{
			desc:     "1 user",
			emails:   []string{"email1"},
			expected: `Equals("email", "email1")`,
		},
		{
			desc:     "2 user",
			emails:   []string{"email1", "email2"},
			expected: `Equals("email", "email1") || Equals("email", "email2")`,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.expected, buildClaims(test.emails))
		})
	}
}
