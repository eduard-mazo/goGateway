package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"goGateway/internal/tsdb"
)

// EquiSenalMapping holds resolved metadata for one SSFV signal instance.
type EquiSenalMapping struct {
	EquisenalID int64
	NombreTopic string
	GroupID     string // = planta.broker_base (the Sparkplug group; contract §1.1)
	NodeID      string // first segment of nombre_topic after the group prefix
	DeviceID    string // last segment of nombre_topic
	EsAlarma    bool
}

// SSFVMappingCache resolves (topic, nombre_instancia) → EquiSenalMapping.
// Loaded eagerly from ssfv schema; call Reload() after catalog changes.
type SSFVMappingCache struct {
	mu     sync.RWMutex
	data   map[string]EquiSenalMapping // key: topic+"\x00"+nombre_instancia
	topics map[string]struct{}          // set of known MQTT topics
}

func NewSSFVMappingCache() *SSFVMappingCache {
	return &SSFVMappingCache{
		data:   make(map[string]EquiSenalMapping),
		topics: make(map[string]struct{}),
	}
}

// Reload loads all active signal-equipo mappings from the ssfv schema.
// Safe to call repeatedly; replaces the map atomically.
//
// Two passes:
//  1. Signal routing map  — equipo+señal rows used for value dispatch.
//  2. Known-topics set    — ALL active equipos, including those with no
//     signals yet. Equipos absent from the set are silently ignored by
//     IsTopic(); including them ensures their metrics appear in "Descartados"
//     instead of being dropped.
func (c *SSFVMappingCache) Reload(pool *pgxpool.Pool) error {
	if pool == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Pass 1: signal routing entries.
	rows, err := pool.Query(ctx, `
		SELECT
		    sxe.equisenal_id,
		    s.codigo_senal,
		    sxe.nombre_instancia,
		    e.nombre_topic,
		    -- Contract §1.1: group = planta.broker_base; node = the first segment of
		    -- nombre_topic after the group prefix; device = the last segment.
		    p.broker_base AS group_id,
		    split_part(substring(e.nombre_topic FROM char_length(p.broker_base) + 2), '/', 1) AS node_id,
		    reverse(split_part(reverse(e.nombre_topic), '/', 1)) AS device_id,
		    (s.codigo_senal LIKE 'AL%' OR s.codigo_senal LIKE 'EF%' OR s.codigo_senal LIKE 'EV%') AS es_alarma
		FROM ssfv.tbl_senales_x_equipo sxe
		JOIN ssfv.tbl_senales           s   ON s.senal_id   = sxe.senal_id
		JOIN ssfv.tbl_equipo            e   ON e.equipo_id  = sxe.equipo_id
		JOIN ssfv.tbl_planta            p   ON p.planta_id  = e.planta_id
		WHERE sxe.activo = TRUE
		  AND e.estado   = 1
		  AND p.estado   = 1
		  AND e.fecha_baja IS NULL
		  AND p.fecha_baja IS NULL`)
	if err != nil {
		return fmt.Errorf("ssfv mapping reload: %w", err)
	}
	defer rows.Close()

	newData := make(map[string]EquiSenalMapping)
	newTopics := make(map[string]struct{})

	for rows.Next() {
		var m EquiSenalMapping
		var codigoSenal, nombreInstancia string
		if err := rows.Scan(&m.EquisenalID, &codigoSenal, &nombreInstancia, &m.NombreTopic,
			&m.GroupID, &m.NodeID, &m.DeviceID, &m.EsAlarma); err != nil {
			log.Printf("ssfv mapping scan: %v", err)
			continue
		}
		// C1 composite key: entity \x00 codigo_senal \x00 nombre_instancia.
		key := m.NombreTopic + "\x00" + codigoSenal + "\x00" + nombreInstancia
		newData[key] = m
		newTopics[m.NombreTopic] = struct{}{}
	}
	rows.Close()

	// Pass 2: add every active equipo to the known-topics set even when it has
	// no signal instances (e.g. just approved from autodiscovery, or its
	// tipo_equipo has an empty junction table). This guarantees IsTopic()
	// returns true for all configured equipment so misses are visible.
	equipoRows, err := pool.Query(ctx, `
		SELECT e.nombre_topic
		FROM ssfv.tbl_equipo e
		JOIN ssfv.tbl_planta p ON p.planta_id = e.planta_id
		WHERE e.estado = 1 AND p.estado = 1
		  AND e.fecha_baja IS NULL AND p.fecha_baja IS NULL`)
	if err != nil {
		log.Printf("ssfv mapping reload equipos: %v", err)
	} else {
		defer equipoRows.Close()
		for equipoRows.Next() {
			var t string
			if err := equipoRows.Scan(&t); err == nil {
				newTopics[t] = struct{}{}
			}
		}
	}

	c.mu.Lock()
	c.data = newData
	c.topics = newTopics
	c.mu.Unlock()

	log.Printf("ssfv: mapping cache loaded (%d signal entries, %d equipos known)",
		len(newData), len(newTopics))
	return nil
}

