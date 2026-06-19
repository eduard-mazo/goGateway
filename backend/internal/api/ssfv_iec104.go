package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/models"
)

// SSFV → IEC-104 bridge.
//
// An SSFV signal already carries everything an IEC-104 point needs except the
// address: the equipo's nombre_topic (entity), the señal's codigo_senal, the
// asignación's nombre_instancia, the variable type and the unit. This bridge
// reads those from the SSFV catalog (TimescaleDB) and creates the minimal
// signal_mappings rows (SQLite) via assignMapping — deriving the IEC-104 type
// and auto-assigning the IOA. One signal (by asignación) or a whole equipo
// (bulk) at a time.

// SetMappingNotifier wires the callback that reloads the worker IEC-104
// MappingCache after the bridge creates mappings (same notifier the /mappings
// handler uses). Set by the router.
func (h *SSFVHandler) SetMappingNotifier(fn func()) { h.mappingReload = fn }

// derivedSignal is one SSFV assignment reduced to the fields an IEC-104 point
// needs. NodeBase is the canonical Sparkplug base (spBv1.0/group/node[/device])
// the worker keys mappings by; MetricName is "<nombre_instancia>/<codigo_senal>".
type derivedSignal struct {
	NodeBase   string
	MetricName string
	IEC104Type string
	Unit       string
}

// exposeResult reports what the bridge did, so a partial run (some already
// mapped, some unmappable) is transparent rather than silently dropping rows.
type exposeResult struct {
	ServerID int64                  `json:"server_id"`
	Created  []models.SignalMapping `json:"created"`
	Skipped  []string               `json:"skipped"`
}

// bridgeIEC104Type picks the wire type for an SSFV signal: an explicit
// analog/digital variable or value type wins; otherwise alarm/event signals are
// single-point (digital) and everything else is a measured short float.
func bridgeIEC104Type(tipoVariable, tipoValor string, esAlarma bool) string {
	if t := deriveIEC104Type(tipoVariable); t != "" {
		return t
	}
	if t := deriveIEC104Type(tipoValor); t != "" {
		return t
	}
	if esAlarma {
		return "M_SP_NA_1"
	}
	return "M_ME_NC_1"
}

