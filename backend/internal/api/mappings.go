package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/models"
)

// ─── Signal Mappings ──────────────────────────────────────────────────────────

type MappingHandler struct {
	DB     *sqlx.DB
	Notify func()
}

func (h *MappingHandler) Mount(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
}

func (h *MappingHandler) notify() {
	if h.Notify != nil {
		h.Notify()
	}
}

const mapCols = `id,server_id,topic_id,signal_id,device_name,variable_type,characteristic,json_key,quality_key,metric_name,iec104_type,ioa,unit,scale,enabled,business,company,deadband_abs,deadband_pct`

func (h *MappingHandler) list(w http.ResponseWriter, r *http.Request) {
	var out []models.SignalMapping
	q := `SELECT ` + mapCols + ` FROM signal_mappings`
	args := []any{}
	conds := []string{}
	if tid := r.URL.Query().Get("topic_id"); tid != "" {
		conds = append(conds, `topic_id=?`)
		args = append(args, tid)
	}
	if sid := r.URL.Query().Get("server_id"); sid != "" {
		conds = append(conds, `server_id=?`)
		args = append(args, sid)
	}
	for i, c := range conds {
		if i == 0 {
			q += ` WHERE ` + c
		} else {
			q += ` AND ` + c
		}
	}
	q += ` ORDER BY server_id, ioa`
	if err := h.DB.Select(&out, q, args...); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (h *MappingHandler) create(w http.ResponseWriter, r *http.Request) {
	var m models.SignalMapping
	if err := decode(r, &m); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if m.TopicID == 0 {
		writeErr(w, 400, "topic_id required")
		return
	}
	// When signal_id is provided, inherit signal definition fields from gateway_signals.
	if m.SignalID != nil && *m.SignalID > 0 {
		var gs models.GatewaySignal
		if err := h.DB.Get(&gs, `SELECT id,topic_id,name,json_key,metric_name,quality_key,variable_type,characteristic,unit,scale,persist_to_db,enabled,created_at FROM gateway_signals WHERE id=?`, *m.SignalID); err == nil {
			m.TopicID = gs.TopicID
			if m.JSONKey == "" {
				m.JSONKey = gs.JSONKey
			}
			if m.MetricName == "" {
				m.MetricName = gs.MetricName
			}
			if m.QualityKey == "" {
				m.QualityKey = gs.QualityKey
			}
			if m.VariableType == "" {
				m.VariableType = gs.VariableType
			}
			if m.Characteristic == "" {
				m.Characteristic = gs.Characteristic
			}
			if m.Unit == "" {
				m.Unit = gs.Unit
			}
			if m.Scale == 0 || m.Scale == 1 {
				m.Scale = gs.Scale
			}
		}
	}
	// Derive server_id from the topic's device. Devices are scoped to a
	// server, so the mapping inherits that scope automatically.
	if err := h.DB.Get(&m.ServerID, `SELECT d.server_id FROM topics t JOIN devices d ON d.id = t.device_id WHERE t.id=?`, m.TopicID); err != nil {
		writeErr(w, 400, "unknown topic")
		return
	}
	if m.Scale == 0 {
		m.Scale = 1.0
	}
	res, err := h.DB.Exec(`INSERT INTO signal_mappings(server_id,topic_id,signal_id,device_name,variable_type,characteristic,json_key,quality_key,metric_name,iec104_type,ioa,unit,scale,enabled,business,company,deadband_abs,deadband_pct) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ServerID, m.TopicID, m.SignalID, m.DeviceName, m.VariableType, m.Characteristic, m.JSONKey, m.QualityKey, m.MetricName, m.IEC104Type, m.IOA, m.Unit, m.Scale, m.Enabled, m.Business, m.Company, m.DeadbandAbs, m.DeadbandPct)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	m.ID, _ = res.LastInsertId()
	h.notify()
	writeJSON(w, 201, m)
}

func (h *MappingHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid id")
		return
	}
	var m models.SignalMapping
	if err := decode(r, &m); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if m.TopicID == 0 {
		writeErr(w, 400, "topic_id required")
		return
	}
	if err := h.DB.Get(&m.ServerID, `SELECT d.server_id FROM topics t JOIN devices d ON d.id = t.device_id WHERE t.id=?`, m.TopicID); err != nil {
		writeErr(w, 400, "unknown topic")
		return
	}
	if m.Scale == 0 {
		m.Scale = 1.0
	}
	if _, err := h.DB.Exec(`UPDATE signal_mappings SET server_id=?,topic_id=?,signal_id=?,device_name=?,variable_type=?,characteristic=?,json_key=?,quality_key=?,metric_name=?,iec104_type=?,ioa=?,unit=?,scale=?,enabled=?,business=?,company=?,deadband_abs=?,deadband_pct=? WHERE id=?`,
		m.ServerID, m.TopicID, m.SignalID, m.DeviceName, m.VariableType, m.Characteristic, m.JSONKey, m.QualityKey, m.MetricName, m.IEC104Type, m.IOA, m.Unit, m.Scale, m.Enabled, m.Business, m.Company, m.DeadbandAbs, m.DeadbandPct, id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	m.ID = id
	h.notify()
	writeJSON(w, 200, m)
}

func (h *MappingHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid id")
		return
	}
	if _, err := h.DB.Exec(`DELETE FROM signal_mappings WHERE id=?`, id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.notify()
	w.WriteHeader(204)
}

// ─── Gateway Signals ──────────────────────────────────────────────────────────

// GatewaySignalHandler provides CRUD for the gateway_signals table.
// Gateway signals are the normalization layer (Menu 2) between inbound MQTT
// topic streams and outbound IEC-104 points.  PersistToDB controls whether
// values are recorded to the local SQLite history store.
type GatewaySignalHandler struct {
	DB     *sqlx.DB
	Notify func() // reuse NotifyMappings to invalidate dispatch cache
}

func (h *GatewaySignalHandler) Mount(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
}

func (h *GatewaySignalHandler) notify() {
	if h.Notify != nil {
		h.Notify()
	}
}

const sigCols = `id,topic_id,name,json_key,metric_name,quality_key,variable_type,characteristic,unit,scale,persist_to_db,enabled,created_at`

func (h *GatewaySignalHandler) list(w http.ResponseWriter, r *http.Request) {
	var out []models.GatewaySignal
	q := `SELECT ` + sigCols + ` FROM gateway_signals`
	args := []any{}
	if tid := r.URL.Query().Get("topic_id"); tid != "" {
		q += ` WHERE topic_id=?`
		args = append(args, tid)
	}
	q += ` ORDER BY topic_id, name`
	if err := h.DB.Select(&out, q, args...); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (h *GatewaySignalHandler) create(w http.ResponseWriter, r *http.Request) {
	var gs models.GatewaySignal
	if err := decode(r, &gs); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if gs.TopicID == 0 {
		writeErr(w, 400, "topic_id required")
		return
	}
	if gs.Name == "" {
		writeErr(w, 400, "name required")
		return
	}
	if gs.JSONKey == "" && gs.MetricName == "" {
		writeErr(w, 400, "json_key or metric_name required")
		return
	}
	if gs.Scale == 0 {
		gs.Scale = 1.0
	}
	res, err := h.DB.Exec(
		`INSERT INTO gateway_signals(topic_id,name,json_key,metric_name,quality_key,variable_type,characteristic,unit,scale,persist_to_db,enabled) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		gs.TopicID, gs.Name, gs.JSONKey, gs.MetricName, gs.QualityKey,
		gs.VariableType, gs.Characteristic, gs.Unit, gs.Scale, gs.PersistToDB, gs.Enabled,
	)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	gs.ID, _ = res.LastInsertId()
	h.notify()
	writeJSON(w, 201, gs)
}

func (h *GatewaySignalHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid id")
		return
	}
	var gs models.GatewaySignal
	if err := decode(r, &gs); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if gs.Scale == 0 {
		gs.Scale = 1.0
	}
	if _, err := h.DB.Exec(
		`UPDATE gateway_signals SET topic_id=?,name=?,json_key=?,metric_name=?,quality_key=?,variable_type=?,characteristic=?,unit=?,scale=?,persist_to_db=?,enabled=? WHERE id=?`,
		gs.TopicID, gs.Name, gs.JSONKey, gs.MetricName, gs.QualityKey,
		gs.VariableType, gs.Characteristic, gs.Unit, gs.Scale, gs.PersistToDB, gs.Enabled, id,
	); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	gs.ID = id
	h.notify()
	writeJSON(w, 200, gs)
}

func (h *GatewaySignalHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid id")
		return
	}
	if _, err := h.DB.Exec(`DELETE FROM gateway_signals WHERE id=?`, id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.notify()
	w.WriteHeader(204)
}
