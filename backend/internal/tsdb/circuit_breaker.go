package tsdb

import (
	"sync"
	"time"
)

type cbState int

const (
	cbClosed   cbState = iota // normal operation
	cbOpen                    // failing — reject fast
	cbHalfOpen                // probing recovery
)

// CircuitBreaker is a 3-state FSM per backend.
// Transitions: Closed→Open after threshold failures; Open→HalfOpen after
// openTimeout; HalfOpen→Closed after successReq successes; HalfOpen→Open on failure.
type CircuitBreaker struct {
	mu          sync.Mutex
	state       cbState
	failures    int
	successes   int
	threshold   int
	successReq  int
	openTimeout time.Duration
	openedAt    time.Time
}

func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		threshold:   5,
		successReq:  2,
		openTimeout: 30 * time.Second,
	}
}

// Allow returns true if a request should be attempted.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case cbClosed:
		return true
	case cbOpen:
		if time.Since(cb.openedAt) >= cb.openTimeout {
			cb.state = cbHalfOpen
			cb.successes = 0
			return true
		}
		return false
	case cbHalfOpen:
		return cb.successes == 0
	}
	return false
}

func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state == cbOpen
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	if cb.state == cbHalfOpen {
		cb.successes++
		if cb.successes >= cb.successReq {
			cb.state = cbClosed
		}
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	if cb.state == cbHalfOpen || cb.failures >= cb.threshold {
		cb.state = cbOpen
		cb.openedAt = time.Now()
		cb.failures = 0
	}
}

func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case cbClosed:
		return "closed"
	case cbOpen:
		return "open"
	case cbHalfOpen:
		return "half-open"
	}
	return "unknown"
}
