package worker

import (
	"sync"

	"github.com/jmoiron/sqlx"

	"goGateway/internal/models"
)

// TopicMapping = resolved row for dispatch (joins topic + mapping).
type TopicMapping struct {
	MappingID  int64
	ServerID   int64 // IEC-104 slave that receives this point.
	TopicID    int64
	Topic      string
	JSONKey    string
	IEC104Type string
	IOA        int
	Scale      float64
	SignalKey  string // = device_name + "." + json_key (for history label)
}

// MappingCache = topic → []TopicMapping. Rebuilt on notifier fire.
type MappingCache struct {
	db *sqlx.DB
	mu sync.RWMutex
	m  map[string][]TopicMapping
}

func NewMappingCache(db *sqlx.DB) *MappingCache {
	return &MappingCache{db: db, m: map[string][]TopicMapping{}}
}

func (c *MappingCache) Reload() error {
	rows, err := c.db.Queryx(`
        SELECT sm.id, sm.server_id, sm.topic_id, t.topic, sm.json_key,
               sm.iec104_type, sm.ioa, sm.scale, sm.device_name
          FROM signal_mappings sm
          JOIN topics t ON t.id = sm.topic_id
          JOIN iec104_servers s ON s.id = sm.server_id
         WHERE sm.enabled = 1 AND t.enabled = 1 AND s.enabled = 1`)
	if err != nil {
		return err
	}
	defer rows.Close()

	next := map[string][]TopicMapping{}
	for rows.Next() {
		var tm TopicMapping
		var deviceName string
		if err := rows.Scan(&tm.MappingID, &tm.ServerID, &tm.TopicID, &tm.Topic,
			&tm.JSONKey, &tm.IEC104Type, &tm.IOA, &tm.Scale, &deviceName); err != nil {
			return err
		}
		if tm.Scale == 0 {
			tm.Scale = 1.0
		}
		tm.SignalKey = deviceName + "." + tm.JSONKey
		next[tm.Topic] = append(next[tm.Topic], tm)
	}
	c.mu.Lock()
	c.m = next
	c.mu.Unlock()
	return nil
}

// Lookup = mappings for topic (nil if none).
func (c *MappingCache) Lookup(topic string) []TopicMapping {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.m[topic]
}

// Topics = distinct subscribed topics.
func (c *MappingCache) Topics() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.m))
	for t := range c.m {
		out = append(out, t)
	}
	return out
}

// LoadMQTTConfig = helper to read singleton config.
func LoadMQTTConfig(db *sqlx.DB) (models.MQTTConfig, error) {
	var c models.MQTTConfig
	err := db.Get(&c, `SELECT id,host,port,username,password,client_id,use_tls FROM mqtt_config WHERE id=1`)
	return c, err
}

// LoadIEC104Servers = active + disabled rows, ordered by id.
func LoadIEC104Servers(db *sqlx.DB) ([]models.IEC104Server, error) {
	var out []models.IEC104Server
	err := db.Select(&out, `SELECT id,name,port,asdu_addr,scada_ips,k,w,t0,t1,t2,t3,enabled FROM iec104_servers ORDER BY id`)
	return out, err
}

// LoadIEC104Gateway = singleton row. Returns the seeded default on a fresh DB.
func LoadIEC104Gateway(db *sqlx.DB) (models.IEC104Gateway, error) {
	var g models.IEC104Gateway
	err := db.Get(&g, `SELECT id, listen_ip FROM iec104_gateway WHERE id=1`)
	return g, err
}
