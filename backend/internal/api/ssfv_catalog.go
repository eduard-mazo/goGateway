package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"goGateway/internal/tsdb"
)

// SSFVHandler serves CRUD endpoints for the ssfv schema catalog.
// It obtains the pool lazily from TSDBMgr so it survives pipeline reloads.
type SSFVHandler struct {
	mgr      *tsdb.Manager
	reloader func() // reloads the worker SSFVMappingCache; set via SetReloader
}

func NewSSFVHandler(mgr *tsdb.Manager) *SSFVHandler {
	return &SSFVHandler{mgr: mgr}
}

// SetReloader registers a callback invoked after any catalog mutation so the
// in-memory SSFVMappingCache (used by the Sparkplug B dispatch path) stays
// in sync without a gateway restart.
func (h *SSFVHandler) SetReloader(fn func()) { h.reloader = fn }

// triggerReload invalidates the adapter-level cache and reloads the worker
// SSFVMappingCache if a reloader has been registered.
func (h *SSFVHandler) triggerReload() {
	if a := h.mgr.SSFVAdapter(); a != nil {
		a.InvalidateCache()
	}
	if h.reloader != nil {
		h.reloader()
	}
}

func (h *SSFVHandler) Mount(r chi.Router) {
	r.Get("/status", h.getStatus)

	// Plantas
	r.Get("/plantas", h.listPlantas)
	r.Post("/plantas", h.createPlanta)
	r.Put("/plantas/{id}", h.updatePlanta)
	r.Delete("/plantas/{id}", h.deletePlanta)
	r.Get("/plantas/{id}/equipos", h.listEquiposByPlanta)

	// Equipos
	r.Get("/equipos", h.listEquipos)
	r.Post("/equipos", h.createEquipo)
	r.Put("/equipos/{id}", h.updateEquipo)
	r.Delete("/equipos/{id}", h.deleteEquipo)
	r.Get("/equipos/{id}/senales", h.listSenalesByEquipo)

	// Señales (catálogo)
	r.Get("/senales", h.listSenales)
	r.Post("/senales", h.createSenal)
	r.Put("/senales/{id}", h.updateSenal)
	r.Delete("/senales/{id}", h.deleteSenal)

	// Señales x Equipo (asignaciones)
	r.Get("/asignaciones", h.listAsignaciones)
	r.Post("/asignaciones", h.createAsignacion)
	r.Put("/asignaciones/{id}", h.updateAsignacion)
	r.Delete("/asignaciones/{id}", h.deleteAsignacion)

	// Fronteras comerciales
	r.Get("/fronteras", h.listFronteras)
	r.Post("/fronteras", h.createFrontera)
	r.Put("/fronteras/{id}", h.updateFrontera)
	r.Delete("/fronteras/{id}", h.deleteFrontera)

	// Catálogos de soporte (solo lectura para el UI)
	r.Get("/tipo-equipo", h.listTipoEquipo)
	r.Get("/tipo-variable", h.listTipoVariable)
	r.Get("/unidades", h.listUnidades)

	// Alarmas
	r.Get("/alarmas", h.listAlarmas)

	// Vistas
	r.Get("/vista/senales-contexto", h.vistaSeñalesContexto)
	r.Get("/vista/ultimas-lecturas", h.vistaUltimasLecturas)
	r.Get("/vista/alarmas-activas", h.vistaAlarmasActivas)

	// Señales sin mapeo (ring buffer de los últimos 300 signal_paths descartados)
	r.Get("/missed", h.getMissedSignals)

	// Invalidar cache
	r.Post("/cache/invalidate", h.invalidateCache)
}

// pool returns the live pgxpool or nil if adapter not connected.
func (h *SSFVHandler) pool() *pgxpool.Pool {
	a := h.mgr.SSFVAdapter()
	if a == nil {
		return nil
	}
	return a.Pool()
}

func (h *SSFVHandler) getStatus(w http.ResponseWriter, r *http.Request) {
	a := h.mgr.SSFVAdapter()
	if a == nil {
		jsonResp(w, http.StatusOK, map[string]any{"connected": false})
		return
	}
	st := a.Status()
	jsonResp(w, http.StatusOK, map[string]any{
		"connected":    true,
		"healthy":      st.Healthy,
		"write_rate":   st.WriteRate,
		"error_rate":   st.ErrorRate,
		"circuit_open": st.CircuitOpen,
		"last_error":   st.LastError,
		"skipped_rows": st.SkippedRows,
	})
}

