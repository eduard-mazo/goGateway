package tsdb

import (
	"net/http"
	"time"
)

// Handler exposes pipeline metrics and DLQ management over HTTP.
// Routes are registered on any mux using RegisterRoutes.
type Handler struct {
	p *WritePipeline
}

func NewHandler(p *WritePipeline) *Handler { return &Handler{p: p} }

// ServeStatus, ServeDLQ, ServeDLQReplay, ServePoints are http.HandlerFunc
// compatible so they can be registered on any mux (chi, stdlib, etc.).

func (h *Handler) ServeStatus(w http.ResponseWriter, r *http.Request) {
	h.status(w, r)
}
func (h *Handler) ServeDLQ(w http.ResponseWriter, r *http.Request) { h.dlqList(w, r) }
func (h *Handler) ServeDLQReplay(w http.ResponseWriter, r *http.Request) {
	h.dlqReplay(w, r)
}
func (h *Handler) ServePoints(w http.ResponseWriter, r *http.Request) { h.points(w, r) }

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	type retryDepths map[string]int
	type resp struct {
		Backends    []BackendStatus `json:"backends"`
		WALPending  int             `json:"walPending"`
		DLQDepth    int             `json:"dlqDepth"`
		InputRate   float64         `json:"inputRate"`
		InputQueue  int             `json:"inputQueue"`
		RetryQueues retryDepths     `json:"retryQueues"`
		Timestamp   time.Time       `json:"timestamp"`
	}

	statuses := make([]BackendStatus, 0, len(h.p.backends))
	for _, b := range h.p.backends {
		statuses = append(statuses, b.Status())
	}
	retryD := make(retryDepths)
	for _, b := range h.p.backends {
		retryD[b.Name()] = h.p.RetryQueueDepth(b.Name())
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp{ //nolint:errcheck
		Backends:    statuses,
		WALPending:  h.p.WAL().Pending(),
		DLQDepth:    h.p.DLQ().Len(),
		InputRate:   h.p.InputRate(),
		InputQueue:  h.p.InputQueueDepth(),
		RetryQueues: retryD,
		Timestamp:   time.Now(),
	})
}

func (h *Handler) dlqList(w http.ResponseWriter, r *http.Request) {
	var entries []DLQEntry
	h.p.DLQ().Replay(func(e DLQEntry) bool { //nolint:errcheck
		entries = append(entries, e)
		return false // read-only
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"count":   len(entries),
		"entries": entries,
	})
}

func (h *Handler) dlqReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	replayed := 0
	h.p.DLQ().Replay(func(e DLQEntry) bool { //nolint:errcheck
		for _, pt := range e.Batch {
			if h.p.Push(pt) != nil {
				return false // pipeline full; keep entry
			}
		}
		replayed++
		return true
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"replayed": replayed}) //nolint:errcheck
}

func (h *Handler) points(w http.ResponseWriter, r *http.Request) {
	snap := h.p.Store().Snapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snap) //nolint:errcheck
}
