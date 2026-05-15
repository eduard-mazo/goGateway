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
	GroupID     string // split_part(broker_base, '/', 2)
	NodeID      string // split_part(broker_base, '/', 4)
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
func (c *SSFVMappingCache) Reload(pool *pgxpool.Pool) error {
	if pool == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
		SELECT
		    sxe.equisenal_id,
		    sxe.nombre_instancia,
		    e.nombre_topic,
		    split_part(p.broker_base, '/', 2) AS group_id,
		    split_part(p.broker_base, '/', 4) AS node_id,
		    reverse(split_part(reverse(e.nombre_topic), '/', 1)) AS device_id,
		    (s.codigo_senal LIKE 'AL%' OR s.codigo_senal LIKE 'EF%' OR s.codigo_senal LIKE 'EV%') AS es_alarma
		FROM ssfv.tbl_senales_x_equipo sxe
		JOIN ssfv.tbl_senales           s   ON s.senal_id   = sxe.senal_id
		JOIN ssfv.tbl_equipo            e   ON e.equipo_id  = sxe.equipo_id
		JOIN ssfv.tbl_planta            p   ON p.planta_id  = e.planta_id
		WHERE sxe.activo = TRUE
		  AND e.estado   = 1
		  AND p.estado   = 1`)
	if err != nil {
		return fmt.Errorf("ssfv mapping reload: %w", err)
	}
	defer rows.Close()

	newData := make(map[string]EquiSenalMapping)
	newTopics := make(map[string]struct{})

	for rows.Next() {
		var m EquiSenalMapping
		var nombreInstancia string
		if err := rows.Scan(&m.EquisenalID, &nombreInstancia, &m.NombreTopic,
			&m.GroupID, &m.NodeID, &m.DeviceID, &m.EsAlarma); err != nil {
			log.Printf("ssfv mapping scan: %v", err)
			continue
		}
		key := m.NombreTopic + "\x00" + nombreInstancia
		newData[key] = m
		newTopics[m.NombreTopic] = struct{}{}
	}

	c.mu.Lock()
	c.data = newData
	c.topics = newTopics
	c.mu.Unlock()

	log.Printf("ssfv: mapping cache loaded (%d entries, %d topics)", len(newData), len(newTopics))
	return nil
}

// Lookup returns the mapping for a (topic, jsonKey) pair.
func (c *SSFVMappingCache) Lookup(topic, jsonKey string) (EquiSenalMapping, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.data[topic+"\x00"+jsonKey]
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
	cache    *SSFVMappingCache
	pipe     *tsdb.WritePipeline // may be nil until TSDB connects
	alarmMgr *AlarmManager       // may be nil until TSDB connects
}

func NewSSFVHandler(cache *SSFVMappingCache, pipe *tsdb.WritePipeline, alarms *AlarmManager) *SSFVHandler {
	return &SSFVHandler{cache: cache, pipe: pipe, alarmMgr: alarms}
}

// SetPipeline updates the pipeline reference after a TSDB reload.
func (h *SSFVHandler) SetPipeline(p *tsdb.WritePipeline) { h.pipe = p }

// SetAlarmManager updates the alarm manager reference.
func (h *SSFVHandler) SetAlarmManager(a *AlarmManager) { h.alarmMgr = a }

// HandleMetric processes a single decoded metric from a Sparkplug B NDATA message.
// topic = MQTT device topic (MetricName minus last segment), code = signal code.
// Returns true if the signal is known to the SSFV catalog.
func (h *SSFVHandler) HandleMetric(topic, code string, value float64, ts time.Time) bool {
	mapping, ok := h.cache.Lookup(topic, code)
	if !ok {
		return false
	}
	if mapping.EsAlarma && h.alarmMgr != nil {
		h.alarmMgr.Process(mapping.EquisenalID, value, ts, alarmType(code))
	}
	if h.pipe != nil {
		h.pipe.Push(tsdb.DataPoint{ //nolint:errcheck
			Measurement: code,
			Tags: map[string]string{
				"signal_path": topic + "/" + code,
				"equipo":      topic,
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

		// Try exact match first (e.g. "AP", "AL_COM"), then indexed base code.
		mapping, ok := h.cache.Lookup(topic, key)
		if !ok {
			baseCode, _ := resolveIndexedSignal(key)
			if baseCode != key {
				continue // indexed signal not in cache (no mapping for this instance)
			}
			continue
		}

		if mapping.EsAlarma && h.alarmMgr != nil {
			h.alarmMgr.Process(mapping.EquisenalID, value, ts, alarmType(key))
		}

		if h.pipe != nil {
			h.pipe.Push(tsdb.DataPoint{ //nolint:errcheck
				Measurement: key,
				Tags: map[string]string{
					"signal_path": topic + "/" + key,
					"equipo":      topic,
				},
				Fields:    map[string]float64{"value": value},
				Timestamp: ts,
			})
		}
	}
	return true
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
