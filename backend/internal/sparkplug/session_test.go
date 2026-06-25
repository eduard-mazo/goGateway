package sparkplug

import "testing"

func bdMetric(n uint64) Metric { return Metric{Name: "bdSeq", dblVal: float64(n), hasDbl: true} }

// SetBirth must flag a bdSeq regression ONLY when the node was already online —
// the duplicate-node-id signal — and never on first birth, advance, equal
// (commanded rebirth), or a reconnect that first went through NDEATH.
func TestSetBirthBdSeqRegression(t *testing.T) {
	s := &NodeSession{aliasMap: map[uint64]string{}}

	// First birth (offline → online): never a regression.
	if _, reg := s.SetBirth(&Payload{Seq: 0, Metrics: []Metric{bdMetric(5)}}); reg {
		t.Fatal("first birth must not flag regression")
	}
	// Advance while online: fine.
	if _, reg := s.SetBirth(&Payload{Seq: 0, Metrics: []Metric{bdMetric(6)}}); reg {
		t.Fatal("advancing bdSeq must not flag regression")
	}
	// Equal while online (commanded rebirth reuses the session): fine.
	if _, reg := s.SetBirth(&Payload{Seq: 0, Metrics: []Metric{bdMetric(6)}}); reg {
		t.Fatal("equal bdSeq must not flag regression")
	}
	// Lower while online: DUPLICATE signal.
	if bd, reg := s.SetBirth(&Payload{Seq: 0, Metrics: []Metric{bdMetric(2)}}); !reg || bd != 2 {
		t.Fatalf("lower bdSeq while online must flag regression (bd=%d reg=%v)", bd, reg)
	}

	// Reconnect path: NDEATH (→offline) then a lower/reset bdSeq is NOT a
	// regression, since the old session ended first.
	s.SetDeath()
	if _, reg := s.SetBirth(&Payload{Seq: 0, Metrics: []Metric{bdMetric(0)}}); reg {
		t.Fatal("birth after NDEATH must not flag regression (legit reconnect)")
	}
}
