package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/models"
)

type HistoryHandler struct{ DB *sqlx.DB }

func (h *HistoryHandler) Mount(r chi.Router) {
	r.Get("/", h.query)
}

// GET /history?mapping_id=1&limit=500
func (h *HistoryHandler) query(w http.ResponseWriter, r *http.Request) {
	q := `SELECT id,mapping_id,signal_path,value,quality,timestamp FROM history`
	args := []any{}
	if mid := r.URL.Query().Get("mapping_id"); mid != "" {
		q += ` WHERE mapping_id=?`
		args = append(args, mid)
	}
	q += ` ORDER BY timestamp DESC LIMIT ?`
	limit := 500
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 10000 {
			limit = n
		}
	}
	args = append(args, limit)

	var out []models.History
	if err := h.DB.Select(&out, q, args...); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}