// Lookup returns the mapping for the C1 composite identity
// (entity, codigo_senal, nombre_instancia).
func (c *SSFVMappingCache) Lookup(entity, codigo, instance string) (EquiSenalMapping, bool) {
	if instance == "" {
		instance = "default"
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.data[entity+"\x00"+codigo+"\x00"+instance]
	return m, ok
}

// IsTopic returns true if the topic matches a known SSFV equipment topic.
func (c *SSFVMappingCache) IsTopic(topic string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.topics[topic]
	return ok
}

// TopicCount returns the number of known equipment topics (for status).
func (c *SSFVMappingCache) TopicCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.topics)
}

// SSFVHandler processes JSON-MQTT messages from SSFV equipment and routes them
// to the TSDB pipeline and alarm manager.
type SSFVHandler struct {
	cache *SSFVMappingCache

	mu       sync.RWMutex
	pipe     *tsdb.WritePipeline             // guarded by mu; nil until TSDB connects
	alarmMgr *AlarmManager                   // guarded by mu; nil until TSDB connects
	missFn   func(signalPath, equipo string) // guarded by mu; optional miss callback
}

func NewSSFVHandler(cache *SSFVMappingCache, pipe *tsdb.WritePipeline, alarms *AlarmManager) *SSFVHandler {
	h := &SSFVHandler{cache: cache}
	h.pipe = pipe
	h.alarmMgr = alarms
	return h
}

// SetPipeline swaps the pipeline reference. Safe to call from any goroutine,
// including concurrently with MQTT message dispatch.
func (h *SSFVHandler) SetPipeline(p *tsdb.WritePipeline) {
	h.mu.Lock()
	h.pipe = p
	h.mu.Unlock()
}

// SetAlarmManager swaps the alarm manager reference. Safe to call from any goroutine.
func (h *SSFVHandler) SetAlarmManager(a *AlarmManager) {
	h.mu.Lock()
	h.alarmMgr = a
	h.mu.Unlock()
}

// SetMissFn registers a callback invoked when a metric's topic is a known SSFV
// equipment but the signal code has no catalog entry. Called from HandleMetric.
// Wire this to SSFVAdapter.RecordMiss after the adapter is created.
func (h *SSFVHandler) SetMissFn(fn func(signalPath, equipo string)) {
	h.mu.Lock()
	h.missFn = fn
	h.mu.Unlock()
}

// hostCategories are the top-level groups of edge-node host telemetry
// (sparkplug-contract.md §4). String identity metrics (System/Device/*) are
// intentionally excluded — they are not numeric samples.
var hostCategories = map[string]bool{
	"CPU": true, "Memory": true, "Disk": true, "Network": true,
	"Temperature": true, "Power": true, "Process": true,
}

// isHostMetric reports whether a Sparkplug metric name is gateway host
// telemetry rather than a plant process signal. Accepts an optional "System/"
// prefix (the producer's configurable metricPrefix).
func isHostMetric(name string) bool {
	n := strings.TrimPrefix(name, "System/")
	if n == "Uptime_h" {
		return true
	}
	first := n
	if i := strings.IndexByte(n, '/'); i >= 0 {
		first = n[:i]
	}
	return hostCategories[first]
}

// IsKnownTopic reports whether topic matches a configured SSFV equipment topic.
func (h *SSFVHandler) IsKnownTopic(topic string) bool {
	return h.cache.IsTopic(topic)
}

