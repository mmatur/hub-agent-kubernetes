package expr_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/neo-agent/pkg/acp/jwt/expr"
)

func TestPluckClaims(t *testing.T) {
	q := map[string]string{
		"String":       "string",
		"String-Slice": "string-slice",
		"Number":       "number",
		"Number-Slice": "number-slice",
		"Bool":         "bool",
		"Bool-Slice":   "bool-slice",
		"Nested-Name":  "nested.name",
		"Unknown":      "unknown",
	}

	claims := `{
		"string": "string",
		"string-slice": ["string", "slice"],
		"number": 42,
		"number-slice": [32.332, 32.333],
		"bool": true,
		"bool-slice": [true, false],
		"nested": {"name": "lol"}
	}`

	want := map[string][]string{
		"String":       {"string"},
		"String-Slice": {"string", "slice"},
		"Number":       {"42"},
		"Number-Slice": {"32.332", "32.333"},
		"Bool":         {"true"},
		"Bool-Slice":   {"true", "false"},
		"Nested-Name":  {"lol"},
	}

	var parsedClaims map[string]interface{}
	dec := json.NewDecoder(bytes.NewBuffer([]byte(claims)))
	dec.UseNumber()
	err := dec.Decode(&parsedClaims)
	require.NoError(t, err)

	got, err := expr.PluckClaims(q, parsedClaims)
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

func TestPluckClaims_FailsOnUnsupportedNestedTypes(t *testing.T) {
	q := map[string]string{
		"String": "bug",
	}

	claims := `{
		"bug": [{}]
	}
	`
	var parsedClaims map[string]interface{}
	dec := json.NewDecoder(bytes.NewBuffer([]byte(claims)))
	dec.UseNumber()
	err := dec.Decode(&parsedClaims)
	require.NoError(t, err)

	_, err = expr.PluckClaims(q, parsedClaims)
	assert.Error(t, err)
}
