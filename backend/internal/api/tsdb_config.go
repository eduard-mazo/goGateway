package api

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
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
		writeErr(w, 500, "failed to load TSDB config")
		return
	}
	// Never send credentials over the wire.
	// Signal to the UI whether credentials are stored so it can show a placeholder.
	hasDSN := c.TsDSN != ""
	hasPass := c.VMPassword != ""
	c.VMPassword = ""
	c.TsDSN = ""
	writeJSON(w, 200, tsdbConfigResp{TSDBConfig: c, HasTsDSN: hasDSN, HasVMPassword: hasPass})
}

// tsdbConfigResp wraps TSDBConfig with boolean flags so the UI knows whether
// credentials are stored without receiving the actual values.
type tsdbConfigResp struct {
	models.TSDBConfig
	HasTsDSN      bool `json:"has_ts_dsn"`
	HasVMPassword bool `json:"has_vm_password"`
}

// tsdbConfigPutReq extends TSDBConfig with explicit clear flags so the UI can
// blank a credential that is masked (empty) in the form without the server
// treating the empty string as "keep stored value".
// HasTsDSN / HasVMPassword are accepted but ignored — the GET response includes
// them as read-only sentinels and the frontend may echo them back in PUT.
type tsdbConfigPutReq struct {
	models.TSDBConfig
	ClearTsDSN      bool `json:"clear_ts_dsn"`
	ClearVMPassword bool `json:"clear_vm_password"`
	HasTsDSN        bool `json:"has_ts_dsn"`
	HasVMPassword   bool `json:"has_vm_password"`
}

func (h *TSDBConfigHandler) update(w http.ResponseWriter, r *http.Request) {
	var req tsdbConfigPutReq
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	c := req.TSDBConfig
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

	// Reject path traversal attempts in WAL/DLQ paths.
	for _, p := range []string{c.WALPath, c.DLQPath} {
		clean := filepath.Clean(p)
		if strings.HasPrefix(clean, "/") || strings.Contains(clean, "..") {
			writeErr(w, 400, "wal_path and dlq_path must be relative paths with no .. components")
			return
		}
	}

	// Preserve stored credentials when the client sends empty values, unless the
	// client explicitly asked to clear them (clear_ts_dsn / clear_vm_password flags).
	needLoad := (c.VMPassword == "" && !req.ClearVMPassword) || (c.TsDSN == "" && !req.ClearTsDSN)
	if needLoad {
		var cur models.TSDBConfig
		if err := h.DB.Get(&cur, `SELECT vm_password,ts_dsn FROM tsdb_config WHERE id=1`); err != nil {
			writeErr(w, 500, "failed to load TSDB config")
			return
		}
		if c.VMPassword == "" && !req.ClearVMPassword {
			c.VMPassword = cur.VMPassword
		}
		if c.TsDSN == "" && !req.ClearTsDSN {
			c.TsDSN = cur.TsDSN
		}
	}

	_, err := h.DB.Exec(
		`UPDATE tsdb_config SET backend=?,vm_url=?,vm_username=?,vm_password=?,ts_dsn=?,ts_table=?,wal_path=?,dlq_path=?,batch_size=?,flush_ms=?,enabled=? WHERE id=1`,
		c.Backend, c.VMUrl, c.VMUsername, c.VMPassword,
		c.TsDSN, c.TsTable, c.WALPath, c.DLQPath,
		c.BatchSize, c.FlushMs, c.Enabled,
	)
	if err != nil {
		writeErr(w, 500, "failed to save TSDB config")
		return
	}
	c.ID = 1
	hasDSN := c.TsDSN != ""
	hasPass := c.VMPassword != ""
	c.VMPassword = ""
	c.TsDSN = ""
	if h.Notify != nil {
		go h.Notify()
	}
	writeJSON(w, 200, tsdbConfigResp{TSDBConfig: c, HasTsDSN: hasDSN, HasVMPassword: hasPass})
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
