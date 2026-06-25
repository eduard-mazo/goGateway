package api

import (
	"strings"
	"testing"
)

// An oversized sparkplug frame must be recorded as metadata only — never
// protobuf-decoded/JSON-marshalled on the hot path — and must not store a body
// larger than the cap.
func TestPushOversizedSparkplugSkipsDecode(t *testing.T) {
	b := NewBrokerMonitor()
	big := make([]byte, maxDecodeBytes+1)
	b.Push("spBv1.0/g/DDATA/n", "sparkplug", big, 0)

	evs := b.snapshot()
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	ev := evs[0]
	if ev.PayloadSize != len(big) {
		t.Errorf("PayloadSize = %d, want original wire size %d", ev.PayloadSize, len(big))
	}
	if !strings.Contains(ev.Payload, "demasiado grande") {
		t.Errorf("oversized frame should record a placeholder, got %q", ev.Payload)
	}
	if len(ev.Payload) > maxPayloadStore+64 {
		t.Errorf("stored body %d bytes exceeds cap", len(ev.Payload))
	}
}

// Non-sparkplug payloads over the store cap are truncated, not dropped.
func TestPushTruncatesLargeJSON(t *testing.T) {
	b := NewBrokerMonitor()
	big := []byte(strings.Repeat("x", maxPayloadStore*2))
	b.Push("some/json/topic", "json", big, 0)

	evs := b.snapshot()
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	if !strings.HasSuffix(evs[0].Payload, "…(truncado)") {
		t.Errorf("large json should be truncated with marker, got suffix %q", tail(evs[0].Payload))
	}
	if evs[0].PayloadSize != len(big) {
		t.Errorf("PayloadSize = %d, want %d", evs[0].PayloadSize, len(big))
	}
}

// The ring buffer keeps at most monitorCap events (bounded memory).
func TestRingBufferBounded(t *testing.T) {
	b := NewBrokerMonitor()
	for i := 0; i < monitorCap+50; i++ {
		b.Push("t", "state", []byte("x"), 0)
	}
	if got := len(b.snapshot()); got != monitorCap {
		t.Errorf("ring length = %d, want cap %d", got, monitorCap)
	}
}

func tail(s string) string {
	if len(s) > 16 {
		return s[len(s)-16:]
	}
	return s
}
