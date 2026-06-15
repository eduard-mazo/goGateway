package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"goGateway/internal/sparkplug"
)

const (
	monitorCap      = 1000
	maxPayloadStore = 2048 // max bytes of decoded text stored per event
	// maxDecodeBytes caps the wire size we will protobuf-decode + JSON-marshal
	// for the monitor. Push runs synchronously on the MQTT receive goroutine, so
	// an unbounded decode of a huge NBIRTH/DDATA frame would stall ingestion and
	// allocate megabytes only to truncate to maxPayloadStore. Oversized frames
	// are recorded with metadata (topic/size/kind) but no decoded body.
	maxDecodeBytes = 128 * 1024
)

// BrokerEvent is one captured MQTT message metadata record.
type BrokerEvent struct {
	ID          int64     `json:"id"`
	At          time.Time `json:"at"`
	Topic       string    `json:"topic"`
	PayloadSize int       `json:"size"`
	Kind        string    `json:"kind"`               // "sparkplug", "ssfv", "json", "state"
	Payload     string    `json:"payload,omitempty"`  // decoded text; JSON for sparkplug, raw for others
	SsfvHits    int       `json:"ssfvHits,omitempty"` // metrics forwarded to SSFV (sparkplug only)
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

	once sync.Once
	done chan struct{} // closed by Close() to unblock all ServeStream goroutines
}

func NewBrokerMonitor() *BrokerMonitor {
	return &BrokerMonitor{
		subs: make(map[chan BrokerEvent]struct{}),
		done: make(chan struct{}),
	}
}

// Close terminates all active SSE connections so http.Server.Shutdown can
// complete without timing out waiting for long-lived SSE clients to disconnect.
// Safe to call multiple times.
func (b *BrokerMonitor) Close() {
	b.once.Do(func() { close(b.done) })
}

// Push records an MQTT message and fans it out to live SSE clients.
// Never blocks: slow clients are dropped rather than back-pressuring the MQTT goroutine.
// For sparkplug messages the binary protobuf is decoded to JSON for display.
// ssfvHits is the number of metrics forwarded to the SSFV pipeline (sparkplug only).
func (b *BrokerMonitor) Push(topic, kind string, payload []byte, ssfvHits int) {
	var payloadStr string
	switch kind {
	case "json", "ssfv", "state":
		if len(payload) <= maxPayloadStore {
			payloadStr = string(payload)
		} else {
			payloadStr = string(payload[:maxPayloadStore]) + "\n…(truncado)"
		}
	case "sparkplug":
		// Decode binary protobuf to human-readable JSON for the monitor UI.
		// The original binary length is still used for PayloadSize below.
		// Guard the cost: skip oversized frames so a flood of large NBIRTH/DDATA
		// can't stall the MQTT receive goroutine (Push runs inline on it).
		switch {
		case len(payload) > maxDecodeBytes:
			payloadStr = fmt.Sprintf("…(trama de %d bytes — demasiado grande para decodificar en el monitor)", len(payload))
		default:
			if p, err := sparkplug.DecodePayload(payload); err == nil {
				if js, err := p.ToJSON(); err == nil {
					if len(js) <= maxPayloadStore {
						payloadStr = string(js)
					} else {
						payloadStr = string(js[:maxPayloadStore]) + "\n…(truncado)"
					}
				}
			}
		}
	}

	b.mu.Lock()
	b.total++
	ev := BrokerEvent{
		ID:          b.total,
		At:          time.Now(),
		Topic:       topic,
		PayloadSize: len(payload), // always the original wire size
		Kind:        kind,
		Payload:     payloadStr,
		SsfvHits:    ssfvHits,
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

// ServeSnapshot returns the most recent broker events as JSON (oldest first).
// Defaults to the full buffer; ?limit=N returns only the last N (the UI shows
// ~300, so it need not pull the whole ~MB buffer on every load/reconnect).
func (b *BrokerMonitor) ServeSnapshot(w http.ResponseWriter, r *http.Request) {
	events := b.snapshot()
	if events == nil {
		events = []BrokerEvent{}
	}
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n < len(events) {
			events = events[len(events)-n:]
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events) //nolint:errcheck
}

// ServeStream is an SSE endpoint that pushes BrokerEvents in real-time.
// It does not replay history — use ServeSnapshot for initial page load.
// This handler must be registered outside any timeout middleware.
// It returns as soon as the client disconnects OR Close() is called.
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
		case <-b.done: // server shutting down — terminate this SSE connection
			return
		case ev := <-ch:
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data) //nolint:errcheck
			flusher.Flush()
		}
	}
}
