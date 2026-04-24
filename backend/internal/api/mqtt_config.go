package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/models"
)

type MQTTConfigHandler struct {
	DB     *sqlx.DB
	Notify func()
}

func (h *MQTTConfigHandler) Mount(r chi.Router) {
	r.Get("/", h.get)
	r.Put("/", h.update)
}

func (h *MQTTConfigHandler) get(w http.ResponseWriter, r *http.Request) {
	var c models.MQTTConfig
	if err := h.DB.Get(&c, `SELECT id,host,port,username,password,client_id,use_tls FROM mqtt_config WHERE id=1`); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, c)
}

func (h *MQTTConfigHandler) update(w http.ResponseWriter, r *http.Request) {
	var c models.MQTTConfig
	if err := decode(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	_, err := h.DB.Exec(`UPDATE mqtt_config SET host=?,port=?,username=?,password=?,client_id=?,use_tls=? WHERE id=1`,
		c.Host, c.Port, c.Username, c.Password, c.ClientID, c.UseTLS)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	c.ID = 1
	if h.Notify != nil {
		h.Notify()
	}
	writeJSON(w, 200, c)
}
