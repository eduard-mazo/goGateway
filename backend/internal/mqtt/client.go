// Package mqtt = paho.mqtt.golang wrapper with reload-on-notify.
package mqtt

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/iec104"
	"goGateway/internal/worker"
)

// Manager = long-lived controller. Holds current paho client + re-connects
// on config/topic changes fired via Notify().
type Manager struct {
	db    *sqlx.DB
	cache *worker.MappingCache
	iec   iec104.Server
	hist  *worker.HistoryLogger

	mu     sync.Mutex
	client paho.Client
	subs   map[string]struct{} // currently subscribed topics
	broker string              // last built broker URL

	messages atomic.Int64
	lastMsg  atomic.Int64 // unix nano
}

// Status = live broker status snapshot.
type Status struct {
	Connected bool   `json:"connected"`
	Broker    string `json:"broker"`
	Topics    int    `json:"topics"`
	Messages  int64  `json:"messages"`
	LastMsgAt int64  `json:"last_msg_at"` // unix nano, 0 if none
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	connected := m.client != nil && m.client.IsConnected()
	return Status{
		Connected: connected,
		Broker:    m.broker,
		Topics:    len(m.subs),
		Messages:  m.messages.Load(),
		LastMsgAt: m.lastMsg.Load(),
	}
}

func NewManager(db *sqlx.DB, cache *worker.MappingCache, iec iec104.Server, hist *worker.HistoryLogger) *Manager {
	return &Manager{db: db, cache: cache, iec: iec, hist: hist, subs: map[string]struct{}{}}
}

// Start = initial connect + subscribe. Safe to call before config exists.
func (m *Manager) Start(ctx context.Context) error {
	return m.reload()
}

// Notify = trigger config/subs reload. Non-blocking.
func (m *Manager) Notify() {
	go func() {
		if err := m.reload(); err != nil {
			log.Printf("mqtt reload: %v", err)
		}
	}()
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client != nil && m.client.IsConnected() {
		m.client.Disconnect(500)
	}
}

func (m *Manager) reload() error {
	cfg, err := worker.LoadMQTTConfig(m.db)
	if err != nil {
		return fmt.Errorf("load mqtt cfg: %w", err)
	}
	if err := m.cache.Reload(); err != nil {
		return fmt.Errorf("reload mappings: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// tear down prior client (simpler than diffing subs + broker change).
	if m.client != nil && m.client.IsConnected() {
		m.client.Disconnect(200)
	}

	scheme := "tcp"
	if cfg.UseTLS {
		scheme = "ssl"
	}
	brokerURL := fmt.Sprintf("%s://%s:%d", scheme, cfg.Host, cfg.Port)
	m.broker = brokerURL
	opts := paho.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(cfg.ClientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetKeepAlive(30 * time.Second).
		SetOnConnectHandler(m.onConnect).
		SetConnectionLostHandler(func(_ paho.Client, err error) {
			log.Printf("mqtt connection lost: %v", err)
		})
	if cfg.Username != "" {
		opts.SetUsername(cfg.Username).SetPassword(cfg.Password)
	}

	c := paho.NewClient(opts)
	m.client = c
	m.subs = map[string]struct{}{}

	tok := c.Connect()
	// don't block: paho retries. Log outcome async.
	go func() {
		tok.Wait()
		if err := tok.Error(); err != nil {
			log.Printf("mqtt connect: %v", err)
		}
	}()
	return nil
}

// onConnect = (re)subscribe to all topics in cache.
func (m *Manager) onConnect(c paho.Client) {
	topics := m.cache.Topics()
	log.Printf("mqtt connected, subscribing %d topic(s)", len(topics))
	for _, t := range topics {
		topic := t
		tok := c.Subscribe(topic, 0, func(_ paho.Client, msg paho.Message) {
			m.onMessage(msg.Topic(), msg.Payload())
		})
		go func() {
			tok.Wait()
			if err := tok.Error(); err != nil {
				log.Printf("mqtt subscribe %s: %v", topic, err)
			}
		}()
		m.mu.Lock()
		m.subs[topic] = struct{}{}
		m.mu.Unlock()
	}
}

func (m *Manager) onMessage(topic string, payload []byte) {
	m.messages.Add(1)
	m.lastMsg.Store(time.Now().UnixNano())
	maps := m.cache.Lookup(topic)
	worker.ParseAndDispatch(topic, payload, maps, m.iec, m.hist)
}
