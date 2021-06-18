package quota

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// Quota allows to enforce quotas.
type Quota struct {
	mu sync.RWMutex

	max      int
	acquired map[string]int
	pending  map[string]int
}

// New returns a new quota enforcer.
func New(max int) *Quota {
	return &Quota{
		max:      max,
		acquired: make(map[string]int),
		pending:  make(map[string]int),
	}
}

// Tx represents a quota transaction.
// Transactions must always be either committed or rolled back.
// Note that neither of these actions can fail, only the creation of a transaction can.
type Tx struct {
	q    *Quota
	id   string
	done int32 // Accessed atomically.
}

// Tx creates a transaction for the given resource.
// An error is returned if we're not sure it could be committed. This means that pending
// transactions can prevent the creation of a new transaction even if they end up rolled back.
func (q *Quota) Tx(resourceID string, amount int) (*Tx, error) {
	q.mu.RLock()
	if q.max <= 0 {
		q.mu.RUnlock()
		return nil, errors.New("feature disabled")
	}

	if _, ok := q.pending[resourceID]; ok {
		q.mu.RUnlock()
		return nil, fmt.Errorf("found pending quota transaction for resource %q", resourceID)
	}

	if amount == 0 {
		q.mu.RUnlock()
		return &Tx{q: q, id: resourceID}, nil
	}

	if amount > q.max {
		taken := q.taken()
		err := fmt.Errorf("quota exceeded: want %d; taken %d; %d left", amount, taken, q.max-taken+q.acquired[resourceID])
		q.mu.RUnlock()
		return nil, err
	}

	diff := amount - q.acquired[resourceID]
	if err := q.allowed(diff); err != nil {
		q.mu.RUnlock()
		return nil, err
	}
	q.mu.RUnlock()

	q.mu.Lock()
	// We need to re-check if the transaction is still allowed since we released the lock in the meantime.
	diff = amount - q.acquired[resourceID]
	if err := q.allowed(diff); err != nil {
		q.mu.Unlock()
		return nil, err
	}

	q.pending[resourceID] = amount
	q.mu.Unlock()

	return &Tx{q: q, id: resourceID}, nil
}

func (q *Quota) taken() int {
	var taken int
	for _, n := range q.acquired {
		taken += n
	}
	for _, n := range q.pending {
		taken += n
	}
	return taken
}

func (q *Quota) allowed(diff int) error {
	taken := q.taken()
	if taken+diff > q.max {
		err := fmt.Errorf("quota exceeded: want %d; taken %d; %d left", diff, taken, q.max-taken)
		return err
	}
	return nil
}

// Commit commits the transaction, applying its changes.
func (t *Tx) Commit() {
	if atomic.LoadInt32(&t.done) == 1 {
		return
	}
	atomic.StoreInt32(&t.done, 1)

	t.q.mu.Lock()
	defer t.q.mu.Unlock()

	t.q.acquired[t.id] = t.q.pending[t.id]
	delete(t.q.pending, t.id)
}

// Rollback rollbacks the transaction, discarding its changes.
func (t *Tx) Rollback() {
	if atomic.LoadInt32(&t.done) == 1 {
		return
	}
	atomic.StoreInt32(&t.done, 1)

	t.q.mu.Lock()
	defer t.q.mu.Unlock()

	delete(t.q.pending, t.id)
}
