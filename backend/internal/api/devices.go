package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/models"
)

type DeviceHandler struct{ DB *sqlx.DB }

func (h *DeviceHandler) Mount(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{id}", h.get)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
}

func (h *DeviceHandler) list(w http.ResponseWriter, r *http.Request) {
	var out []models.Device
	if err := h.DB.Select(&out, `SELECT id,name,description,created_at FROM devices ORDER BY id`); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (h *DeviceHandler) create(w http.ResponseWriter, r *http.Request) {
	var d models.Device
	if err := decode(r, &d); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	res, err := h.DB.Exec(`INSERT INTO devices(name,description) VALUES(?,?)`, d.Name, d.Description)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	d.ID, _ = res.LastInsertId()
	writeJSON(w, 201, d)
}

func (h *DeviceHandler) get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var d models.Device
	if err := h.DB.Get(&d, `SELECT id,name,description,created_at FROM devices WHERE id=?`, id); err != nil {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, d)
}

func (h *DeviceHandler) update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var d models.Device
	if err := decode(r, &d); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if _, err := h.DB.Exec(`UPDATE devices SET name=?,description=? WHERE id=?`, d.Name, d.Description, id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	d.ID = id
	writeJSON(w, 200, d)
}

func (h *DeviceHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := h.DB.Exec(`DELETE FROM devices WHERE id=?`, id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	w.WriteHeader(204)
}
