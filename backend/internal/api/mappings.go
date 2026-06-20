package api

import (
	"net/http"
	"strconv"
	"strings"

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
	r.Post("/assign", h.assign)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
}

// deriveIEC104Type maps a signal's variable type (as carried in the SSFV
// catalog / gateway_signals) to the default IEC-104 monitored type for that
// class of point. Returns "" when the type is unknown so the caller can require
// an explicit iec104_type instead of guessing wrong.
func deriveIEC104Type(variableType string) string {
	switch strings.ToLower(strings.TrimSpace(variableType)) {
	case "analog", "analogica", "analógica", "ai", "measured", "instantaneo", "instantáneo", "acumulado":
		return "M_ME_NC_1" // measured value, short float, no time
	case "digital", "binary", "di", "bool", "boolean", "single", "single-point":
		return "M_SP_NA_1" // single point, no time
	default:
		return ""
	}
}

// nextFreeIOA returns the next unused IOA on a server (max+1, starting at 1).
// IOA 0 is reserved in IEC-104, so the first assigned address is 1.
func nextFreeIOA(db *sqlx.DB, serverID int64) (int, error) {
	var maxIOA int
	if err := db.Get(&maxIOA, `SELECT COALESCE(MAX(ioa),0) FROM signal_mappings WHERE server_id=?`, serverID); err != nil {
		return 0, err
	}
	return maxIOA + 1, nil
}

// mappingSpec is the input to assignMapping: only the wire-relevant fields plus
// the optional SSFV linkage. Everything else keeps its DB default.
type mappingSpec struct {
	ServerID, TopicID            int64
	MetricName, IEC104Type, Unit string
	IOA                          int     // <=0 auto-assigns the next free IOA on the server
	Scale                        float64 // <=0 defaults to 1.0
	// SSFV linkage (0 = standalone mapping); set by the SSFV mirror path so the
	// mapping is cascade-deleted when the SSFV source goes away.
	SSFVPlantaID, SSFVEquipoID, SSFVEquisenalID int64
}

// assignMapping writes a minimal IEC-104 mapping (only the wire-relevant columns
// + SSFV linkage; everything else keeps its DB default) and returns the stored
// row. Shared by the HTTP assign endpoint and the SSFV→IEC-104 mirror.
func assignMapping(db *sqlx.DB, s mappingSpec) (models.SignalMapping, error) {
	ioa := s.IOA
	if ioa <= 0 {
		n, err := nextFreeIOA(db, s.ServerID)
		if err != nil {
			return models.SignalMapping{}, err
		}
		ioa = n
	}
	scale := s.Scale
	if scale == 0 {
		scale = 1.0
	}
	res, err := db.Exec(
		`INSERT INTO signal_mappings(server_id,topic_id,metric_name,iec104_type,ioa,scale,unit,enabled,ssfv_planta_id,ssfv_equipo_id,ssfv_equisenal_id) VALUES(?,?,?,?,?,?,?,1,?,?,?)`,
		s.ServerID, s.TopicID, s.MetricName, s.IEC104Type, ioa, scale, s.Unit,
		s.SSFVPlantaID, s.SSFVEquipoID, s.SSFVEquisenalID)
	if err != nil {
		return models.SignalMapping{}, err
	}
	id, _ := res.LastInsertId()
	var m models.SignalMapping
	if err := db.Get(&m, `SELECT `+mapCols+` FROM signal_mappings WHERE id=?`, id); err != nil {
		return models.SignalMapping{}, err
	}
	return m, nil
}

// assignReq is the minimal input to expose a Sparkplug signal on IEC-104.
// Only the fields that actually reach the wire are accepted; everything else in
// signal_mappings keeps its DB default. A producer of these values (e.g. an
// SSFV signal) supplies topic_id + metric_name + variable_type and lets the
// gateway derive the type and assign the address.
type assignReq struct {
	TopicID      int64   `json:"topic_id"`      // required: the spBv1.0 node/device topic row
	MetricName   string  `json:"metric_name"`   // required: Sparkplug metric (instancia/codigo)
	VariableType string  `json:"variable_type"` // analog|digital → derives iec104_type
	IEC104Type   string  `json:"iec104_type"`   // optional: explicit override of the derived type
	IOA          int     `json:"ioa"`           // optional: <=0 auto-assigns the next free IOA
	Scale        float64 `json:"scale"`         // optional: default 1.0
	Unit         string  `json:"unit"`          // optional metadata
}

// assign creates a minimal IEC-104 mapping: it derives server_id from the
// topic, derives the wire type from variable_type when not given, auto-assigns
// the next free IOA when omitted, and enables the point. All non-wire columns
// (business/company/json_key/quality_key/device_name/…) are left at their DB
// defaults — only the necessary fields are written.
func (h *MappingHandler) assign(w http.ResponseWriter, r *http.Request) {
	var req assignReq
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if req.TopicID == 0 {
		writeErr(w, 400, "topic_id required")
		return
	}
	if req.MetricName == "" {
		writeErr(w, 400, "metric_name required")
		return
	}
	iecType := req.IEC104Type
	if iecType == "" {
		iecType = deriveIEC104Type(req.VariableType)
	}
	if iecType == "" {
		writeErr(w, 400, "iec104_type required (or set variable_type to analog|digital)")
		return
	}
	var serverID int64
	if err := h.DB.Get(&serverID, `SELECT d.server_id FROM topics t JOIN devices d ON d.id = t.device_id WHERE t.id=?`, req.TopicID); err != nil {
		writeErr(w, 400, "unknown topic")
		return
	}
	m, err := assignMapping(h.DB, mappingSpec{
		ServerID: serverID, TopicID: req.TopicID, MetricName: req.MetricName,
		IEC104Type: iecType, IOA: req.IOA, Scale: req.Scale, Unit: req.Unit,
	})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.notify()
	writeJSON(w, 201, m)
}

func (h *MappingHandler) notify() {
	if h.Notify != nil {
		h.Notify()
	}
}

const mapCols = `id,server_id,topic_id,signal_id,device_name,variable_type,characteristic,json_key,quality_key,metric_name,iec104_type,ioa,unit,scale,enabled,business,company,deadband_abs,deadband_pct,ssfv_planta_id,ssfv_equipo_id,ssfv_equisenal_id`

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
