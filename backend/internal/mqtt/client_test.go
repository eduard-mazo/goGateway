package mqtt

import (
	"testing"
	"time"
)

// recordDuplicateSuspect upserts per node, dedupes reasons, and surfaces through
// Status() so the operator/UI sees the collision.
func TestDuplicateSuspectRegistry(t *testing.T) {
	m := &Manager{}
	m.recordDuplicateSuspect("EPM_SOAK", "edge-1", "rebirth-storm")
	m.recordDuplicateSuspect("EPM_SOAK", "edge-1", "bdseq-regression")
	m.recordDuplicateSuspect("EPM_SOAK", "edge-1", "rebirth-storm") // dup reason
	m.recordDuplicateSuspect("EPM_SOAK", "edge-2", "rebirth-storm")

	got := m.duplicateSuspects()
	if len(got) != 2 {
		t.Fatalf("want 2 suspect nodes, got %d", len(got))
	}
	// Sorted by group/node → edge-1 first.
	e1 := got[0]
	if e1.Node != "edge-1" || e1.Count != 3 {
		t.Fatalf("edge-1: node=%s count=%d (want edge-1/3)", e1.Node, e1.Count)
	}
	if len(e1.Reasons) != 2 {
		t.Fatalf("edge-1 reasons should dedupe to 2, got %v", e1.Reasons)
	}
	if s := m.Status(); len(s.Suspects) != 2 {
		t.Fatalf("Status should surface 2 suspects, got %d", len(s.Suspects))
	}
}

// detectRebirthStorm should stay quiet under the threshold, warn once when a
// sustained run within the window indicates a duplicate node id, rate-limit
// repeat warnings, and forget hits that age out of the window.
func TestDetectRebirthStorm(t *testing.T) {
	m := &Manager{}
	const key = "EPM_SOAK/edge-1"
	base := time.Unix(1_700_000_000, 0)

	// stormThreshold-1 requests in the window: no warning yet.
	for i := 0; i < stormThreshold-1; i++ {
		if m.detectRebirthStorm(key, base.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("unexpected storm warning at request %d (below threshold)", i+1)
		}
	}

	// The threshold-th request trips it exactly once.
	if !m.detectRebirthStorm(key, base.Add(stormThreshold*time.Second)) {
		t.Fatal("expected storm warning at threshold")
	}
	// Immediately again → rate-limited, no second warning.
	if m.detectRebirthStorm(key, base.Add(stormThreshold*time.Second+time.Second)) {
		t.Fatal("warning should be rate-limited within stormWarnInterval")
	}

	// A different node is tracked independently and stays quiet on one hit.
	if m.detectRebirthStorm("EPM_SOAK/edge-9", base) {
		t.Fatal("a single hit on a fresh node must not warn")
	}

	// Far in the future the window has emptied → one hit must not warn.
	future := base.Add(stormWindow + stormWarnInterval + time.Hour)
	if m.detectRebirthStorm(key, future) {
		t.Fatal("hits should age out of the window; one fresh hit must not warn")
	}
}
