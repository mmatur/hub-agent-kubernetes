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
)

// PluckClaim returns the claim with a given name from a set of claims.
func PluckClaim(selection string, claims map[string]interface{}) ([]string, error) {
	claimVal, ok := resolve(selection, claims)
	if !ok {
		return nil, nil
	}

	var result []string
	switch val := claimVal.(type) {
	case []interface{}:
		for _, v := range val {
			strVal, err := toStr(v)
			if err != nil {
				return nil, err
			}

			result = append(result, strVal)
		}

	default:
		strVal, err := toStr(val)
		if err != nil {
			return nil, err
		}

		result = append(result, strVal)
	}

	return result, nil
}

// PluckClaims returns the claims with the given names from a set of claims.
func PluckClaims(selection map[string]string, claims map[string]interface{}) (map[string][]string, error) {
	result := make(map[string][]string, len(selection))

	for name, claim := range selection {
		res, err := PluckClaim(claim, claims)
		if err != nil {
			return nil, err
		}
		if len(res) == 0 {
			continue
		}
		result[name] = res
	}

	return result, nil
}

func toStr(val interface{}) (string, error) {
	switch v := val.(type) {
	case string:
		return v, nil

	case json.Number:
		return v.String(), nil

	case bool:
		return strconv.FormatBool(v), nil

	default:
		return "", fmt.Errorf("unsupported type %T", val)
	}
}
