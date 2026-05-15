package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	monitorCap     = 1000
	maxPayloadStore = 2048 // max bytes stored per event for JSON/state payloads
)

// BrokerEvent is one captured MQTT message metadata record.
type BrokerEvent struct {
	ID          int64     `json:"id"`
	At          time.Time `json:"at"`
	Topic       string    `json:"topic"`
	PayloadSize int       `json:"size"`
	Kind        string    `json:"kind"`             // "sparkplug", "ssfv", "json", "state"
	Payload     string    `json:"payload,omitempty"` // raw text for json/ssfv/state; empty for sparkplug
}

// BrokerMonitor is a thread-safe ring buffer for recent broker events that
// fans out new events to registered SSE clients.
type BrokerMonitor struct {
	mu    sync.Mutex
	ring  [monitorCap]BrokerEvent
	head  int   // next-write index
	count int   // fill level 0..monitorCap
	total int64 // monotonic ID counter
	subs  map[chan BrokerEvent]struct{}
}

func NewBrokerMonitor() *BrokerMonitor {
	return &BrokerMonitor{subs: make(map[chan BrokerEvent]struct{})}
}

// Push records an MQTT message and fans it out to live SSE clients.
// Never blocks: slow clients are dropped rather than back-pressuring the MQTT goroutine.
// For json, ssfv, and state kinds the raw payload is stored (truncated at maxPayloadStore).
// Sparkplug payloads are binary protobuf and not stored.
func (b *BrokerMonitor) Push(topic, kind string, payload []byte) {
	var payloadStr string
	switch kind {
	case "json", "ssfv", "state":
		if len(payload) <= maxPayloadStore {
			payloadStr = string(payload)
		} else {
			payloadStr = string(payload[:maxPayloadStore]) + "\n…(truncado)"
		}
	}

	b.mu.Lock()
	b.total++
	ev := BrokerEvent{
		ID:          b.total,
		At:          time.Now(),
		Topic:       topic,
		PayloadSize: len(payload),
		Kind:        kind,
		Payload:     payloadStr,
	}
	b.ring[b.head] = ev
	b.head = (b.head + 1) % monitorCap
	if b.count < monitorCap {
		b.count++
	}
	subs := make([]chan BrokerEvent, 0, len(b.subs))
	for ch := range b.subs {
		subs = append(subs, ch)
	}
	b.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- ev:
		default: // slow client → drop, never block the MQTT goroutine
		}
	}
}

func (b *BrokerMonitor) subscribe() chan BrokerEvent {
	ch := make(chan BrokerEvent, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *BrokerMonitor) unsubscribe(ch chan BrokerEvent) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
}

// snapshot returns all buffered events in chronological order (oldest first).
func (b *BrokerMonitor) snapshot() []BrokerEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.count == 0 {
		return nil
	}
	out := make([]BrokerEvent, b.count)
	if b.count < monitorCap {
		copy(out, b.ring[:b.count])
	} else {
		// ring is full; head points to the oldest slot
		n := copy(out, b.ring[b.head:])
		copy(out[n:], b.ring[:b.head])
	}
	return out
}

// ServeSnapshot returns the last ≤1000 broker events as JSON.
func (b *BrokerMonitor) ServeSnapshot(w http.ResponseWriter, r *http.Request) {
	events := b.snapshot()
	if events == nil {
		events = []BrokerEvent{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events) //nolint:errcheck
}

// ServeStream is an SSE endpoint that pushes BrokerEvents in real-time.
// It does not replay history — use ServeSnapshot for initial page load.
// This handler must be registered outside any timeout middleware.
func (b *BrokerMonitor) ServeStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := b.subscribe()
	defer b.unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data) //nolint:errcheck
			flusher.Flush()
		}
	}
}
