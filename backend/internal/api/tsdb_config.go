package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/models"
	"goGateway/internal/tsdb"
)

// TSDBConfigHandler serves GET/PUT for tsdb_config and POST /test.
type TSDBConfigHandler struct {
	DB     *sqlx.DB
	Notify func() // triggers manager.Reload on the new config
}

func (h *TSDBConfigHandler) Mount(r chi.Router) {
	r.Get("/", h.get)
	r.Put("/", h.update)
	r.Post("/test", h.test)
}

func (h *TSDBConfigHandler) get(w http.ResponseWriter, r *http.Request) {
	var c models.TSDBConfig
	if err := h.DB.Get(&c, `SELECT id,backend,vm_url,vm_username,vm_password,ts_dsn,ts_table,wal_path,dlq_path,batch_size,flush_ms,enabled FROM tsdb_config WHERE id=1`); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, c)
}

func (h *TSDBConfigHandler) update(w http.ResponseWriter, r *http.Request) {
	var c models.TSDBConfig
	if err := decode(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if c.TsTable == "" {
		c.TsTable = "signals"
	}
	if c.WALPath == "" {
		c.WALPath = "data/wal.bolt"
	}
	if c.DLQPath == "" {
		c.DLQPath = "data/dlq.bolt"
	}
	if c.BatchSize == 0 {
		c.BatchSize = 2000
	}
	if c.FlushMs == 0 {
		c.FlushMs = 100
	}

	_, err := h.DB.Exec(
		`UPDATE tsdb_config SET backend=?,vm_url=?,vm_username=?,vm_password=?,ts_dsn=?,ts_table=?,wal_path=?,dlq_path=?,batch_size=?,flush_ms=?,enabled=? WHERE id=1`,
		c.Backend, c.VMUrl, c.VMUsername, c.VMPassword,
		c.TsDSN, c.TsTable, c.WALPath, c.DLQPath,
		c.BatchSize, c.FlushMs, c.Enabled,
	)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	c.ID = 1
	if h.Notify != nil {
		go h.Notify()
	}
	writeJSON(w, 200, c)
}

func (h *TSDBConfigHandler) test(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Backend    string `json:"backend"`
		VMUrl      string `json:"vm_url"`
		VMUsername string `json:"vm_username"`
		VMPassword string `json:"vm_password"`
		TsDSN      string `json:"ts_dsn"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var result tsdb.TestResult
	switch req.Backend {
	case "victoriametrics":
		result = tsdb.TestVMConnection(ctx, tsdb.VMConfig{
			URL:      req.VMUrl,
			Username: req.VMUsername,
			Password: req.VMPassword,
		})
	case "timescaledb":
		result = tsdb.TestTimescaleConnection(ctx, req.TsDSN)
	default:
		writeErr(w, 400, "unknown backend: "+req.Backend)
		return
	}
	writeJSON(w, 200, result)
}
