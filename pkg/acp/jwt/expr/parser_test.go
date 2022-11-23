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

package expr

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCustomClaims(t *testing.T) {
	tests := []struct {
		desc   string
		claims string
		expr   string
		want   bool
	}{
		{
			desc:   "simple expression",
			claims: `{"grp":"admin"}`,
			expr:   "Equals(`grp`, `admin`)",
			want:   true,
		},
		{
			desc:   "simple expression with numbers",
			claims: `{"int":1512435789, "flt":1.512435789}`,
			expr:   "Equals(`int`, `1512435789`) && Equals(`flt`, `1.512435789`)",
			want:   true,
		},
		{
			desc:   "simple expression with boolean",
			claims: `{"active":true}`,
			expr:   "Equals(`active`, `true`)",
			want:   true,
		},
		{
			desc:   "simple AND expression",
			claims: `{"grp":"admin","scope":"deploy"}`,
			expr:   "Equals(`grp`, `admin`) && Equals(`scope`, `deploy`)",
			want:   true,
		},
		{
			desc:   "simple AND expression (false)",
			claims: `{"grp":"dev","scope":"deploy"}`,
			expr:   "Equals(`grp`, `admin`) && Equals(`scope`, `deploy`)",
			want:   false,
		},
		{
			desc:   "complex expression",
			claims: `{"grp":"dev","scope":"deploy"}`,
			expr:   "(Equals(`grp`, `admin`) || Equals(`grp`, `dev`)) && Equals(`scope`, `deploy`)",
			want:   true,
		},
		{
			desc:   "nested claim",
			claims: `{"grp":"dev","user":{"name":"john","role":"developer"}}`,
			expr:   "(Equals(`user.name`, `john`))",
			want:   true,
		},
		{
			desc:   "nested non exiting claim",
			claims: `{"grp":"dev","user":{"name":"john","role":"developer"}}`,
			expr:   "(Equals(`user.fullname`, `john`))",
			want:   false,
		},
		{
			desc:   "neted claim referring an unknown nested value",
			claims: `{"grp":"dev","user":{"name":"john","role":"developer"}}`,
			expr:   "(Equals(`user.role.foo`, `john`))",
			want:   false,
		},
		{
			desc:   "neted claim referring a nested object",
			claims: `{"grp":"dev","user":{"name":"john","role":{}}}`,
			expr:   "(Equals(`user.role`, `john`))",
			want:   false,
		},
		{
			desc:   "contains expression",
			claims: `{"grp":["admin", "dev"]}`,
			expr:   "Contains(`grp`, `admin`)",
			want:   true,
		},
		{
			desc:   "contains expression with numbers",
			claims: `{"grp":[500, 900]}`,
			expr:   "Contains(`grp`, `900`)",
			want:   true,
		},
		{
			desc:   "contains nested  expression",
			claims: `{"grp":"dev","user":{"name":"john","role":["developer", "admin", "product"]}}`,
			expr:   "Contains(`user.role`, `admin`)",
			want:   true,
		},
		{
			desc:   "contains string",
			claims: `{"scope":"foo bar baz bat"}`,
			expr:   "Contains(`scope`, `baz`)",
			want:   true,
		},
		{
			desc:   "contains nested string",
			claims: `{"grp":"dev","user":{"name":"john","role":"developer admin product"}}`,
			expr:   "Contains(`user.role`, `admin`)",
			want:   true,
		},
		{
			desc:   "prefix string",
			claims: `{"grp":"example.com/foo/bar"}`,
			expr:   "Prefix(`grp`, `example.com/foo`)",
			want:   true,
		},
		{
			desc:   "prefix string",
			claims: `{"grp":"example.com/foo/bar"}`,
			expr:   "Prefix(`grp`, `example.com/bar`)",
			want:   false,
		},
		{
			desc:   "nested prefix string",
			claims: `{"grp":"dev","user":{"name":"john","url":"example.com/bar/bozo"}}`,
			expr:   "Prefix(`user.url`, `example.com/bar`)",
			want:   true,
		},
		{
			desc:   "splitContains space",
			claims: `{"scope":"foo bar baz bat"}`,
			expr:   "SplitContains(`scope`, ` `, `baz`)",
			want:   true,
		},
		{
			desc:   "splitContains comma",
			claims: `{"scope":"foo,bar, baz , bat"}`,
			expr:   "SplitContains(`scope`, `,`, `baz`)",
			want:   true,
		},
		{
			desc:   "splitContains comma and semicolon",
			claims: `{"scope":"foo,;bar,; baz ,; bat"}`,
			expr:   "SplitContains(`scope`, `,;`, `baz`)",
			want:   true,
		},
		{
			desc:   "splitContains nested",
			claims: `{"grp":"dev","user":{"name":"bruce","roles": "foo bar buz batman"}}`,
			expr:   "SplitContains(`user.roles`, ` `,  `batman`)",
			want:   true,
		},
		{
			desc:   "ohubf expression",
			claims: `{"grp":"admin"}`,
			expr:   "Ohubf(`grp`, `admin`, `dev`)",
			want:   true,
		},
		{
			desc:   "ohubf expression with numbers",
			claims: `{"gid":500}`,
			expr:   "Ohubf(`gid`, `500`, `900`)",
			want:   true,
		},
		{
			desc:   "ohubf expression with nested value",
			claims: `{"grp":"dev","user":{"name":"bruce","role": "batman"}}`,
			expr:   "Ohubf(`user.role`, `jocker`, `batman`)",
			want:   true,
		},
		{
			desc:   "handles escaped dots",
			claims: `{"grp":"dev","user":{"full.name":"bruce","role": "batman"}}`,
			expr:   "Equals(`user.full\\.name`, `bruce`)",
			want:   true,
		},
		{
			desc:   "handles escaped escape characters",
			claims: `{"grp":"dev","user":{"full\\": {"name":"bruce","role": "batman"}}}`,
			expr:   "Equals(`user.full\\\\.name`, `bruce`)",
			want:   true,
		},
		{
			desc:   "handles escape characters at end of string",
			claims: `{"grp":"dev","user":{"full": {"name\\":"bruce","role": "batman"}}}`,
			expr:   "Equals(`user.full.name\\`, `bruce`)",
			want:   true,
		},
		{
			desc:   "handles escape characters at end of string",
			claims: `{"grp":"dev","user":{"ful\\l": {"name":"bruce","role": "batman"}}}`,
			expr:   "Equals(`user.ful\\l.name`, `bruce`)",
			want:   true,
		},
		{
			desc:   "handles empty claimName",
			claims: `{"grp":"dev","user":{"full": {"name":"bruce","role": "batman"}}}`,
			expr:   "Equals(``, `bruce`)",
			want:   false,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			pred, err := Parse(test.expr)
			require.NoError(t, err)

			var claims map[string]interface{}
			dec := json.NewDecoder(bytes.NewReader([]byte(test.claims)))
			dec.UseNumber()
			err = dec.Decode(&claims)
			require.NoError(t, err)

			assert.Equal(t, test.want, pred(claims))
		})
	}
}
