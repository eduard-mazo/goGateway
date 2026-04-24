package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/models"
)

// IEC104ServersHandler exposes CRUD over the iec104_servers table — one row
// per passive slave endpoint. Each row carries its own Common ASDU Address so
// several SCADA masters can read the gateway under different ASDUs.
type IEC104ServersHandler struct {
	DB     *sqlx.DB
	Notify func()
}

const iec104Cols = `id,name,listen_addr,port,asdu_addr,k,w,t0,t1,t2,t3,enabled`

func (h *IEC104ServersHandler) Mount(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{id}", h.get)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
}

func (h *IEC104ServersHandler) notify() {
	if h.Notify != nil {
		h.Notify()
	}
}

func (h *IEC104ServersHandler) list(w http.ResponseWriter, r *http.Request) {
	var out []models.IEC104Server
	if err := h.DB.Select(&out, `SELECT `+iec104Cols+` FROM iec104_servers ORDER BY id`); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

func (h *IEC104ServersHandler) get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var s models.IEC104Server
	if err := h.DB.Get(&s, `SELECT `+iec104Cols+` FROM iec104_servers WHERE id=?`, id); err != nil {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, s)
}

func (h *IEC104ServersHandler) create(w http.ResponseWriter, r *http.Request) {
	var s models.IEC104Server
	if err := decode(r, &s); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	applyServerDefaults(&s)
	res, err := h.DB.Exec(
		`INSERT INTO iec104_servers(name,listen_addr,port,asdu_addr,k,w,t0,t1,t2,t3,enabled) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		s.Name, s.ListenAddr, s.Port, s.ASDUAddr, s.K, s.W, s.T0, s.T1, s.T2, s.T3, s.Enabled)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.ID, _ = res.LastInsertId()
	h.notify()
	writeJSON(w, 201, s)
}

func (h *IEC104ServersHandler) update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var s models.IEC104Server
	if err := decode(r, &s); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	applyServerDefaults(&s)
	if _, err := h.DB.Exec(
		`UPDATE iec104_servers SET name=?,listen_addr=?,port=?,asdu_addr=?,k=?,w=?,t0=?,t1=?,t2=?,t3=?,enabled=? WHERE id=?`,
		s.Name, s.ListenAddr, s.Port, s.ASDUAddr, s.K, s.W, s.T0, s.T1, s.T2, s.T3, s.Enabled, id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.ID = id
	h.notify()
	writeJSON(w, 200, s)
}

func (h *IEC104ServersHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := h.DB.Exec(`DELETE FROM iec104_servers WHERE id=?`, id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h.notify()
	w.WriteHeader(204)
}

// applyServerDefaults backfills IEC-104 §5 defaults for any zero-valued knob.
func applyServerDefaults(s *models.IEC104Server) {
	if s.ListenAddr == "" {
		s.ListenAddr = "0.0.0.0"
	}
	if s.Port == 0 {
		s.Port = 2404
	}
	if s.ASDUAddr == 0 {
		s.ASDUAddr = 1
	}
	if s.K == 0 {
		s.K = 12
	}
	if s.W == 0 {
		s.W = 8
	}
	if s.T0 == 0 {
		s.T0 = 30
	}
	if s.T1 == 0 {
		s.T1 = 15
	}
	if s.T2 == 0 {
		s.T2 = 10
	}
	if s.T3 == 0 {
		s.T3 = 20
	}
}
