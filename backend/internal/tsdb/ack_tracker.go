package tsdb

import (
	"sync"
	"sync/atomic"
)

// ackTracker counts backend confirmations per WAL entry.
// When remaining reaches zero the WAL entry is deleted.
type ackTracker struct {
	mu      sync.Mutex
	entries map[uint64]*ackEntry
	wal     *WAL
}

type ackEntry struct {
	remaining atomic.Int32
}

func newAckTracker(wal *WAL) *ackTracker {
	return &ackTracker{
		entries: make(map[uint64]*ackEntry),
		wal:     wal,
	}
}

// Register creates a pending entry expecting count backend acks.
func (t *ackTracker) Register(walID uint64, count int) {
	e := &ackEntry{}
	e.remaining.Store(int32(count))
	t.mu.Lock()
	t.entries[walID] = e
	t.mu.Unlock()
}

// Ack records one backend completion; deletes WAL entry when all done.
func (t *ackTracker) Ack(walID uint64) {
	t.mu.Lock()
	e, ok := t.entries[walID]
	t.mu.Unlock()
	if !ok {
		return
	}
	if e.remaining.Add(-1) == 0 {
		t.wal.Ack(walID)
		t.mu.Lock()
		delete(t.entries, walID)
		t.mu.Unlock()
	}
}
