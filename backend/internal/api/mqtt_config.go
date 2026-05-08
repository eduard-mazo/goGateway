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
	if err := h.DB.Get(&c, `SELECT id,host,port,username,password,client_id,use_tls,
	                               sparkplug_enabled,sp_group_id,sp_host_id
	                          FROM mqtt_config WHERE id=1`); err != nil {
		writeErr(w, 500, "failed to load MQTT config")
		return
	}
	c.Password = "" // never send the stored credential over the wire
	writeJSON(w, 200, c)
}

func (h *MQTTConfigHandler) update(w http.ResponseWriter, r *http.Request) {
	var c models.MQTTConfig
	if err := decode(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}

	// Preserve the stored password when the client sends an empty value
	// (GET returns "" so the UI form starts blank — only update if user typed a new one).
	if c.Password == "" {
		if err := h.DB.Get(&c.Password, `SELECT password FROM mqtt_config WHERE id=1`); err != nil {
			writeErr(w, 500, "failed to load MQTT config")
			return
		}
	}

	_, err := h.DB.Exec(
		`UPDATE mqtt_config
		    SET host=?,port=?,username=?,password=?,client_id=?,use_tls=?,
		        sparkplug_enabled=?,sp_group_id=?,sp_host_id=?
		  WHERE id=1`,
		c.Host, c.Port, c.Username, c.Password, c.ClientID, c.UseTLS,
		c.SparkplugEnabled, c.SpGroupID, c.SpHostID,
	)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	c.ID = 1
	c.Password = "" // don't echo the credential back
	if h.Notify != nil {
		h.Notify()
	}
	writeJSON(w, 200, c)
}
