package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/models"
)

type TopicHandler struct {
	DB     *sqlx.DB
	Notify func() // fired on change to rebuild MQTT subs.
}

func (h *TopicHandler) Mount(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
}

func (h *TopicHandler) notify() {
	if h.Notify != nil {
		h.Notify()
	}
}

func (h *TopicHandler) list(w http.ResponseWriter, r *http.Request) {
	var out []models.Topic
	if err := h.DB.Select(&out, `SELECT id,device_id,topic,qos,enabled,payload_format FROM topics ORDER BY id`); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (h *TopicHandler) create(w http.ResponseWriter, r *http.Request) {
	var t models.Topic
	if err := decode(r, &t); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if t.PayloadFormat == "" {
		t.PayloadFormat = "json"
	}
	res, err := h.DB.Exec(`INSERT INTO topics(device_id,topic,qos,enabled,payload_format) VALUES(?,?,?,?,?)`,
		t.DeviceID, t.Topic, t.QoS, t.Enabled, t.PayloadFormat)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	t.ID, _ = res.LastInsertId()
	h.notify()
	writeJSON(w, 201, t)
}

func (h *TopicHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid id")
		return
	}
	var t models.Topic
	if err := decode(r, &t); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if t.PayloadFormat == "" {
		t.PayloadFormat = "json"
	}
	if _, err := h.DB.Exec(`UPDATE topics SET device_id=?,topic=?,qos=?,enabled=?,payload_format=? WHERE id=?`,
		t.DeviceID, t.Topic, t.QoS, t.Enabled, t.PayloadFormat, id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	t.ID = id
	h.notify()
	writeJSON(w, 200, t)
}

func (h *TopicHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid id")
		return
	}
	if _, err := h.DB.Exec(`DELETE FROM topics WHERE id=?`, id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.notify()
	w.WriteHeader(204)
}