// HandleMetric processes a single decoded metric value against the SSFV catalog
// using the C1 composite identity (entity, codigo, instance). Writes to
// ssfv.tbl_valores on a catalog hit; an unregistered signal on a known entity is
// quarantined to Descartados. Returns true on a catalog hit.
func (h *SSFVHandler) HandleMetric(entity, codigo, instance string, value float64, ts time.Time) bool {
	if instance == "" {
		instance = "default"
	}
	mapping, ok := h.cache.Lookup(entity, codigo, instance)
	if !ok {
		// Entity exists but this signal/instance is unregistered → actionable miss.
		if h.cache.IsTopic(entity) {
			h.mu.RLock()
			fn := h.missFn
			h.mu.RUnlock()
			if fn != nil {
				fn(entity+"/"+codigo+"@"+instance, entity)
			}
		}
		return false
	}

	h.mu.RLock()
	pipe := h.pipe
	alarmMgr := h.alarmMgr
	h.mu.RUnlock()

	if mapping.EsAlarma && alarmMgr != nil {
		alarmMgr.Process(mapping.EquisenalID, value, ts, alarmType(codigo))
	}
	if pipe != nil {
		pipe.Push(tsdb.DataPoint{ //nolint:errcheck
			Measurement: codigo,
			Tags: map[string]string{
				"equipo":    entity,
				"codigo":    codigo,
				"instancia": instance,
			},
			Fields:    map[string]float64{"value": value},
			Timestamp: ts,
		})
	}
	return true
}

// Handle processes one MQTT message. Returns true if handled as SSFV topic.
func (h *SSFVHandler) Handle(topic string, payload []byte) bool {
	if !h.cache.IsTopic(topic) {
		return false
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		log.Printf("ssfv: parse %s: %v", topic, err)
		return true // topic matched → claim the message even on parse error
	}

	// Snapshot pipeline + alarm manager once under a single read lock.
	h.mu.RLock()
	pipe := h.pipe
	alarmMgr := h.alarmMgr
	h.mu.RUnlock()

	// Extract timestamp from "date" field; fall back to now.
	ts := time.Now().UTC()
	if dateRaw, ok := raw["date"]; ok {
		var dateStr string
		if err := json.Unmarshal(dateRaw, &dateStr); err == nil {
			if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
				ts = t.UTC()
			}
		}
	}

	for key, valRaw := range raw {
		if key == "date" {
			continue
		}
		var value float64
		if err := json.Unmarshal(valRaw, &value); err != nil {
			continue
		}

		// C1: map the JSON key to (codigo, instance). A flat key is its own code
		// with the default instance; an indexed key ("IDC_1") splits into its base
		// code ("IDC_x") + the channel as the instance.
		codigo, instance := jsonSignalParts(key)
		mapping, ok := h.cache.Lookup(topic, codigo, instance)
		if !ok {
			continue // not in cache for this equipo/instance
		}

		if mapping.EsAlarma && alarmMgr != nil {
			alarmMgr.Process(mapping.EquisenalID, value, ts, alarmType(key))
		}

		if pipe != nil {
			pipe.Push(tsdb.DataPoint{ //nolint:errcheck
				Measurement: codigo,
				Tags: map[string]string{
					"equipo":    topic,
					"codigo":    codigo,
					"instancia": instance,
				},
				Fields:    map[string]float64{"value": value},
				Timestamp: ts,
			})
		}
	}
	return true
}

// jsonSignalParts maps a JSON field key to the C1 composite (codigo, instance).
// An indexed key ("IDC_1") → base code "IDC_x" + channel instance "IDC_1";
// a flat key ("AP") → code "AP" + instance "default".
func jsonSignalParts(jsonKey string) (codigo, instance string) {
	base, idx := resolveIndexedSignal(jsonKey)
	if idx > 0 {
		return base, jsonKey
	}
	return jsonKey, "default"
}

// resolveIndexedSignal converts "IDC_1" → ("IDC_x", 1), non-indexed → (key, 0).
func resolveIndexedSignal(jsonKey string) (baseCode string, index int) {
	parts := strings.Split(jsonKey, "_")
	if len(parts) >= 2 {
		if idx, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			base := strings.Join(parts[:len(parts)-1], "_") + "_x"
			return base, idx
		}
	}
	return jsonKey, 0
}

// alarmType classifies an alarm signal code into a Tbl_Alarmas tipo_alarma value.
func alarmType(code string) string {
	if code == "AL_COM" {
		return "Comunicacion"
	}
	if strings.HasPrefix(code, "AL_") {
		return "Dispositivo"
	}
	if strings.HasPrefix(code, "EF_") {
		return "Dispositivo"
	}
	if strings.HasPrefix(code, "EV_") {
		return "Fabricante"
	}
	return "Dispositivo"
}
