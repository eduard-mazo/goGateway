package tsdb

import (
	"net/http"
	"time"
)

// Handler exposes pipeline metrics and DLQ management over HTTP.
type Handler struct {
	mgr *Manager
}

func NewHandler(mgr *Manager) *Handler { return &Handler{mgr: mgr} }

func (h *Handler) ServeStatus(w http.ResponseWriter, r *http.Request) {
	p := h.mgr.Pipeline()
	if p == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"running":     false,
			"backends":    []any{},
			"walPending":  0,
			"dlqDepth":    0,
			"inputRate":   0,
			"inputQueue":  0,
			"retryQueues": map[string]int{},
			"timestamp":   time.Now(),
		})
		return
	}

	type retryDepths map[string]int
	type resp struct {
		Running     bool            `json:"running"`
		Backends    []BackendStatus `json:"backends"`
		WALPending  int             `json:"walPending"`
		DLQDepth    int             `json:"dlqDepth"`
		InputRate   float64         `json:"inputRate"`
		InputQueue  int             `json:"inputQueue"`
		RetryQueues retryDepths     `json:"retryQueues"`
		Timestamp   time.Time       `json:"timestamp"`
	}

	statuses := make([]BackendStatus, 0, len(p.backends))
	for _, b := range p.backends {
		statuses = append(statuses, b.Status())
	}
	retryD := make(retryDepths)
	for _, b := range p.backends {
		retryD[b.Name()] = p.RetryQueueDepth(b.Name())
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp{ //nolint:errcheck
		Running:     true,
		Backends:    statuses,
		WALPending:  p.WAL().Pending(),
		DLQDepth:    p.DLQ().Len(),
		InputRate:   p.InputRate(),
		InputQueue:  p.InputQueueDepth(),
		RetryQueues: retryD,
		Timestamp:   time.Now(),
	})
}

func (h *Handler) ServeDLQ(w http.ResponseWriter, r *http.Request) {
	p := h.mgr.Pipeline()
	if p == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"count": 0, "entries": []any{}}) //nolint:errcheck
		return
	}
	var entries []DLQEntry
	p.DLQ().Replay(func(e DLQEntry) bool { //nolint:errcheck
		entries = append(entries, e)
		return false // read-only
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"count":   len(entries),
		"entries": entries,
	})
}

func (h *Handler) ServeDLQReplay(w http.ResponseWriter, r *http.Request) {
	p := h.mgr.Pipeline()
	if p == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"replayed": 0}) //nolint:errcheck
		return
	}
	replayed := 0
	p.DLQ().Replay(func(e DLQEntry) bool { //nolint:errcheck
		for _, pt := range e.Batch {
			if p.Push(pt) != nil {
				return false // pipeline full; keep entry
			}
		}
		replayed++
		return true
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"replayed": replayed}) //nolint:errcheck
}

func (h *Handler) ServePoints(w http.ResponseWriter, r *http.Request) {
	p := h.mgr.Pipeline()
	if p == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{}) //nolint:errcheck
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p.Store().Snapshot()) //nolint:errcheck
}
