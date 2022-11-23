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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/vulcand/predicate"
)

// Predicate represents a function that can be evaluated to get the result of an expression.
type Predicate func(a map[string]interface{}) bool

// Parse returns a predicate from the given expression.
func Parse(expr string) (Predicate, error) {
	parser, err := predicate.NewParser(predicate.Def{
		Operators: predicate.Operators{
			AND: andFunc,
			OR:  orFunc,
			NOT: notFunc,
		},
		Functions: map[string]interface{}{
			"Equals":        equals,
			"Prefix":        prefix,
			"Contains":      contains,
			"SplitContains": splitContains,
			"Ohubf":         ohubf,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create parser: %w", err)
	}

	p, err := parser.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("unable to parse expression: %w", err)
	}

	return p.(Predicate), nil
}

func andFunc(a, b Predicate) Predicate {
	return func(v map[string]interface{}) bool {
		return a(v) && b(v)
	}
}

func orFunc(a, b Predicate) Predicate {
	return func(v map[string]interface{}) bool {
		return a(v) || b(v)
	}
}

func notFunc(a Predicate) Predicate {
	return func(v map[string]interface{}) bool {
		return !a(v)
	}
}

func equals(claimName, expected string) Predicate {
	return func(claims map[string]interface{}) bool {
		claim, ok := resolve(claimName, claims)
		if !ok {
			return false
		}

		return matches(claim, expected)
	}
}

func prefix(claimName, expected string) Predicate {
	return func(claims map[string]interface{}) bool {
		claim, ok := resolve(claimName, claims)
		if !ok {
			return false
		}

		str, ok := claim.(string)
		if !ok {
			return false
		}

		return strings.HasPrefix(str, expected)
	}
}

func contains(claimName, expected string) Predicate {
	return func(claims map[string]interface{}) bool {
		claim, ok := resolve(claimName, claims)
		if !ok {
			return false
		}

		switch val := claim.(type) {
		case []interface{}:
			for _, v := range val {
				if matches(v, expected) {
					return true
				}
			}
			return false

		case string:
			return strings.Contains(val, expected)

		default:
			return false
		}
	}
}

func splitContains(claimName, sep, expected string) Predicate {
	return func(claims map[string]interface{}) bool {
		claim, ok := resolve(claimName, claims)
		if !ok {
			return false
		}

		str, ok := claim.(string)
		if !ok {
			return false
		}

		arr := strings.Split(str, sep)
		for _, v := range arr {
			if matches(strings.TrimSpace(v), expected) {
				return true
			}
		}
		return false
	}
}

func ohubf(claimName string, expected ...string) Predicate {
	return func(claims map[string]interface{}) bool {
		claim, ok := resolve(claimName, claims)
		if !ok {
			return false
		}

		switch val := claim.(type) {
		case string:
			for _, exp := range expected {
				if val == exp {
					return true
				}
			}
			return false

		case json.Number:
			for _, exp := range expected {
				if val.String() == exp {
					return true
				}
			}
			return false

		default:
			return false
		}
	}
}

func matches(v interface{}, expected string) bool {
	switch val := v.(type) {
	case string:
		return val == expected

	case json.Number:
		return val.String() == expected

	case bool:
		return strconv.FormatBool(val) == expected

	default:
		return false
	}
}

// resolve fetches the value addressed by claimName in the given claims map. It handles nesting.
func resolve(claimName string, claims map[string]interface{}) (interface{}, bool) {
	parts := split(claimName, '.')
	v := claims

	for idx, part := range parts {
		got, ok := v[part]
		if !ok {
			return nil, false
		}

		isLast := idx == len(parts)-1

		switch val := got.(type) {
		case map[string]interface{}:
			if isLast {
				return nil, false
			}

			v = val
			continue
		default:
			if !isLast {
				return nil, false
			}

			return val, true
		}
	}

	return nil, false
}

// split splits on given sep ignoring escaped separators.
func split(s string, sep rune) []string {
	var (
		arr     []string
		current strings.Builder
	)

	rs := []rune(s)
	for idx := 0; idx < len(rs); idx++ {
		r := rs[idx]
		switch r {
		case '\\':
			if idx+1 == len(s) {
				_, _ = current.WriteRune(r)
				break
			}

			// if the next rune is not `.` or `\` then we can write the `\`.
			if rs[idx+1] != '.' && rs[idx+1] != '\\' {
				_, _ = current.WriteRune(r)
			}

			idx++
			_, _ = current.WriteRune(rs[idx])
		case sep:
			arr = append(arr, current.String())
			current.Reset()

		default:
			_, _ = current.WriteRune(r)
		}
	}

	arr = append(arr, current.String())

	return arr
}
