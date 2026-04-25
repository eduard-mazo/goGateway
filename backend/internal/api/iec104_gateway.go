package api

import (
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/models"
)

// IEC104GatewayHandler exposes the singleton iec104_gateway row — the bind IP
// shared by every slave endpoint in the fleet. Operators set this once per
// host (or leave it on 0.0.0.0).
type IEC104GatewayHandler struct {
	DB     *sqlx.DB
	Notify func()
}

func (h *IEC104GatewayHandler) Mount(r chi.Router) {
	r.Get("/", h.get)
	r.Put("/", h.put)
}

func (h *IEC104GatewayHandler) get(w http.ResponseWriter, r *http.Request) {
	var g models.IEC104Gateway
	if err := h.DB.Get(&g, `SELECT id, listen_ip FROM iec104_gateway WHERE id=1`); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, g)
}

func (h *IEC104GatewayHandler) put(w http.ResponseWriter, r *http.Request) {
	var g models.IEC104Gateway
	if err := decode(r, &g); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if g.ListenIP == "" {
		g.ListenIP = "0.0.0.0"
	}
	// Validate: must be a parseable IP literal. Hostnames are rejected — the
	// kernel needs a concrete bind target and async resolution masks errors.
	if net.ParseIP(g.ListenIP) == nil {
		writeErr(w, 400, "listen_ip must be a numeric IPv4/IPv6 address (e.g. 0.0.0.0 or 10.114.199.57)")
		return
	}
	if _, err := h.DB.Exec(`UPDATE iec104_gateway SET listen_ip=? WHERE id=1`, g.ListenIP); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	g.ID = 1
	if h.Notify != nil {
		h.Notify()
	}
	writeJSON(w, 200, g)
}
