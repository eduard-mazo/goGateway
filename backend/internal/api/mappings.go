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

const mapCols = `id,topic_id,device_name,variable_type,characteristic,json_key,iec104_type,ioa,unit,scale,enabled`

func (h *MappingHandler) list(w http.ResponseWriter, r *http.Request) {
	var out []models.SignalMapping
	q := `SELECT ` + mapCols + ` FROM signal_mappings`
	args := []any{}
	if tid := r.URL.Query().Get("topic_id"); tid != "" {
		q += ` WHERE topic_id=?`
		args = append(args, tid)
	}
	q += ` ORDER BY ioa`
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
	if m.Scale == 0 {
		m.Scale = 1.0
	}
	res, err := h.DB.Exec(`INSERT INTO signal_mappings(topic_id,device_name,variable_type,characteristic,json_key,iec104_type,ioa,unit,scale,enabled) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		m.TopicID, m.DeviceName, m.VariableType, m.Characteristic, m.JSONKey, m.IEC104Type, m.IOA, m.Unit, m.Scale, m.Enabled)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	m.ID, _ = res.LastInsertId()
	h.notify()
	writeJSON(w, 201, m)
}

func (h *MappingHandler) update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var m models.SignalMapping
	if err := decode(r, &m); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if m.Scale == 0 {
		m.Scale = 1.0
	}
	if _, err := h.DB.Exec(`UPDATE signal_mappings SET topic_id=?,device_name=?,variable_type=?,characteristic=?,json_key=?,iec104_type=?,ioa=?,unit=?,scale=?,enabled=? WHERE id=?`,
		m.TopicID, m.DeviceName, m.VariableType, m.Characteristic, m.JSONKey, m.IEC104Type, m.IOA, m.Unit, m.Scale, m.Enabled, id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	m.ID = id
	h.notify()
	writeJSON(w, 200, m)
}

func (h *MappingHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := h.DB.Exec(`DELETE FROM signal_mappings WHERE id=?`, id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.notify()
	w.WriteHeader(204)
}