// ─── Plantas ─────────────────────────────────────────────────────────────────

func (h *SSFVHandler) listPlantas(w http.ResponseWriter, r *http.Request) {
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT planta_id, nombre, ubicacion, propietario,
		       broker_base, capacidad_kwp, fecha_comisionamiento, estado,
		       fecha_creacion, fecha_modif
		FROM ssfv.tbl_planta ORDER BY planta_id`)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var id, estado int
		var nombre, brokerBase string
		var ubicacion, propietario, fechaCom *string
		var capacidad *float64
		var fechaCreacion, fechaModif time.Time
		if err := rows.Scan(&id, &nombre, &ubicacion, &propietario,
			&brokerBase, &capacidad, &fechaCom, &estado,
			&fechaCreacion, &fechaModif); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"planta_id": id, "nombre": nombre, "ubicacion": ubicacion,
			"propietario": propietario, "broker_base": brokerBase,
			"capacidad_kWp": capacidad, "fecha_comisionamiento": fechaCom,
			"estado": estado, "fecha_creacion": fechaCreacion, "fecha_modif": fechaModif,
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	jsonResp(w, http.StatusOK, out)
}

func (h *SSFVHandler) createPlanta(w http.ResponseWriter, r *http.Request) {
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	var body struct {
		Nombre              string   `json:"nombre"`
		Ubicacion           *string  `json:"ubicacion"`
		Propietario         *string  `json:"propietario"`
		BrokerBase          string   `json:"broker_base"`
		CapacidadKWp        *float64 `json:"capacidad_kWp"`
		FechaComisionamiento *string  `json:"fecha_comisionamiento"`
		Estado              int      `json:"estado"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var id int
	err := pool.QueryRow(ctx, `
		INSERT INTO ssfv.tbl_planta
		    (nombre, ubicacion, propietario, broker_base, capacidad_kwp, fecha_comisionamiento, estado)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING planta_id`,
		body.Nombre, body.Ubicacion, body.Propietario, body.BrokerBase,
		body.CapacidadKWp, body.FechaComisionamiento, body.Estado,
	).Scan(&id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResp(w, http.StatusCreated, map[string]any{"planta_id": id})
}

func (h *SSFVHandler) updatePlanta(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	var body struct {
		Nombre              string   `json:"nombre"`
		Ubicacion           *string  `json:"ubicacion"`
		Propietario         *string  `json:"propietario"`
		BrokerBase          string   `json:"broker_base"`
		CapacidadKWp        *float64 `json:"capacidad_kWp"`
		FechaComisionamiento *string  `json:"fecha_comisionamiento"`
		Estado              int      `json:"estado"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		UPDATE ssfv.tbl_planta
		SET nombre=$1, ubicacion=$2, propietario=$3, broker_base=$4,
		    capacidad_kwp=$5, fecha_comisionamiento=$6, estado=$7, fecha_modif=NOW()
		WHERE planta_id=$8`,
		body.Nombre, body.Ubicacion, body.Propietario, body.BrokerBase,
		body.CapacidadKWp, body.FechaComisionamiento, body.Estado, id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SSFVHandler) deletePlanta(w http.ResponseWriter, r *http.Request) {
	h.deleteRow(w, r, `DELETE FROM ssfv.tbl_planta WHERE planta_id=$1`)
}

// ─── Equipos ─────────────────────────────────────────────────────────────────

func (h *SSFVHandler) listEquipos(w http.ResponseWriter, r *http.Request) {
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	plantaID := r.URL.Query().Get("planta_id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	query := `
		SELECT e.equipo_id, e.planta_id, e.tipo_id, e.nombre_equipo,
		       e.nombre_topic, e.fabricante, e.modelo, e.nro_serie,
		       e.estado, e.fecha_creacion, e.fecha_modif,
		       te.nombre AS tipo_nombre, p.nombre AS planta_nombre
		FROM ssfv.tbl_equipo e
		JOIN ssfv.tbl_tipo_equipo te ON te.tipo_id = e.tipo_id
		JOIN ssfv.tbl_planta p ON p.planta_id = e.planta_id`
	args := []any{}
	if plantaID != "" {
		query += ` WHERE e.planta_id=$1`
		args = append(args, plantaID)
	}
	query += ` ORDER BY e.equipo_id`

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var eqID, plantaIDv, tipoID, estado int
		var nombre, nombreTopic, tipoNombre, plantaNombre string
		var fabricante, modelo, nroSerie *string
		var fechaC, fechaM time.Time
		if err := rows.Scan(&eqID, &plantaIDv, &tipoID, &nombre, &nombreTopic,
			&fabricante, &modelo, &nroSerie, &estado, &fechaC, &fechaM,
			&tipoNombre, &plantaNombre); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"equipo_id": eqID, "planta_id": plantaIDv, "tipo_id": tipoID,
			"nombre_equipo": nombre, "nombre_topic": nombreTopic,
			"fabricante": fabricante, "modelo": modelo, "nro_serie": nroSerie,
			"estado": estado, "fecha_creacion": fechaC, "fecha_modif": fechaM,
			"tipo_nombre": tipoNombre, "planta_nombre": plantaNombre,
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	jsonResp(w, http.StatusOK, out)
}

func (h *SSFVHandler) listEquiposByPlanta(w http.ResponseWriter, r *http.Request) {
	plantaID := chi.URLParam(r, "id")
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `
		SELECT e.equipo_id, e.planta_id, e.tipo_id, e.nombre_equipo,
		       e.nombre_topic, e.fabricante, e.modelo, e.nro_serie,
		       e.estado, e.fecha_creacion, e.fecha_modif,
		       te.nombre AS tipo_nombre
		FROM ssfv.tbl_equipo e
		JOIN ssfv.tbl_tipo_equipo te ON te.tipo_id = e.tipo_id
		WHERE e.planta_id=$1 ORDER BY e.equipo_id`, plantaID)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var eqID, plantaIDv, tipoID, estado int
		var nombre, nombreTopic, tipoNombre string
		var fabricante, modelo, nroSerie *string
		var fechaC, fechaM time.Time
		if err := rows.Scan(&eqID, &plantaIDv, &tipoID, &nombre, &nombreTopic,
			&fabricante, &modelo, &nroSerie, &estado, &fechaC, &fechaM, &tipoNombre); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"equipo_id": eqID, "planta_id": plantaIDv, "tipo_id": tipoID,
			"nombre_equipo": nombre, "nombre_topic": nombreTopic,
			"fabricante": fabricante, "modelo": modelo, "nro_serie": nroSerie,
			"estado": estado, "fecha_creacion": fechaC, "fecha_modif": fechaM,
			"tipo_nombre": tipoNombre,
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	jsonResp(w, http.StatusOK, out)
}

func (h *SSFVHandler) listSenalesByEquipo(w http.ResponseWriter, r *http.Request) {
	equipoID := chi.URLParam(r, "id")
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `
		SELECT sxe.equisenal_id, sxe.senal_id, sxe.equipo_id,
		       sxe.indice_canal, sxe.nombre_instancia, sxe.activo,
		       s.nombre AS senal_nombre, s.codigo_senal,
		       (s.codigo_senal LIKE 'AL%' OR s.codigo_senal LIKE 'EF%' OR s.codigo_senal LIKE 'EV%') AS es_alarma,
		       s.es_indexada, s.tipo_valor,
		       tv.nombre AS tipo_variable, u.simbolo AS unidad,
		       e.nombre_topic
		FROM ssfv.tbl_senales_x_equipo sxe
		JOIN ssfv.tbl_senales       s   ON s.senal_id    = sxe.senal_id
		JOIN ssfv.tbl_tipo_variable tv  ON tv.tipovar_id = s.tipovar_id
		JOIN ssfv.tbl_unidades      u   ON u.unidad_id   = s.unidad_id
		JOIN ssfv.tbl_equipo        e   ON e.equipo_id   = sxe.equipo_id
		WHERE sxe.equipo_id=$1
		ORDER BY sxe.equisenal_id`, equipoID)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var equiSenalID, senalID, equipoIDv int
		var nombreInstancia, senalNombre, codigoSenal, tipoVariable, unidad, tipoValor, nombreTopic string
		var indiceCanal *int
		var activo, esAlarma, esIndexada bool
		if err := rows.Scan(&equiSenalID, &senalID, &equipoIDv,
			&indiceCanal, &nombreInstancia, &activo,
			&senalNombre, &codigoSenal, &esAlarma, &esIndexada, &tipoValor,
			&tipoVariable, &unidad, &nombreTopic); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"equisenal_id":    equiSenalID,
			"senal_id":        senalID,
			"equipo_id":       equipoIDv,
			"indice_canal":    indiceCanal,
			"nombre_instancia": nombreInstancia,
			"activo":          activo,
			"senal_nombre":    senalNombre,
			"codigo_senal":    codigoSenal,
			"es_alarma":       esAlarma,
			"es_indexada":     esIndexada,
			"tipo_valor":      tipoValor,
			"tipo_variable":   tipoVariable,
			"unidad":          unidad,
			"signal_path":     nombreTopic + "/" + nombreInstancia,
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	jsonResp(w, http.StatusOK, out)
}

func (h *SSFVHandler) createEquipo(w http.ResponseWriter, r *http.Request) {
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	var body struct {
		PlantaID     int     `json:"planta_id"`
		TipoID       int     `json:"tipo_id"`
		NombreEquipo string  `json:"nombre_equipo"`
		NombreTopic  string  `json:"nombre_topic"`
		Fabricante   *string `json:"fabricante"`
		Modelo       *string `json:"modelo"`
		NroSerie     *string `json:"nro_serie"`
		Estado       int     `json:"estado"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Insert equipo.
	var id int
	err := pool.QueryRow(ctx, `
		INSERT INTO ssfv.tbl_equipo
		    (planta_id, tipo_id, nombre_equipo, nombre_topic, fabricante, modelo, nro_serie, estado)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING equipo_id`,
		body.PlantaID, body.TipoID, body.NombreEquipo, body.NombreTopic,
		body.Fabricante, body.Modelo, body.NroSerie, body.Estado,
	).Scan(&id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Auto-instanciate signals from Tbl_Senales_x_Tipo_Equipo.
	if autoErr := h.autoInstanciarSenales(ctx, pool, id, body.TipoID); autoErr != nil {
		log.Printf("ssfv: auto-instanciar equipo %d tipo %d: %v", id, body.TipoID, autoErr)
	}

	h.triggerReload()
	jsonResp(w, http.StatusCreated, map[string]any{"equipo_id": id})
}

// autoInstanciarSenales creates Tbl_Senales_x_Equipo rows for every signal
// in the tipo_equipo catalog, handling indexed signals by expanding channels.
func (h *SSFVHandler) autoInstanciarSenales(ctx context.Context, pool *pgxpool.Pool, equipoID, tipoID int) error {
	rows, err := pool.Query(ctx, `
		SELECT s.senal_id, s.codigo_senal, s.es_indexada, st.num_canales
		FROM public.tbl_senales_x_tipo_equipo st
		JOIN ssfv.tbl_senales s ON s.senal_id = st.senal_id
		WHERE st.tipo_id = $1 AND s.activo = TRUE`, tipoID)
	if err != nil {
		return fmt.Errorf("query senales x tipo: %w", err)
	}
	defer rows.Close()

	type senal struct {
		senalID     int
		codigoSenal string
		esIndexada  bool
		numCanales  int
	}
	var senales []senal
	for rows.Next() {
		var s senal
		if err := rows.Scan(&s.senalID, &s.codigoSenal, &s.esIndexada, &s.numCanales); err != nil {
			continue
		}
		senales = append(senales, s)
	}

	for _, s := range senales {
		if s.esIndexada {
			base := strings.TrimSuffix(s.codigoSenal, "_x")
			for i := 1; i <= s.numCanales; i++ {
				instancia := base + "_" + strconv.Itoa(i)
				_, err := pool.Exec(ctx, `
					INSERT INTO ssfv.tbl_senales_x_equipo
					    (senal_id, equipo_id, indice_canal, nombre_instancia, activo)
					VALUES ($1,$2,$3,$4,TRUE)
					ON CONFLICT DO NOTHING`,
					s.senalID, equipoID, i, instancia)
				if err != nil {
					log.Printf("ssfv: insert sxe %s idx %d: %v", instancia, i, err)
				}
			}
		} else {
			_, err := pool.Exec(ctx, `
				INSERT INTO ssfv.tbl_senales_x_equipo
				    (senal_id, equipo_id, nombre_instancia, activo)
				VALUES ($1,$2,$3,TRUE)
				ON CONFLICT DO NOTHING`,
				s.senalID, equipoID, s.codigoSenal)
			if err != nil {
				log.Printf("ssfv: insert sxe %s: %v", s.codigoSenal, err)
			}
		}
	}
	return nil
}

func (h *SSFVHandler) updateEquipo(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	var body struct {
		PlantaID    int     `json:"planta_id"`
		TipoID      int     `json:"tipo_id"`
		NombreEquipo string `json:"nombre_equipo"`
		NombreTopic  string `json:"nombre_topic"`
		Fabricante  *string `json:"fabricante"`
		Modelo      *string `json:"modelo"`
		NroSerie    *string `json:"nro_serie"`
		Estado      int     `json:"estado"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		UPDATE ssfv.tbl_equipo
		SET planta_id=$1, tipo_id=$2, nombre_equipo=$3, nombre_topic=$4,
		    fabricante=$5, modelo=$6, nro_serie=$7, estado=$8, fecha_modif=NOW()
		WHERE equipo_id=$9`,
		body.PlantaID, body.TipoID, body.NombreEquipo, body.NombreTopic,
		body.Fabricante, body.Modelo, body.NroSerie, body.Estado, id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.triggerReload()
	w.WriteHeader(http.StatusNoContent)
}

func (h *SSFVHandler) deleteEquipo(w http.ResponseWriter, r *http.Request) {
	h.deleteRow(w, r, `DELETE FROM ssfv.tbl_equipo WHERE equipo_id=$1`)
}

// ─── Señales ─────────────────────────────────────────────────────────────────

func (h *SSFVHandler) listSenales(w http.ResponseWriter, r *http.Request) {
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT s.senal_id, s.tipovar_id, s.unidad_id, s.nombre, s.descripcion,
		       s.tipo_valor, s.codigo_senal, s.es_indexada, s.activo, s.fecha_alta,
		       tv.nombre AS tipo_var_nombre, u.simbolo AS unidad_simbolo
		FROM ssfv.tbl_senales s
		JOIN ssfv.tbl_tipo_variable tv ON tv.tipovar_id = s.tipovar_id
		JOIN ssfv.tbl_unidades u ON u.unidad_id = s.unidad_id
		ORDER BY s.senal_id`)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var id, tipoVarID, unidadID int
		var nombre, tipoValor, codigoSenal, tipoVarNombre, unidadSimbolo string
		var descripcion *string
		var esIndexada, activo bool
		var fechaAlta time.Time
		if err := rows.Scan(&id, &tipoVarID, &unidadID, &nombre, &descripcion,
			&tipoValor, &codigoSenal, &esIndexada, &activo, &fechaAlta,
			&tipoVarNombre, &unidadSimbolo); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"senal_id": id, "tipavar_id": tipoVarID, "unidad_id": unidadID,
			"nombre": nombre, "descripcion": descripcion,
			"tipo_valor": tipoValor, "codigo_senal": codigoSenal,
			"es_indexada": esIndexada, "activo": activo, "fecha_alta": fechaAlta,
			"tipo_var_nombre": tipoVarNombre, "unidad_simbolo": unidadSimbolo,
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	jsonResp(w, http.StatusOK, out)
}

func (h *SSFVHandler) createSenal(w http.ResponseWriter, r *http.Request) {
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	var body struct {
		TipoVarID   int     `json:"tipavar_id"`
		UnidadID    int     `json:"unidad_id"`
		Nombre      string  `json:"nombre"`
		Descripcion *string `json:"descripcion"`
		TipoValor   string  `json:"tipo_valor"`
		CodigoSenal string  `json:"codigo_senal"`
		EsIndexada  bool    `json:"es_indexada"`
		Activo      bool    `json:"activo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var id int
	err := pool.QueryRow(ctx, `
		INSERT INTO ssfv.tbl_senales
		    (tipovar_id, unidad_id, nombre, descripcion, tipo_valor, codigo_senal, es_indexada, activo)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING senal_id`,
		body.TipoVarID, body.UnidadID, body.Nombre, body.Descripcion,
		body.TipoValor, body.CodigoSenal, body.EsIndexada, body.Activo,
	).Scan(&id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResp(w, http.StatusCreated, map[string]any{"senal_id": id})
}

func (h *SSFVHandler) updateSenal(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	var body struct {
		TipoVarID   int     `json:"tipavar_id"`
		UnidadID    int     `json:"unidad_id"`
		Nombre      string  `json:"nombre"`
		Descripcion *string `json:"descripcion"`
		TipoValor   string  `json:"tipo_valor"`
		CodigoSenal string  `json:"codigo_senal"`
		EsIndexada  bool    `json:"es_indexada"`
		Activo      bool    `json:"activo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		UPDATE ssfv.tbl_senales
		SET tipovar_id=$1, unidad_id=$2, nombre=$3, descripcion=$4,
		    tipo_valor=$5, codigo_senal=$6, es_indexada=$7, activo=$8
		WHERE senal_id=$9`,
		body.TipoVarID, body.UnidadID, body.Nombre, body.Descripcion,
		body.TipoValor, body.CodigoSenal, body.EsIndexada, body.Activo, id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SSFVHandler) deleteSenal(w http.ResponseWriter, r *http.Request) {
	h.deleteRow(w, r, `DELETE FROM ssfv.tbl_senales WHERE senal_id=$1`)
}

// ─── Asignaciones (Señales x Equipo) ─────────────────────────────────────────

func (h *SSFVHandler) listAsignaciones(w http.ResponseWriter, r *http.Request) {
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	equipoID := r.URL.Query().Get("equipo_id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	query := `
		SELECT sxe.equisenal_id, sxe.senal_id, sxe.equipo_id,
		       sxe.indice_canal, sxe.nombre_instancia, sxe.activo, sxe.fecha_alta,
		       s.nombre AS senal_nombre, s.codigo_senal,
		       e.nombre_equipo, e.nombre_topic
		FROM ssfv.tbl_senales_x_equipo sxe
		JOIN ssfv.tbl_senales s ON s.senal_id = sxe.senal_id
		JOIN ssfv.tbl_equipo e ON e.equipo_id = sxe.equipo_id`
	args := []any{}
	if equipoID != "" {
		query += ` WHERE sxe.equipo_id=$1`
		args = append(args, equipoID)
	}
	query += ` ORDER BY sxe.equisenal_id`

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var equiSenalID, senalID, equipoIDv int
		var nombreInstancia, senalNombre, codigoSenal, nombreEquipo, nombreTopic string
		var indiceCanal *int
		var activo bool
		var fechaAlta time.Time
		if err := rows.Scan(&equiSenalID, &senalID, &equipoIDv,
			&indiceCanal, &nombreInstancia, &activo, &fechaAlta,
			&senalNombre, &codigoSenal, &nombreEquipo, &nombreTopic); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"equisenal_id": equiSenalID, "senal_id": senalID, "equipo_id": equipoIDv,
			"indice_canal": indiceCanal, "nombre_instancia": nombreInstancia,
			"activo": activo, "fecha_alta": fechaAlta,
			"senal_nombre": senalNombre, "codigo_senal": codigoSenal,
			"nombre_equipo": nombreEquipo, "nombre_topic": nombreTopic,
			"signal_path": nombreTopic + "/" + nombreInstancia,
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	jsonResp(w, http.StatusOK, out)
}

func (h *SSFVHandler) createAsignacion(w http.ResponseWriter, r *http.Request) {
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	var body struct {
		SenalID         int     `json:"senal_id"`
		EquipoID        int     `json:"equipo_id"`
		IndiceCanal     *int    `json:"indice_canal"`
		NombreInstancia string  `json:"nombre_instancia"`
		Activo          bool    `json:"activo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var id int
	err := pool.QueryRow(ctx, `
		INSERT INTO ssfv.tbl_senales_x_equipo
		    (senal_id, equipo_id, indice_canal, nombre_instancia, activo)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING equisenal_id`,
		body.SenalID, body.EquipoID, body.IndiceCanal,
		body.NombreInstancia, body.Activo,
	).Scan(&id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.triggerReload()
	jsonResp(w, http.StatusCreated, map[string]any{"equisenal_id": id})
}

func (h *SSFVHandler) updateAsignacion(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	var body struct {
		SenalID         int    `json:"senal_id"`
		EquipoID        int    `json:"equipo_id"`
		IndiceCanal     *int   `json:"indice_canal"`
		NombreInstancia string `json:"nombre_instancia"`
		Activo          bool   `json:"activo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		UPDATE ssfv.tbl_senales_x_equipo
		SET senal_id=$1, equipo_id=$2, indice_canal=$3,
		    nombre_instancia=$4, activo=$5
		WHERE equisenal_id=$6`,
		body.SenalID, body.EquipoID, body.IndiceCanal,
		body.NombreInstancia, body.Activo, id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.triggerReload()
	w.WriteHeader(http.StatusNoContent)
}

func (h *SSFVHandler) deleteAsignacion(w http.ResponseWriter, r *http.Request) {
	h.triggerReload()
	h.deleteRow(w, r, `DELETE FROM ssfv.tbl_senales_x_equipo WHERE equisenal_id=$1`)
}

// ─── Fronteras Comerciales ────────────────────────────────────────────────────

func (h *SSFVHandler) listFronteras(w http.ResponseWriter, r *http.Request) {
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT f.frontera_id, f.planta_id, f.codigo_nie, f.nombre,
		       f.tipo_conexion, f.activo, f.fecha_creacion,
		       p.nombre AS planta_nombre
		FROM ssfv.tbl_frontera_comercial f
		JOIN ssfv.tbl_planta p ON p.planta_id = f.planta_id
		ORDER BY f.frontera_id`)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var frontID, plantaID int
		var codigoNIE, nombre, plantaNombre string
		var tipoConexion *string
		var activo bool
		var fechaC time.Time
		if err := rows.Scan(&frontID, &plantaID, &codigoNIE, &nombre,
			&tipoConexion, &activo, &fechaC, &plantaNombre); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"frontera_id": frontID, "planta_id": plantaID,
			"codigo_nie": codigoNIE, "nombre": nombre,
			"tipo_conexion": tipoConexion, "activo": activo,
			"fecha_creacion": fechaC, "planta_nombre": plantaNombre,
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	jsonResp(w, http.StatusOK, out)
}

func (h *SSFVHandler) createFrontera(w http.ResponseWriter, r *http.Request) {
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	var body struct {
		PlantaID     int     `json:"planta_id"`
		CodigoNIE    string  `json:"codigo_nie"`
		Nombre       string  `json:"nombre"`
		TipoConexion *string `json:"tipo_conexion"`
		Activo       bool    `json:"activo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var id int
	err := pool.QueryRow(ctx, `
		INSERT INTO ssfv.tbl_frontera_comercial
		    (planta_id, codigo_nie, nombre, tipo_conexion, activo)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING frontera_id`,
		body.PlantaID, body.CodigoNIE, body.Nombre, body.TipoConexion, body.Activo,
	).Scan(&id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResp(w, http.StatusCreated, map[string]any{"frontera_id": id})
}

func (h *SSFVHandler) updateFrontera(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	var body struct {
		PlantaID     int     `json:"planta_id"`
		CodigoNIE    string  `json:"codigo_nie"`
		Nombre       string  `json:"nombre"`
		TipoConexion *string `json:"tipo_conexion"`
		Activo       bool    `json:"activo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		UPDATE ssfv.tbl_frontera_comercial
		SET planta_id=$1, codigo_nie=$2, nombre=$3,
		    tipo_conexion=$4, activo=$5, fecha_modif=NOW()
		WHERE frontera_id=$6`,
		body.PlantaID, body.CodigoNIE, body.Nombre, body.TipoConexion, body.Activo, id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SSFVHandler) deleteFrontera(w http.ResponseWriter, r *http.Request) {
	h.deleteRow(w, r, `DELETE FROM ssfv.tbl_frontera_comercial WHERE frontera_id=$1`)
}

// ─── Catálogos de soporte ─────────────────────────────────────────────────────

func (h *SSFVHandler) listTipoEquipo(w http.ResponseWriter, r *http.Request) {
	h.queryJSON(w, r, `SELECT tipo_id, nombre, descripcion, activo FROM ssfv.tbl_tipo_equipo ORDER BY tipo_id`)
}

func (h *SSFVHandler) listTipoVariable(w http.ResponseWriter, r *http.Request) {
	h.queryJSON(w, r, `SELECT tipovar_id, nombre, descripcion, activo FROM ssfv.tbl_tipo_variable ORDER BY tipovar_id`)
}

func (h *SSFVHandler) listUnidades(w http.ResponseWriter, r *http.Request) {
	h.queryJSON(w, r, `SELECT unidad_id, simbolo, nombre, magnitud, activo FROM ssfv.tbl_unidades ORDER BY unidad_id`)
}

// ─── Vistas ───────────────────────────────────────────────────────────────────

func (h *SSFVHandler) vistaSeñalesContexto(w http.ResponseWriter, r *http.Request) {
	h.queryJSON(w, r, `SELECT * FROM ssfv.v_senales_contexto ORDER BY equisenal_id`)
}

func (h *SSFVHandler) vistaUltimasLecturas(w http.ResponseWriter, r *http.Request) {
	h.queryJSON(w, r, `SELECT * FROM ssfv.v_ultimas_lecturas ORDER BY equisenal_id`)
}

func (h *SSFVHandler) vistaAlarmasActivas(w http.ResponseWriter, r *http.Request) {
	h.queryJSON(w, r, `SELECT * FROM ssfv.v_alarmas_activas`)
}

// ─── Alarmas ─────────────────────────────────────────────────────────────────

func (h *SSFVHandler) listAlarmas(w http.ResponseWriter, r *http.Request) {
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	equipoID := r.URL.Query().Get("equipo_id")
	query := `
		SELECT a.alarma_id, a.equisenal_id, a.ts_inicio, a.ts_fin,
		       a.tipo_alarma, a.severidad, a.descripcion, a.activa,
		       e.nombre_equipo, e.nombre_topic,
		       p.nombre AS planta_nombre,
		       sxe.nombre_instancia
		FROM ssfv.tbl_alarmas a
		JOIN ssfv.tbl_senales_x_equipo sxe ON sxe.equisenal_id = a.equisenal_id
		JOIN ssfv.tbl_equipo           e   ON e.equipo_id      = sxe.equipo_id
		JOIN ssfv.tbl_planta           p   ON p.planta_id      = e.planta_id`
	args := []any{}
	if equipoID != "" {
		query += ` WHERE e.equipo_id=$1`
		args = append(args, equipoID)
	} else {
		query += ` WHERE a.activa = TRUE`
	}
	query += ` ORDER BY a.ts_inicio DESC LIMIT 200`

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var alarmaID, equiSenalID int64
		var tipoAlarma, severidad, nombreEquipo, nombreTopic, plantaNombre, nombreInstancia string
		var descripcion *string
		var tsFin *time.Time
		var tsInicio time.Time
		var activa bool
		if err := rows.Scan(&alarmaID, &equiSenalID, &tsInicio, &tsFin,
			&tipoAlarma, &severidad, &descripcion, &activa,
			&nombreEquipo, &nombreTopic, &plantaNombre, &nombreInstancia); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"alarma_id":       alarmaID,
			"equisenal_id":    equiSenalID,
			"ts_inicio":       tsInicio,
			"ts_fin":          tsFin,
			"tipo_alarma":     tipoAlarma,
			"severidad":       severidad,
			"descripcion":     descripcion,
			"activa":          activa,
			"nombre_equipo":   nombreEquipo,
			"nombre_topic":    nombreTopic,
			"planta_nombre":   plantaNombre,
			"nombre_instancia": nombreInstancia,
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	jsonResp(w, http.StatusOK, out)
}

// ─── Cache ────────────────────────────────────────────────────────────────────

// getMissedSignals returns the deduplicated ring buffer of signal_paths that
// arrived from the broker but had no catalog match in tbl_senales_x_equipo.
// These are actionable: configure the signal in the SSFV catalog to persist it.
func (h *SSFVHandler) getMissedSignals(w http.ResponseWriter, r *http.Request) {
	a := h.mgr.SSFVAdapter()
	if a == nil {
		jsonResp(w, http.StatusOK, []any{})
		return
	}
	misses := a.RecentMisses()
	if misses == nil {
		misses = []tsdb.MissedSignal{}
	}
	jsonResp(w, http.StatusOK, misses)
}

func (h *SSFVHandler) invalidateCache(w http.ResponseWriter, r *http.Request) {
	if h.mgr.SSFVAdapter() == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	h.triggerReload()
	jsonResp(w, http.StatusOK, map[string]any{"invalidated": true})
}

// ─── Generic helpers ──────────────────────────────────────────────────────────

func (h *SSFVHandler) deleteRow(w http.ResponseWriter, r *http.Request, query string) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, query, id); err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// queryJSON runs a raw SELECT and serializes rows as a JSON array.
// Column names come from the result description. Use only for simple read queries.
func (h *SSFVHandler) queryJSON(w http.ResponseWriter, r *http.Request, query string) {
	pool := h.pool()
	if pool == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, query)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	cols := rows.FieldDescriptions()
	var out []map[string]any
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			continue
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[string(col.Name)] = vals[i]
		}
		out = append(out, row)
	}
	if out == nil {
		out = []map[string]any{}
	}
	jsonResp(w, http.StatusOK, out)
}

func jsonResp(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func errResp(w http.ResponseWriter, status int, msg string) {
	jsonResp(w, status, map[string]any{"error": msg})
}
