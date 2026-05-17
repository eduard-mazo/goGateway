package tsdb

import (
	"testing"
	"time"
)

func TestDedupBatch(t *testing.T) {
	t0 := time.Now().Truncate(time.Millisecond)
	t1 := t0.Add(5 * time.Millisecond)
	t2 := t0.Add(10 * time.Millisecond)

	pt := func(path string, ts time.Time, v float64) DataPoint {
		return DataPoint{
			Measurement: "test",
			Tags:        map[string]string{"signal_path": path},
			Fields:      map[string]float64{"value": v},
			Timestamp:   ts,
		}
	}

	tests := []struct {
		name    string
		input   []DataPoint
		wantLen int
	}{
		{
			name: "different timestamps same path all kept (b1/b2 scenario)",
			input: []DataPoint{
				pt("A/sig1", t0, 1),
				pt("A/sig1", t1, 2),
				pt("A/sig1", t2, 3),
			},
			wantLen: 3,
		},
		{
			name: "exact duplicate (same ts+path) collapsed to last",
			input: []DataPoint{
				pt("A/sig1", t0, 1),
				pt("A/sig1", t0, 2),
			},
			wantLen: 1,
		},
		{
			name: "mix: some dup ts, some unique ts",
			input: []DataPoint{
				pt("A/sig1", t0, 1),
				pt("A/sig1", t1, 2),
				pt("A/sig1", t0, 3), // duplicate of first
				pt("A/sig2", t0, 4),
			},
			wantLen: 3,
		},
		{
			name: "b3 scenario: single point per path",
			input: []DataPoint{
				pt("A/sig1", t0, 1),
				pt("A/sig2", t0, 2),
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupBatch(tt.input)
			if len(got) != tt.wantLen {
				t.Errorf("got %d points, want %d", len(got), tt.wantLen)
			}
		})
	}

	// Verify last-value-wins for exact duplicate
	dup := []DataPoint{pt("A/sig1", t0, 1), pt("A/sig1", t0, 99)}
	out := dedupBatch(dup)
	if out[0].Fields["value"] != 99 {
		t.Errorf("expected last value 99, got %g", out[0].Fields["value"])
	}
}
