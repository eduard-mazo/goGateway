package worker

import (
	"math"
	"sync"
)

// DeadbandFilter suppresses micro-fluctuations by comparing each incoming
// value to the last dispatched value for the same (serverID, IOA) point.
//
// Two threshold modes:
//   - Absolute: skip if |new - last| < deadband_abs
//   - Percentage: skip if |new - last| < |last| * deadband_pct
//
// The stricter of the two thresholds wins (max of the two computed deltas).
// When both are zero the filter is a pass-through.
//
// The filter always passes the first value for a new point (cold start) and
// always passes quality changes regardless of value magnitude, so alarm
// conditions are never swallowed.
type DeadbandFilter struct {
	mu   sync.RWMutex
	last map[uint64]float64 // (serverID << 32 | uint32(ioa)) → last dispatched value
}

func NewDeadbandFilter() *DeadbandFilter {
	return &DeadbandFilter{last: make(map[uint64]float64)}
}

// Allow returns true when the new value should be forwarded.
// If it returns true it also stores the new value as the new baseline.
func (f *DeadbandFilter) Allow(serverID int64, ioa int, abs, pct, newVal float64) bool {
	if abs <= 0 && pct <= 0 {
		return true // no deadband configured — always pass
	}

	key := packKey(serverID, ioa)

	f.mu.RLock()
	prev, exists := f.last[key]
	f.mu.RUnlock()

	if !exists {
		f.mu.Lock()
		f.last[key] = newVal
		f.mu.Unlock()
		return true // first observation — always dispatch
	}

	diff := math.Abs(newVal - prev)
	absDelta := abs
	pctDelta := math.Abs(prev) * pct
	threshold := math.Max(absDelta, pctDelta)

	if diff < threshold {
		return false
	}

	f.mu.Lock()
	f.last[key] = newVal
	f.mu.Unlock()
	return true
}

func packKey(serverID int64, ioa int) uint64 {
	return uint64(serverID)<<32 | uint64(uint32(ioa))
}
