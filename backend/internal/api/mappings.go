package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/models"
)

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

const mapCols = `id,server_id,topic_id,device_name,variable_type,characteristic,json_key,iec104_type,ioa,unit,scale,enabled`

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
	// Derive server_id from the topic's device. Devices are scoped to a
	// server, so the mapping inherits that scope automatically.
	if err := h.DB.Get(&m.ServerID, `SELECT d.server_id FROM topics t JOIN devices d ON d.id = t.device_id WHERE t.id=?`, m.TopicID); err != nil {
		writeErr(w, 400, "unknown topic")
		return
	}
	if m.Scale == 0 {
		m.Scale = 1.0
	}
	res, err := h.DB.Exec(`INSERT INTO signal_mappings(server_id,topic_id,device_name,variable_type,characteristic,json_key,iec104_type,ioa,unit,scale,enabled) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		m.ServerID, m.TopicID, m.DeviceName, m.VariableType, m.Characteristic, m.JSONKey, m.IEC104Type, m.IOA, m.Unit, m.Scale, m.Enabled)
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
	if _, err := h.DB.Exec(`UPDATE signal_mappings SET server_id=?,topic_id=?,device_name=?,variable_type=?,characteristic=?,json_key=?,iec104_type=?,ioa=?,unit=?,scale=?,enabled=? WHERE id=?`,
		m.ServerID, m.TopicID, m.DeviceName, m.VariableType, m.Characteristic, m.JSONKey, m.IEC104Type, m.IOA, m.Unit, m.Scale, m.Enabled, id); err != nil {
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
