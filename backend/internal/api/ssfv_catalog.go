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
	mgr *tsdb.Manager
}

func NewSSFVHandler(mgr *tsdb.Manager) *SSFVHandler {
	return &SSFVHandler{mgr: mgr}
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
	r.Get("/vista/raw", h.vistaRaw)

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
		SELECT "Planta_Id","Nombre","Ubicacion","Propietario",
		       "Broker_Base","Capacidad_kWp","Fecha_Comisionamiento","Estado",
		       "Fecha_Creacion","Fecha_Modif"
		FROM ssfv."Tbl_Planta" ORDER BY "Planta_Id"`)
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
		INSERT INTO ssfv."Tbl_Planta"
		    ("Nombre","Ubicacion","Propietario","Broker_Base","Capacidad_kWp","Fecha_Comisionamiento","Estado")
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING "Planta_Id"`,
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
		UPDATE ssfv."Tbl_Planta"
		SET "Nombre"=$1,"Ubicacion"=$2,"Propietario"=$3,"Broker_Base"=$4,
		    "Capacidad_kWp"=$5,"Fecha_Comisionamiento"=$6,"Estado"=$7,"Fecha_Modif"=NOW()
		WHERE "Planta_Id"=$8`,
		body.Nombre, body.Ubicacion, body.Propietario, body.BrokerBase,
		body.CapacidadKWp, body.FechaComisionamiento, body.Estado, id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SSFVHandler) deletePlanta(w http.ResponseWriter, r *http.Request) {
	h.deleteRow(w, r, `DELETE FROM ssfv."Tbl_Planta" WHERE "Planta_Id"=$1`)
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
		SELECT e."Equipo_Id",e."Planta_Id",e."Tipo_Id",e."Nombre_Equipo",
		       e."Nombre_Topic",e."Fabricante",e."Modelo",e."Nro_Serie",
		       e."Estado",e."Fecha_Creacion",e."Fecha_Modif",
		       te."Nombre" AS tipo_nombre, p."Nombre" AS planta_nombre
		FROM ssfv."Tbl_Equipo" e
		JOIN ssfv."Tbl_Tipo_Equipo" te ON te."Tipo_Id" = e."Tipo_Id"
		JOIN ssfv."Tbl_Planta" p ON p."Planta_Id" = e."Planta_Id"`
	args := []any{}
	if plantaID != "" {
		query += ` WHERE e."Planta_Id"=$1`
		args = append(args, plantaID)
	}
	query += ` ORDER BY e."Equipo_Id"`

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
		SELECT e."Equipo_Id",e."Planta_Id",e."Tipo_Id",e."Nombre_Equipo",
		       e."Nombre_Topic",e."Fabricante",e."Modelo",e."Nro_Serie",
		       e."Estado",e."Fecha_Creacion",e."Fecha_Modif",
		       te."Nombre" AS tipo_nombre
		FROM ssfv."Tbl_Equipo" e
		JOIN ssfv."Tbl_Tipo_Equipo" te ON te."Tipo_Id" = e."Tipo_Id"
		WHERE e."Planta_Id"=$1 ORDER BY e."Equipo_Id"`, plantaID)
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
		SELECT sxe."EquiSenal_Id", sxe."Senal_Id", sxe."Equipo_Id",
		       sxe."Indice_Canal", sxe."Nombre_Instancia", sxe."Activo",
		       s."Nombre" AS senal_nombre, s."Codigo_Senal",
		       s."Es_Alarma", s."Es_Indexada", s."Tipo_Valor",
		       tv."Nombre" AS tipo_variable, u."Simbolo" AS unidad,
		       e."Nombre_Topic"
		FROM ssfv."Tbl_Senales_x_Equipo" sxe
		JOIN ssfv."Tbl_Senales"       s   ON s."Senal_Id"    = sxe."Senal_Id"
		JOIN ssfv."Tbl_Tipo_Variable"  tv  ON tv."TipoVar_Id" = s."TipoVar_Id"
		JOIN ssfv."Tbl_Unidades"       u   ON u."Unidad_Id"   = s."Unidad_Id"
		JOIN ssfv."Tbl_Equipo"         e   ON e."Equipo_Id"   = sxe."Equipo_Id"
		WHERE sxe."Equipo_Id"=$1
		ORDER BY sxe."EquiSenal_Id"`, equipoID)
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
		INSERT INTO ssfv."Tbl_Equipo"
		    ("Planta_Id","Tipo_Id","Nombre_Equipo","Nombre_Topic","Fabricante","Modelo","Nro_Serie","Estado")
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING "Equipo_Id"`,
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

	if a := h.mgr.SSFVAdapter(); a != nil {
		a.InvalidateCache()
	}
	jsonResp(w, http.StatusCreated, map[string]any{"equipo_id": id})
}

// autoInstanciarSenales creates Tbl_Senales_x_Equipo rows for every signal
// in the tipo_equipo catalog, handling indexed signals by expanding channels.
func (h *SSFVHandler) autoInstanciarSenales(ctx context.Context, pool *pgxpool.Pool, equipoID, tipoID int) error {
	rows, err := pool.Query(ctx, `
		SELECT s."Senal_Id", s."Codigo_Senal", s."Es_Indexada", st."Num_Canales"
		FROM ssfv."Tbl_Senales_x_Tipo_Equipo" st
		JOIN ssfv."Tbl_Senales" s ON s."Senal_Id" = st."Senal_Id"
		WHERE st."Tipo_Id" = $1 AND s."Activo" = TRUE`, tipoID)
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
					INSERT INTO ssfv."Tbl_Senales_x_Equipo"
					    ("Senal_Id","Equipo_Id","Indice_Canal","Nombre_Instancia","Activo")
					VALUES ($1,$2,$3,$4,TRUE)
					ON CONFLICT DO NOTHING`,
					s.senalID, equipoID, i, instancia)
				if err != nil {
					log.Printf("ssfv: insert sxe %s idx %d: %v", instancia, i, err)
				}
			}
		} else {
			_, err := pool.Exec(ctx, `
				INSERT INTO ssfv."Tbl_Senales_x_Equipo"
				    ("Senal_Id","Equipo_Id","Nombre_Instancia","Activo")
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
		UPDATE ssfv."Tbl_Equipo"
		SET "Planta_Id"=$1,"Tipo_Id"=$2,"Nombre_Equipo"=$3,"Nombre_Topic"=$4,
		    "Fabricante"=$5,"Modelo"=$6,"Nro_Serie"=$7,"Estado"=$8,"Fecha_Modif"=NOW()
		WHERE "Equipo_Id"=$9`,
		body.PlantaID, body.TipoID, body.NombreEquipo, body.NombreTopic,
		body.Fabricante, body.Modelo, body.NroSerie, body.Estado, id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Invalidate cache since topic mapping may have changed.
	if a := h.mgr.SSFVAdapter(); a != nil {
		a.InvalidateCache()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SSFVHandler) deleteEquipo(w http.ResponseWriter, r *http.Request) {
	h.deleteRow(w, r, `DELETE FROM ssfv."Tbl_Equipo" WHERE "Equipo_Id"=$1`)
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
		SELECT s."Senal_Id",s."TipoVar_Id",s."Unidad_Id",s."Nombre",s."Descripcion",
		       s."Tipo_Valor",s."Codigo_Senal",s."Es_Indexada",s."Activo",s."Fecha_Alta",
		       tv."Nombre" AS tipo_var_nombre, u."Simbolo" AS unidad_simbolo
		FROM ssfv."Tbl_Senales" s
		JOIN ssfv."Tbl_Tipo_Variable" tv ON tv."TipoVar_Id" = s."TipoVar_Id"
		JOIN ssfv."Tbl_Unidades" u ON u."Unidad_Id" = s."Unidad_Id"
		ORDER BY s."Senal_Id"`)
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
		INSERT INTO ssfv."Tbl_Senales"
		    ("TipoVar_Id","Unidad_Id","Nombre","Descripcion","Tipo_Valor","Codigo_Senal","Es_Indexada","Activo")
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING "Senal_Id"`,
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
		UPDATE ssfv."Tbl_Senales"
		SET "TipoVar_Id"=$1,"Unidad_Id"=$2,"Nombre"=$3,"Descripcion"=$4,
		    "Tipo_Valor"=$5,"Codigo_Senal"=$6,"Es_Indexada"=$7,"Activo"=$8
		WHERE "Senal_Id"=$9`,
		body.TipoVarID, body.UnidadID, body.Nombre, body.Descripcion,
		body.TipoValor, body.CodigoSenal, body.EsIndexada, body.Activo, id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SSFVHandler) deleteSenal(w http.ResponseWriter, r *http.Request) {
	h.deleteRow(w, r, `DELETE FROM ssfv."Tbl_Senales" WHERE "Senal_Id"=$1`)
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
		SELECT sxe."EquiSenal_Id",sxe."Senal_Id",sxe."Equipo_Id",
		       sxe."Indice_Canal",sxe."Nombre_Instancia",sxe."Activo",sxe."Fecha_Alta",
		       s."Nombre" AS senal_nombre, s."Codigo_Senal",
		       e."Nombre_Equipo", e."Nombre_Topic"
		FROM ssfv."Tbl_Senales_x_Equipo" sxe
		JOIN ssfv."Tbl_Senales" s ON s."Senal_Id" = sxe."Senal_Id"
		JOIN ssfv."Tbl_Equipo" e ON e."Equipo_Id" = sxe."Equipo_Id"`
	args := []any{}
	if equipoID != "" {
		query += ` WHERE sxe."Equipo_Id"=$1`
		args = append(args, equipoID)
	}
	query += ` ORDER BY sxe."EquiSenal_Id"`

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
		INSERT INTO ssfv."Tbl_Senales_x_Equipo"
		    ("Senal_Id","Equipo_Id","Indice_Canal","Nombre_Instancia","Activo")
		VALUES ($1,$2,$3,$4,$5)
		RETURNING "EquiSenal_Id"`,
		body.SenalID, body.EquipoID, body.IndiceCanal,
		body.NombreInstancia, body.Activo,
	).Scan(&id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	if a := h.mgr.SSFVAdapter(); a != nil {
		a.InvalidateCache()
	}
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
		UPDATE ssfv."Tbl_Senales_x_Equipo"
		SET "Senal_Id"=$1,"Equipo_Id"=$2,"Indice_Canal"=$3,
		    "Nombre_Instancia"=$4,"Activo"=$5
		WHERE "EquiSenal_Id"=$6`,
		body.SenalID, body.EquipoID, body.IndiceCanal,
		body.NombreInstancia, body.Activo, id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	if a := h.mgr.SSFVAdapter(); a != nil {
		a.InvalidateCache()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SSFVHandler) deleteAsignacion(w http.ResponseWriter, r *http.Request) {
	if a := h.mgr.SSFVAdapter(); a != nil {
		a.InvalidateCache()
	}
	h.deleteRow(w, r, `DELETE FROM ssfv."Tbl_Senales_x_Equipo" WHERE "EquiSenal_Id"=$1`)
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
		SELECT f."Frontera_Id",f."Planta_Id",f."Codigo_NIE",f."Nombre",
		       f."Tipo_Conexion",f."Activo",f."Fecha_Creacion",
		       p."Nombre" AS planta_nombre
		FROM ssfv."Tbl_Frontera_Comercial" f
		JOIN ssfv."Tbl_Planta" p ON p."Planta_Id" = f."Planta_Id"
		ORDER BY f."Frontera_Id"`)
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
		INSERT INTO ssfv."Tbl_Frontera_Comercial"
		    ("Planta_Id","Codigo_NIE","Nombre","Tipo_Conexion","Activo")
		VALUES ($1,$2,$3,$4,$5)
		RETURNING "Frontera_Id"`,
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
		UPDATE ssfv."Tbl_Frontera_Comercial"
		SET "Planta_Id"=$1,"Codigo_NIE"=$2,"Nombre"=$3,
		    "Tipo_Conexion"=$4,"Activo"=$5,"Fecha_Modif"=NOW()
		WHERE "Frontera_Id"=$6`,
		body.PlantaID, body.CodigoNIE, body.Nombre, body.TipoConexion, body.Activo, id)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SSFVHandler) deleteFrontera(w http.ResponseWriter, r *http.Request) {
	h.deleteRow(w, r, `DELETE FROM ssfv."Tbl_Frontera_Comercial" WHERE "Frontera_Id"=$1`)
}

// ─── Catálogos de soporte ─────────────────────────────────────────────────────

func (h *SSFVHandler) listTipoEquipo(w http.ResponseWriter, r *http.Request) {
	h.queryJSON(w, r, `SELECT "Tipo_Id","Nombre","Descripcion","Activo" FROM ssfv."Tbl_Tipo_Equipo" ORDER BY "Tipo_Id"`)
}

func (h *SSFVHandler) listTipoVariable(w http.ResponseWriter, r *http.Request) {
	h.queryJSON(w, r, `SELECT "TipoVar_Id","Nombre","Descripcion","Activo" FROM ssfv."Tbl_Tipo_Variable" ORDER BY "TipoVar_Id"`)
}

func (h *SSFVHandler) listUnidades(w http.ResponseWriter, r *http.Request) {
	h.queryJSON(w, r, `SELECT "Unidad_Id","Simbolo","Nombre","Magnitud","Activo" FROM ssfv."Tbl_Unidades" ORDER BY "Unidad_Id"`)
}

// ─── Vistas ───────────────────────────────────────────────────────────────────

func (h *SSFVHandler) vistaSeñalesContexto(w http.ResponseWriter, r *http.Request) {
	h.queryJSON(w, r, `SELECT * FROM ssfv.v_Senales_Contexto ORDER BY "EquiSenal_Id"`)
}

func (h *SSFVHandler) vistaUltimasLecturas(w http.ResponseWriter, r *http.Request) {
	h.queryJSON(w, r, `SELECT * FROM ssfv.v_Ultimas_Lecturas ORDER BY "EquiSenal_Id"`)
}

func (h *SSFVHandler) vistaAlarmasActivas(w http.ResponseWriter, r *http.Request) {
	h.queryJSON(w, r, `SELECT * FROM ssfv.v_Alarmas_Activas`)
}

func (h *SSFVHandler) vistaRaw(w http.ResponseWriter, r *http.Request) {
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "100"
	}
	h.queryJSON(w, r, `SELECT ts,signal_path,signal,value,quality,tags FROM ssfv.signals_raw ORDER BY ts DESC LIMIT `+limit)
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
		SELECT a."Alarma_Id", a."EquiSenal_Id", a."Ts_Inicio", a."Ts_Fin",
		       a."Tipo_Alarma", a."Severidad", a."Descripcion", a."Activa",
		       e."Nombre_Equipo", e."Nombre_Topic",
		       p."Nombre" AS planta_nombre,
		       sxe."Nombre_Instancia"
		FROM ssfv."Tbl_Alarmas" a
		JOIN ssfv."Tbl_Senales_x_Equipo" sxe ON sxe."EquiSenal_Id" = a."EquiSenal_Id"
		JOIN ssfv."Tbl_Equipo"           e   ON e."Equipo_Id"       = sxe."Equipo_Id"
		JOIN ssfv."Tbl_Planta"           p   ON p."Planta_Id"       = e."Planta_Id"`
	args := []any{}
	if equipoID != "" {
		query += ` WHERE e."Equipo_Id"=$1`
		args = append(args, equipoID)
	} else {
		query += ` WHERE a."Activa" = TRUE`
	}
	query += ` ORDER BY a."Ts_Inicio" DESC LIMIT 200`

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

func (h *SSFVHandler) invalidateCache(w http.ResponseWriter, r *http.Request) {
	a := h.mgr.SSFVAdapter()
	if a == nil {
		errResp(w, http.StatusServiceUnavailable, "ssfv adapter not connected")
		return
	}
	a.InvalidateCache()
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
