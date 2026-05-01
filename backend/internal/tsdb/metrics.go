package tsdb

import (
	"sync"
	"time"
)

// rateTracker computes a per-second rate over a sliding window.
type rateTracker struct {
	window time.Duration
	mu     sync.Mutex
	slots  []rateSlot
	pos    int
}

type rateSlot struct {
	ts    time.Time
	count int64
}

func newRateTracker(window time.Duration) *rateTracker {
	size := int(window.Seconds())
	if size < 1 {
		size = 10
	}
	return &rateTracker{window: window, slots: make([]rateSlot, size)}
}

func (r *rateTracker) record(n int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().Truncate(time.Second)
	if r.slots[r.pos].ts.Equal(now) {
		r.slots[r.pos].count += n
	} else {
		r.pos = (r.pos + 1) % len(r.slots)
		r.slots[r.pos] = rateSlot{ts: now, count: n}
	}
}

func (r *rateTracker) rate() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := time.Now().Add(-r.window)
	var total int64
	for _, s := range r.slots {
		if s.ts.After(cutoff) {
			total += s.count
		}
	}
	return float64(total) / r.window.Seconds()
}
