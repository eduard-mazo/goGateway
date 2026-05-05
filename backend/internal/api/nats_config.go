package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/models"
)

type NATSConfigHandler struct {
	DB     *sqlx.DB
	Notify func()
}

func (h *NATSConfigHandler) Mount(r chi.Router) {
	r.Get("/", h.get)
	r.Put("/", h.update)
}

func (h *NATSConfigHandler) get(w http.ResponseWriter, r *http.Request) {
	var c models.NATSConfig
	if err := h.DB.Get(&c, `SELECT id,host,port,stream_name,enabled FROM nats_config WHERE id=1`); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(c)
}

func (h *NATSConfigHandler) update(w http.ResponseWriter, r *http.Request) {
	var c models.NATSConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "bad json", 400)
		return
	}

	_, err := h.DB.Exec(
		`UPDATE nats_config SET host=?, port=?, stream_name=?, enabled=? WHERE id=1`,
		c.Host, c.Port, c.StreamName, c.Enabled,
	)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if h.Notify != nil {
		h.Notify()
	}

	w.WriteHeader(204)
}
