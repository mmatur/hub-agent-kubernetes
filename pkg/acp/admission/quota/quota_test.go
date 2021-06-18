package quota

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuota_Tx(t *testing.T) {
	tests := []struct {
		desc     string
		allowed  int
		acquired map[string]int
		pending  map[string]int
		amount   int
		wantErr  assert.ErrorAssertionFunc
	}{
		{
			desc:     "creating a tx for 1 unit with 3 allowed is valid",
			allowed:  3,
			acquired: nil,
			pending:  nil,
			amount:   1,
			wantErr:  assert.NoError,
		},
		{
			desc:     "creating a tx for 2 units with 1 allowed is invalid",
			allowed:  1,
			acquired: nil,
			pending:  nil,
			amount:   2,
			wantErr:  assert.Error,
		},
		{
			desc:     "creating a tx for 1 unit with 3 allowed, 2 acquired is valid",
			allowed:  3,
			acquired: map[string]int{"ing2@ns": 2},
			pending:  nil,
			amount:   1,
			wantErr:  assert.NoError,
		},
		{
			desc:     "creating a tx for 1 unit with 3 allowed, 3 acquired is invalid",
			allowed:  3,
			acquired: map[string]int{"ing2@ns": 3},
			pending:  nil,
			amount:   1,
			wantErr:  assert.Error,
		},
		{
			desc:     "creating a tx for 1 unit with 3 allowed, 1 acquired, 1 pending is valid",
			allowed:  3,
			acquired: map[string]int{"ing2@ns": 1},
			pending:  map[string]int{"ing2@ns": 1},
			amount:   1,
			wantErr:  assert.NoError,
		},
		{
			desc:     "creating a tx for 1 unit with 4 allowed, 1 acquired, 3 pending is invalid",
			allowed:  4,
			acquired: map[string]int{"ing2@ns": 1},
			pending:  map[string]int{"ing2@ns": 1, "ing3@ns": 2},
			amount:   1,
			wantErr:  assert.Error,
		},
		{
			desc:     "creating a tx for 0 unit with 3 allowed is valid",
			allowed:  3,
			acquired: nil,
			pending:  nil,
			amount:   0,
			wantErr:  assert.NoError,
		},
		{
			desc:     "creating a tx for 0 unit with 0 allowed is invalid",
			allowed:  0,
			acquired: nil,
			pending:  nil,
			amount:   0,
			wantErr:  assert.Error,
		},
		{
			desc:     "creating a tx for 1 unit for a resource that already has a pending tx is invalid",
			allowed:  3,
			acquired: nil,
			pending:  map[string]int{"ing1@ns": 1},
			amount:   1,
			wantErr:  assert.Error,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			q := New(test.allowed)
			if test.acquired != nil {
				q.acquired = test.acquired
			}
			if test.pending != nil {
				q.pending = test.pending
			}

			pendingBeforeTx := q.pending["ing1@ns"]

			_, err := q.Tx("ing1@ns", test.amount)
			test.wantErr(t, err)

			if err == nil {
				assert.Equal(t, test.amount, q.pending["ing1@ns"])
			} else {
				assert.Equal(t, 0, q.pending["ing1@ns"]-pendingBeforeTx)
			}
		})
	}
}

func TestTx_Commit(t *testing.T) {
	q := New(3)

	tx, err := q.Tx("ing@ns", 1)
	require.NoError(t, err)
	require.Equal(t, 1, q.pending["ing@ns"])

	tx.Commit()
	assert.Equal(t, 1, q.acquired["ing@ns"])
	assert.Len(t, q.pending, 0)

	// Committing a committed transaction is a no-op.
	tx.Commit()
	assert.Equal(t, 1, q.acquired["ing@ns"])
	assert.Len(t, q.pending, 0)
}

func TestTx_Rollback(t *testing.T) {
	q := New(3)

	tx, err := q.Tx("ing@ns", 1)
	require.NoError(t, err)
	require.Equal(t, 1, q.pending["ing@ns"])

	tx.Rollback()
	assert.Equal(t, 0, q.acquired["ing@ns"])
	assert.Len(t, q.pending, 0)

	// Rolling back an already rolled back transaction is a no-op.
	tx.Rollback()
	assert.Equal(t, 0, q.acquired["ing@ns"])
	assert.Len(t, q.pending, 0)
}
