package worker

import (
	"sync"

	"github.com/jmoiron/sqlx"

	"goGateway/internal/models"
	"goGateway/internal/sparkplug"
)

// TopicMapping = resolved row for dispatch (joins topic + mapping).
type TopicMapping struct {
	MappingID  int64
	ServerID   int64 // IEC-104 slave that receives this point.
	TopicID    int64
	Topic      string
	JSONKey    string
	QualityKey string // optional: JSON key for per-signal quality; overrides payload-level "quality"
	MetricName string // Sparkplug B metric name; used when SparkplugEnabled=true
	IEC104Type string
	IOA        int
	Scale      float64
	Business   string
	Company    string
	// SignalPath = full signal identity: business/company/B1/.../signal.
	// Pre-computed for JSON mode; set at dispatch time for Sparkplug mode.
	SignalPath string
}

// MappingCache = topic → []TopicMapping. Rebuilt on notifier fire.
// Two secondary indexes support Sparkplug B mode:
//   - sp:   (nodeBase + NUL + metricName) → []TopicMapping  — per-metric lookup
//   - spAll: nodeBase → []TopicMapping                       — full-node lookup (NDEATH stale)
type MappingCache struct {
	db    *sqlx.DB
	mu    sync.RWMutex
	m     map[string][]TopicMapping // JSON mode: exact topic → mappings
	qos   map[string]byte           // JSON mode: topic → QoS (from topics.qos)
	sp    map[string][]TopicMapping // Sparkplug B: nodeBase\x00metricName → mappings
	spAll map[string][]TopicMapping // Sparkplug B: nodeBase → all mappings for that node
}

func NewMappingCache(db *sqlx.DB) *MappingCache {
	return &MappingCache{
		db:    db,
		m:     map[string][]TopicMapping{},
		qos:   map[string]byte{},
		sp:    map[string][]TopicMapping{},
		spAll: map[string][]TopicMapping{},
	}
}

func (c *MappingCache) Reload() error {
	rows, err := c.db.Queryx(`
        SELECT sm.id, sm.server_id, sm.topic_id, t.topic, t.qos, sm.json_key,
               sm.quality_key, sm.metric_name, sm.iec104_type, sm.ioa, sm.scale,
               sm.device_name, sm.business, sm.company
          FROM signal_mappings sm
          JOIN topics t ON t.id = sm.topic_id
          JOIN iec104_servers s ON s.id = sm.server_id
         WHERE sm.enabled = 1 AND t.enabled = 1 AND s.enabled = 1`)
	if err != nil {
		return err
	}
	defer rows.Close()

	next := map[string][]TopicMapping{}
	nextQoS := map[string]byte{}
	sp := map[string][]TopicMapping{}
	spAll := map[string][]TopicMapping{}

	for rows.Next() {
		var tm TopicMapping
		var deviceName string
		var topicQoS int
		if err := rows.Scan(
			&tm.MappingID, &tm.ServerID, &tm.TopicID, &tm.Topic, &topicQoS,
			&tm.JSONKey, &tm.QualityKey, &tm.MetricName,
			&tm.IEC104Type, &tm.IOA, &tm.Scale,
			&deviceName, &tm.Business, &tm.Company,
		); err != nil {
			return err
		}
		if tm.Scale == 0 {
			tm.Scale = 1.0
		}

		if tm.MetricName != "" {
			// Sparkplug B: index by nodeBase (without message-type segment), not the
			// raw subscription topic. LookupByMetric receives topic.NodeBase() which
			// strips the msg-type component (NBIRTH/NDATA/…) from the live message.
			base := spNodeBase(tm.Topic)
			key := base + "\x00" + tm.MetricName
			sp[key] = append(sp[key], tm)
			spAll[base] = append(spAll[base], tm)
		} else {
			// JSON mode: path = business/company/topic/json_key — fully deterministic.
			tm.SignalPath = tm.Business + "/" + tm.Company + "/" + tm.Topic + "/" + tm.JSONKey
			next[tm.Topic] = append(next[tm.Topic], tm)
			if q := byte(topicQoS); q > nextQoS[tm.Topic] {
				nextQoS[tm.Topic] = q
			}
		}
	}

	c.mu.Lock()
	c.m = next
	c.qos = nextQoS
	c.sp = sp
	c.spAll = spAll
	c.mu.Unlock()
	return nil
}

// Lookup returns mappings for an exact JSON-mode topic (nil if none).
func (c *MappingCache) Lookup(topic string) []TopicMapping {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.m[topic]
}

// LookupByMetric returns all mappings for a Sparkplug B metric.
// nodeBase = "spBv1.0/{group}/{node}" or "spBv1.0/{group}/{node}/{device}".
// metricName = the Sparkplug metric name (e.g., "outputs/power").
func (c *MappingCache) LookupByMetric(nodeBase, metricName string) []TopicMapping {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sp[nodeBase+"\x00"+metricName]
}

// LookupByNode returns all Sparkplug B mappings for a node (used on NDEATH to
// mark every associated IEC-104 point as stale).
func (c *MappingCache) LookupByNode(nodeBase string) []TopicMapping {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.spAll[nodeBase]
}

// TopicsQoS returns topic → QoS for all active JSON-mode topics.
// The QoS is the highest value configured across all mappings for that topic.
func (c *MappingCache) TopicsQoS() map[string]byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]byte, len(c.qos))
	for t, q := range c.qos {
		out[t] = q
	}
	return out
}

// MQTTConfigSnapshot holds the subset of MQTTConfig that the mqtt.Manager
// needs in callback goroutines without hitting the DB again.
type MQTTConfigSnapshot struct {
	SpGroupID string
	SpHostID  string
	SpTopics  string // newline/comma-separated extra MQTT topic patterns
}

// LoadMQTTConfig = helper to read singleton config (all columns).
func LoadMQTTConfig(db *sqlx.DB) (models.MQTTConfig, error) {
	var c models.MQTTConfig
	err := db.Get(&c, `SELECT id,host,port,username,password,client_id,use_tls,
	                          sparkplug_enabled,sp_group_id,sp_host_id,sp_topics
	                     FROM mqtt_config WHERE id=1`)
	return c, err
}

// LoadIEC104Servers = active + disabled rows, ordered by id.
func LoadIEC104Servers(db *sqlx.DB) ([]models.IEC104Server, error) {
	var out []models.IEC104Server
	err := db.Select(&out, `SELECT id,name,port,asdu_addr,scada_ips,k,w,t0,t1,t2,t3,enabled FROM iec104_servers ORDER BY id`)
	return out, err
}

// spNodeBase derives the Sparkplug B nodeBase from a raw subscription topic.
// Raw topics in the DB can be stored as either the nodeBase itself
// ("spBv1.0/group/node") or a typed topic ("spBv1.0/group/NDATA/node").
// LookupByMetric and LookupByNode always receive topic.NodeBase() from live
// messages, so we normalise here to avoid a systematic miss.
func spNodeBase(raw string) string {
	t, ok := sparkplug.ParseTopic(raw)
	if !ok {
		return raw // already a nodeBase or unknown format — use as-is
	}
	if t.DeviceID != "" {
		return t.DeviceBase()
	}
	return t.NodeBase()
}

// LoadIEC104Gateway = singleton row. Returns the seeded default on a fresh DB.
func LoadIEC104Gateway(db *sqlx.DB) (models.IEC104Gateway, error) {
	var g models.IEC104Gateway
	err := db.Get(&g, `SELECT id, listen_ip FROM iec104_gateway WHERE id=1`)
	return g, err
}
