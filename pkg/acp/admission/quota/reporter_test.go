package quota

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReporter_Run(t *testing.T) {
	q := New(999)

	evt := make(chan int)
	pc := platformClientMock(func(n int) error {
		evt <- n
		return nil
	})

	r := NewReporter(pc, q, time.Microsecond)
	go r.Run(context.Background())

	var calledWith int
	select {
	case calledWith = <-evt:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first call")
	}

	assert.Equal(t, 0, calledWith)

	tx, err := q.Tx("id", 3)
	require.NoError(t, err)
	tx.Commit()

	select {
	case calledWith = <-evt:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second call")
	}

	assert.Equal(t, 3, calledWith)

	tx, err = q.Tx("id", 5)
	require.NoError(t, err)
	tx.Commit()

	select {
	case calledWith = <-evt:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for third call")
	}

	assert.Equal(t, 5, calledWith)
}

type platformClientMock func(n int) error

func (m platformClientMock) ReportSecuredRoutesInUse(_ context.Context, n int) error {
	return m(n)
}
