package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/iec104"
	"goGateway/internal/mqtt"
)

type StatusHandler struct {
	DB        *sqlx.DB
	MQTT      *mqtt.Manager
	IEC104    iec104.Server
	StartedAt time.Time
}

func (h *StatusHandler) Mount(r chi.Router) {
	r.Get("/", h.get)
}

type statusResp struct {
	MQTT          mqtt.Status   `json:"mqtt"`
	IEC104        iec104.Status `json:"iec104"`
	Devices       int           `json:"devices"`
	Topics        int           `json:"topics"`
	Mappings      int           `json:"mappings"`
	HistoryCount  int64         `json:"history_count"`
	LastSampleAt  *time.Time    `json:"last_sample_at,omitempty"`
	UptimeSeconds int64         `json:"uptime_seconds"`
	StartedAt     time.Time     `json:"started_at"`
}

func (h *StatusHandler) get(w http.ResponseWriter, r *http.Request) {
	resp := statusResp{
		MQTT:          h.MQTT.Status(),
		IEC104:        h.IEC104.Status(),
		UptimeSeconds: int64(time.Since(h.StartedAt).Seconds()),
		StartedAt:     h.StartedAt,
	}
	_ = h.DB.Get(&resp.Devices, `SELECT COUNT(*) FROM devices`)
	_ = h.DB.Get(&resp.Topics, `SELECT COUNT(*) FROM topics`)
	_ = h.DB.Get(&resp.Mappings, `SELECT COUNT(*) FROM signal_mappings`)
	_ = h.DB.Get(&resp.HistoryCount, `SELECT COUNT(*) FROM history`)

	var last *time.Time
	var ts time.Time
	if err := h.DB.Get(&ts, `SELECT timestamp FROM history ORDER BY timestamp DESC LIMIT 1`); err == nil {
		last = &ts
	}
	resp.LastSampleAt = last

	writeJSON(w, 200, resp)
}