// ensureIEC104Topic finds (or creates) the topic row for a Sparkplug node base
// on the given server, so a mapping can reference it. Bridge-created devices are
// named "iec104:<nodeBase>" and tagged so they're recognisable. Idempotent.
func ensureIEC104Topic(db *sqlx.DB, serverID int64, nodeBase string) (int64, error) {
	var topicID int64
	err := db.Get(&topicID, `
		SELECT t.id FROM topics t JOIN devices d ON d.id = t.device_id
		WHERE d.server_id=? AND t.topic=? LIMIT 1`, serverID, nodeBase)
	if err == nil {
		return topicID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	devName := "iec104:" + nodeBase
	var devID int64
	err = db.Get(&devID, `SELECT id FROM devices WHERE server_id=? AND name=? LIMIT 1`, serverID, devName)
	if errors.Is(err, sql.ErrNoRows) {
		res, e := db.Exec(`INSERT INTO devices(server_id,name,description) VALUES(?,?,?)`,
			serverID, devName, "auto: SSFV→IEC-104 bridge")
		if e != nil {
			return 0, e
		}
		devID, _ = res.LastInsertId()
	} else if err != nil {
		return 0, err
	}

	res, e := db.Exec(`INSERT INTO topics(device_id,topic,qos,enabled,payload_format) VALUES(?,?,1,1,'sparkplug')`,
		devID, nodeBase)
	if e != nil {
		return 0, e
	}
	topicID, _ = res.LastInsertId()
	return topicID, nil
}

// exposeSignals is the SQLite-side core of the bridge: ensure each signal's
// topic exists, skip ones already mapped (so re-running is safe), and assign the
// rest. ioaStart<=0 auto-assigns the next free IOA per signal; a positive value
// places the created points contiguously from that base. Pure SQLite — unit
// testable without TimescaleDB.
func exposeSignals(db *sqlx.DB, serverID int64, sigs []derivedSignal, ioaStart int) (exposeResult, error) {
	out := exposeResult{ServerID: serverID}
	for _, s := range sigs {
		if s.MetricName == "" || s.IEC104Type == "" {
			out.Skipped = append(out.Skipped, s.NodeBase+" "+s.MetricName+" (missing metric/type)")
			continue
		}
		topicID, err := ensureIEC104Topic(db, serverID, s.NodeBase)
		if err != nil {
			return out, err
		}
		var exists int
		if err := db.Get(&exists, `SELECT COUNT(*) FROM signal_mappings WHERE topic_id=? AND metric_name=?`,
			topicID, s.MetricName); err != nil {
			return out, err
		}
		if exists > 0 {
			out.Skipped = append(out.Skipped, s.MetricName+" (already mapped)")
			continue
		}
		ioa := 0
		if ioaStart > 0 {
			ioa = ioaStart + len(out.Created) // contiguous over points actually created
		}
		m, err := assignMapping(db, serverID, topicID, s.MetricName, s.IEC104Type, ioa, 1.0, s.Unit)
		if err != nil {
			return out, err
		}
		out.Created = append(out.Created, m)
	}
	return out, nil
}

// exposeReq is the bridge input: which IEC-104 server to expose on, and an
// optional IOA base (auto-assigned per server when omitted).
type exposeReq struct {
	ServerID int64 `json:"server_id"`
	IOAStart int   `json:"ioa_start"`
}

// querySSFVSignals reads SSFV assignments (active only) and reduces each to a
// derivedSignal. When byEquipo, id is an equipo_id (bulk); otherwise it is an
// equisenal_id (single assignment).
func (h *SSFVHandler) querySSFVSignals(ctx context.Context, byEquipo bool, id string) ([]derivedSignal, error) {
	pool := h.pool()
	if pool == nil {
		return nil, errors.New("ssfv adapter not connected")
	}
	where := "sxe.equisenal_id=$1"
	if byEquipo {
		where = "sxe.equipo_id=$1"
	}
	rows, err := pool.Query(ctx, `
		SELECT e.nombre_topic, sxe.nombre_instancia, s.codigo_senal,
		       tv.nombre AS tipo_variable, s.tipo_valor,
		       u.simbolo AS unidad,
		       (s.codigo_senal LIKE 'AL%' OR s.codigo_senal LIKE 'EF%' OR s.codigo_senal LIKE 'EV%') AS es_alarma
		FROM ssfv.tbl_senales_x_equipo sxe
		JOIN ssfv.tbl_senales       s  ON s.senal_id    = sxe.senal_id
		JOIN ssfv.tbl_tipo_variable tv ON tv.tipovar_id = s.tipovar_id
		JOIN ssfv.tbl_unidades      u  ON u.unidad_id   = s.unidad_id
		JOIN ssfv.tbl_equipo        e  ON e.equipo_id   = sxe.equipo_id
		WHERE `+where+` AND sxe.activo = TRUE
		ORDER BY sxe.equisenal_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []derivedSignal
	for rows.Next() {
		var nombreTopic, nombreInstancia, codigoSenal, tipoVariable, tipoValor, unidad string
		var esAlarma bool
		if err := rows.Scan(&nombreTopic, &nombreInstancia, &codigoSenal,
			&tipoVariable, &tipoValor, &unidad, &esAlarma); err != nil {
			return nil, err
		}
		metric := codigoSenal
		if nombreInstancia != "" {
			metric = nombreInstancia + "/" + codigoSenal
		}
		out = append(out, derivedSignal{
			NodeBase:   "spBv1.0/" + nombreTopic,
			MetricName: metric,
			IEC104Type: bridgeIEC104Type(tipoVariable, tipoValor, esAlarma),
			Unit:       unidad,
		})
	}
	return out, rows.Err()
}

// exposeIEC104 handles both bridge routes: equipo (bulk) and asignación (single).
func (h *SSFVHandler) exposeIEC104(w http.ResponseWriter, r *http.Request, byEquipo bool) {
	id := chi.URLParam(r, "id")
	var req exposeReq
	if err := decode(r, &req); err != nil {
		errResp(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ServerID == 0 {
		errResp(w, http.StatusBadRequest, "server_id required")
		return
	}
	if h.db == nil {
		errResp(w, http.StatusServiceUnavailable, "sqlite not available")
		return
	}
	var serverExists int
	if err := h.db.Get(&serverExists, `SELECT COUNT(*) FROM iec104_servers WHERE id=?`, req.ServerID); err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	if serverExists == 0 {
		errResp(w, http.StatusBadRequest, "unknown server_id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	sigs, err := h.querySSFVSignals(ctx, byEquipo, id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(sigs) == 0 {
		errResp(w, http.StatusNotFound, "no active SSFV signals found for that id")
		return
	}

	res, err := exposeSignals(h.db, req.ServerID, sigs, req.IOAStart)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(res.Created) > 0 && h.mappingReload != nil {
		h.mappingReload() // reload the worker IEC-104 cache so new points dispatch
	}
	jsonResp(w, http.StatusCreated, res)
}

func (h *SSFVHandler) exposeEquipoIEC104(w http.ResponseWriter, r *http.Request) {
	h.exposeIEC104(w, r, true)
}

func (h *SSFVHandler) exposeAsignacionIEC104(w http.ResponseWriter, r *http.Request) {
	h.exposeIEC104(w, r, false)
}
