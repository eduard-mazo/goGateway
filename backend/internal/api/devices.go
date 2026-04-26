// Package api exposes the gateway's REST endpoints via chi.Router.
// Each resource type has a dedicated handler struct that mounts its routes.
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

const devCols = `id,server_id,name,description,created_at`

func (h *DeviceHandler) list(w http.ResponseWriter, r *http.Request) {
	var out []models.Device
	q := `SELECT ` + devCols + ` FROM devices`
	args := []any{}
	if sid := r.URL.Query().Get("server_id"); sid != "" {
		q += ` WHERE server_id=?`
		args = append(args, sid)
	}
	q += ` ORDER BY server_id, id`
	if err := h.DB.Select(&out, q, args...); err != nil {
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
	if d.ServerID == 0 {
		writeErr(w, 400, "server_id required")
		return
	}
	res, err := h.DB.Exec(`INSERT INTO devices(server_id,name,description) VALUES(?,?,?)`, d.ServerID, d.Name, d.Description)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	d.ID, _ = res.LastInsertId()
	writeJSON(w, 201, d)
}

func (h *DeviceHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid id")
		return
	}
	var d models.Device
	if err := h.DB.Get(&d, `SELECT `+devCols+` FROM devices WHERE id=?`, id); err != nil {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, d)
}

func (h *DeviceHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid id")
		return
	}
	var d models.Device
	if err := decode(r, &d); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if d.ServerID == 0 {
		writeErr(w, 400, "server_id required")
		return
	}
	if _, err := h.DB.Exec(`UPDATE devices SET server_id=?,name=?,description=? WHERE id=?`, d.ServerID, d.Name, d.Description, id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	d.ID = id
	writeJSON(w, 200, d)
}

func (h *DeviceHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid id")
		return
	}
	if _, err := h.DB.Exec(`DELETE FROM devices WHERE id=?`, id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	w.WriteHeader(204)
}
