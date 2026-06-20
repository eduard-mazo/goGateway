package api

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
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

// derivedSignal is one SSFV assignment reduced to the fields an IEC-104 mirror
// needs. Topic is the stored topic string (a full canonical Sparkplug topic —
// see ssfvStorageTopic); MetricName is "<nombre_instancia>/<codigo_senal>". The
// PlantaID/EquipoID/EquisenalID link the mirror back to its SSFV source so it
// cascade-deletes.
type derivedSignal struct {
	PlantaID    int64
	EquipoID    int64
	EquisenalID int64
	Topic       string
	MetricName  string
	IEC104Type  string
	Unit        string
}

// ssfvStorageTopic turns an SSFV nombre_topic ("group/node" or
// "group/node/device") into a FULL Sparkplug topic that the worker's spNodeBase
// re-parses back to the exact NodeBase()/DeviceBase() the live dispatch path
// uses. A bare device base (spBv1.0/group/node/device) must NOT be stored: it has
// four segments, which ParseTopic mis-reads (node/device shifted) since it does
// not validate the message-type segment — so the cache key would never match the
// runtime DeviceBase(). Inserting a message-type segment (DDATA/NDATA, ignored
// by NodeBase/DeviceBase) makes the round-trip exact.
func ssfvStorageTopic(nombreTopic string) string {
	switch parts := strings.Split(nombreTopic, "/"); len(parts) {
	case 3: // group/node/device → spBv1.0/group/DDATA/node/device
		return "spBv1.0/" + parts[0] + "/DDATA/" + parts[1] + "/" + parts[2]
	case 2: // group/node → spBv1.0/group/NDATA/node
		return "spBv1.0/" + parts[0] + "/NDATA/" + parts[1]
	default:
		return "spBv1.0/" + nombreTopic
	}
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

// ensureIEC104Topic finds (or creates) the topic row for a stored Sparkplug
// topic on the given server, so a mapping can reference it. Bridge-created
// devices are named "iec104:<topic>" and tagged so they're recognisable.
// Idempotent.
func ensureIEC104Topic(db *sqlx.DB, serverID int64, topic string) (int64, error) {
	var topicID int64
	err := db.Get(&topicID, `
		SELECT t.id FROM topics t JOIN devices d ON d.id = t.device_id
		WHERE d.server_id=? AND t.topic=? LIMIT 1`, serverID, topic)
	if err == nil {
		return topicID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	devName := "iec104:" + topic
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
		devID, topic)
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
			out.Skipped = append(out.Skipped, s.Topic+" "+s.MetricName+" (missing metric/type)")
			continue
		}
		topicID, err := ensureIEC104Topic(db, serverID, s.Topic)
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
		m, err := assignMapping(db, mappingSpec{
			ServerID: serverID, TopicID: topicID, MetricName: s.MetricName,
			IEC104Type: s.IEC104Type, IOA: ioa, Unit: s.Unit,
			SSFVPlantaID: s.PlantaID, SSFVEquipoID: s.EquipoID, SSFVEquisenalID: s.EquisenalID,
		})
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

// querySSFVSignals reads active SSFV assignments and reduces each to a
// derivedSignal (carrying the planta/equipo/equisenal refs). scopeCol selects
// the filter column: "e.planta_id" (whole plant), "sxe.equipo_id" (one equipo),
// or "sxe.equisenal_id" (one assignment).
func (h *SSFVHandler) querySSFVSignals(ctx context.Context, scopeCol, id string) ([]derivedSignal, error) {
	pool := h.pool()
	if pool == nil {
		return nil, errors.New("ssfv adapter not connected")
	}
	rows, err := pool.Query(ctx, `
		SELECT e.planta_id, sxe.equipo_id, sxe.equisenal_id,
		       e.nombre_topic, sxe.nombre_instancia, s.codigo_senal,
		       tv.nombre AS tipo_variable, s.tipo_valor,
		       u.simbolo AS unidad,
		       (s.codigo_senal LIKE 'AL%' OR s.codigo_senal LIKE 'EF%' OR s.codigo_senal LIKE 'EV%') AS es_alarma
		FROM ssfv.tbl_senales_x_equipo sxe
		JOIN ssfv.tbl_senales       s  ON s.senal_id    = sxe.senal_id
		JOIN ssfv.tbl_tipo_variable tv ON tv.tipovar_id = s.tipovar_id
		JOIN ssfv.tbl_unidades      u  ON u.unidad_id   = s.unidad_id
		JOIN ssfv.tbl_equipo        e  ON e.equipo_id   = sxe.equipo_id
		WHERE `+scopeCol+`=$1 AND sxe.activo = TRUE
		ORDER BY sxe.equisenal_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []derivedSignal
	for rows.Next() {
		var plantaID, equipoID, equisenalID int64
		var nombreTopic, nombreInstancia, codigoSenal, tipoVariable, tipoValor, unidad string
		var esAlarma bool
		if err := rows.Scan(&plantaID, &equipoID, &equisenalID,
			&nombreTopic, &nombreInstancia, &codigoSenal,
			&tipoVariable, &tipoValor, &unidad, &esAlarma); err != nil {
			return nil, err
		}
		metric := codigoSenal
		if nombreInstancia != "" {
			metric = nombreInstancia + "/" + codigoSenal
		}
		out = append(out, derivedSignal{
			PlantaID:    plantaID,
			EquipoID:    equipoID,
			EquisenalID: equisenalID,
			Topic:       ssfvStorageTopic(nombreTopic),
			MetricName:  metric,
			IEC104Type:  bridgeIEC104Type(tipoVariable, tipoValor, esAlarma),
			Unit:        unidad,
		})
	}
	return out, rows.Err()
}

// cascadeDeleteMirrors removes IEC-104 mirrors whose SSFV link column matches id
// and reloads the worker cache if any went away. Called by the SSFV delete
// handlers so removing an SSFV signal/equipo/planta removes its IEC-104 points.
// col is an internal constant (ssfv_planta_id|ssfv_equipo_id|ssfv_equisenal_id).
func (h *SSFVHandler) cascadeDeleteMirrors(col string, id int64) {
	if h.db == nil {
		return
	}
	res, err := h.db.Exec(`DELETE FROM signal_mappings WHERE `+col+`=?`, id)
	if err != nil {
		log.Printf("iec104 mirror cascade (%s=%d): %v", col, id, err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 && h.mappingReload != nil {
		h.mappingReload()
	}
}

// autoMirrorEquisenal mirrors one freshly-created SSFV assignment onto IEC-104,
// but ONLY if its plant is linked to a server. Best-effort: a missing link or an
// unreachable catalog simply means no mirror (logged, never fatal to the SSFV op).
func (h *SSFVHandler) autoMirrorEquisenal(ctx context.Context, equisenalID int64) {
	if h.db == nil || h.pool() == nil {
		return
	}
	sigs, err := h.querySSFVSignals(ctx, "sxe.equisenal_id", strconv.FormatInt(equisenalID, 10))
	if err != nil || len(sigs) == 0 {
		return
	}
	var serverID int64
	err = h.db.Get(&serverID, `SELECT server_id FROM ssfv_iec104_link WHERE planta_id=?`, sigs[0].PlantaID)
	if errors.Is(err, sql.ErrNoRows) {
		return // plant not linked → no mirror
	}
	if err != nil {
		log.Printf("iec104 auto-mirror link lookup (planta=%d): %v", sigs[0].PlantaID, err)
		return
	}
	res, err := exposeSignals(h.db, serverID, sigs, 0)
	if err != nil {
		log.Printf("iec104 auto-mirror (equisenal=%d): %v", equisenalID, err)
		return
	}
	if len(res.Created) > 0 && h.mappingReload != nil {
		h.mappingReload()
	}
}

// plantaLinkStatus is the GET response for a plant's IEC-104 link.
type plantaLinkStatus struct {
	PlantaID    int64 `json:"planta_id"`
	Linked      bool  `json:"linked"`
	ServerID    int64 `json:"server_id"`
	MirrorCount int   `json:"mirror_count"`
}

// getPlantaIEC104Link reports whether a plant is linked to an IEC-104 server and
// how many mirrors currently exist for it.
func (h *SSFVHandler) getPlantaIEC104Link(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		errResp(w, http.StatusServiceUnavailable, "sqlite not available")
		return
	}
	plantaID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		errResp(w, http.StatusBadRequest, "invalid planta id")
		return
	}
	st := plantaLinkStatus{PlantaID: plantaID}
	err = h.db.Get(&st.ServerID, `SELECT server_id FROM ssfv_iec104_link WHERE planta_id=?`, plantaID)
	if err == nil {
		st.Linked = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.db.Get(&st.MirrorCount, `SELECT COUNT(*) FROM signal_mappings WHERE ssfv_planta_id=?`, plantaID)
	jsonResp(w, http.StatusOK, st)
}

// linkPlantaIEC104 links a plant to an IEC-104 server (operator-chosen) and
// backfills mirrors for the plant's existing SSFV signals. Idempotent: re-linking
// updates the server and (re)mirrors any not-yet-mirrored signals. From here on,
// new signals on this plant auto-mirror (see createAsignacion).
func (h *SSFVHandler) linkPlantaIEC104(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		errResp(w, http.StatusServiceUnavailable, "sqlite not available")
		return
	}
	plantaID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		errResp(w, http.StatusBadRequest, "invalid planta id")
		return
	}
	var req exposeReq
	if err := decode(r, &req); err != nil {
		errResp(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ServerID == 0 {
		errResp(w, http.StatusBadRequest, "server_id required")
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
	if _, err := h.db.Exec(
		`INSERT INTO ssfv_iec104_link(planta_id,server_id) VALUES(?,?)
		 ON CONFLICT(planta_id) DO UPDATE SET server_id=excluded.server_id`,
		plantaID, req.ServerID); err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	sigs, err := h.querySSFVSignals(ctx, "e.planta_id", strconv.FormatInt(plantaID, 10))
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	res, err := exposeSignals(h.db, req.ServerID, sigs, req.IOAStart)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(res.Created) > 0 && h.mappingReload != nil {
		h.mappingReload()
	}
	jsonResp(w, http.StatusCreated, res)
}

// unlinkPlantaIEC104 removes a plant's IEC-104 link and all its mirrors.
func (h *SSFVHandler) unlinkPlantaIEC104(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		errResp(w, http.StatusServiceUnavailable, "sqlite not available")
		return
	}
	plantaID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		errResp(w, http.StatusBadRequest, "invalid planta id")
		return
	}
	if _, err := h.db.Exec(`DELETE FROM ssfv_iec104_link WHERE planta_id=?`, plantaID); err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.cascadeDeleteMirrors("ssfv_planta_id", plantaID)
	jsonResp(w, http.StatusOK, map[string]any{"planta_id": plantaID, "unlinked": true})
}
