// Package mqtt = paho.mqtt.golang wrapper with reload-on-notify.
package mqtt

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/sparkplug"
	"goGateway/internal/worker"
)

// Manager = long-lived controller. Holds current paho client + re-connects
// on config/topic changes fired via Notify().
type Manager struct {
	db    *sqlx.DB
	cache *worker.MappingCache
	d     worker.Dispatcher

	mu        sync.Mutex
	client    paho.Client
	subs      map[string]struct{} // currently subscribed topics
	broker    string              // last built broker URL
	connected atomic.Bool         // set by onConnect / connectionLost; faster than IsConnected()

	messages atomic.Int64
	lastMsg  atomic.Int64 // unix nano

	// Sparkplug B state — non-nil only when SparkplugEnabled.
	registry  *sparkplug.Registry
	spHandler *worker.SparkplugHandler
	bdSeq     atomic.Uint64 // birth/death sequence; increments on every MQTT CONNECT
	cfg       worker.MQTTConfigSnapshot

	// SSFV JSON handler — intercepts equipment topics before generic dispatch.
	ssfvHandler *worker.SSFVHandler

	// autoDisc records NBIRTH/DBIRTH events in SQLite for operator review.
	autoDisc *worker.AutoDiscoveryService

	// monitorHook receives (topic, kind, payload, ssfvHits) for every inbound message.
	// ssfvHits is the count of metrics forwarded to SSFV (sparkplug messages only).
	// nil = disabled. Set via SetMonitorHook before Start().
	monitorHook func(topic, kind string, payload []byte, ssfvHits int)
}

// MQTTConfigSnapshot is a copy of the config values the manager needs outside
// the DB lock (e.g., in callback goroutines).
// It mirrors worker.LoadMQTTConfig but avoids a DB round-trip in hot paths.

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
	return Status{
		Connected: m.connected.Load(),
		Broker:    m.broker,
		Topics:    len(m.subs),
		Messages:  m.messages.Load(),
		LastMsgAt: m.lastMsg.Load(),
	}
}

func NewManager(db *sqlx.DB, cache *worker.MappingCache, d worker.Dispatcher) *Manager {
	return &Manager{db: db, cache: cache, d: d, subs: map[string]struct{}{}}
}

// SetSSFVHandler registers the SSFV JSON handler.
// Must be called before Start() or Notify() to take effect on next (re)connect.
func (m *Manager) SetSSFVHandler(h *worker.SSFVHandler) {
	m.mu.Lock()
	m.ssfvHandler = h
	m.mu.Unlock()
}

// SetAutoDiscovery registers the auto-discovery service so that NBIRTH/DBIRTH
// events are recorded in SQLite. Must be called before Start().
func (m *Manager) SetAutoDiscovery(a *worker.AutoDiscoveryService) {
	m.mu.Lock()
	m.autoDisc = a
	m.mu.Unlock()
}

// SetMonitorHook registers a callback invoked for every inbound MQTT message.
// fn receives (topic, kind, payload, ssfvHits). kind is one of "sparkplug",
// "ssfv", "json", or "state". ssfvHits is the number of metrics forwarded to
// the SSFV pipeline (non-zero only for sparkplug messages). Safe to call at any time.
func (m *Manager) SetMonitorHook(fn func(topic, kind string, payload []byte, ssfvHits int)) {
	m.mu.Lock()
	m.monitorHook = fn
	m.mu.Unlock()
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

	// Build Sparkplug B handler when enabled.
	if cfg.SparkplugEnabled {
		if m.registry == nil {
			m.registry = sparkplug.NewRegistry()
		}
		m.spHandler = worker.NewSparkplugHandler(m.registry, m.cache, m.d)
		m.spHandler.SetRebirthFn(m.publishRebirth)
		if m.ssfvHandler != nil {
			m.spHandler.SetSSFVHandler(m.ssfvHandler)
		}
		if m.autoDisc != nil {
			m.spHandler.SetAutoDiscovery(m.autoDisc)
			m.autoDisc.SetRebirthFn(m.publishRebirth)
		}
		m.cfg = worker.MQTTConfigSnapshot{
			SpGroupID: cfg.SpGroupID,
			SpHostID:  cfg.SpHostID,
			SpTopics:  cfg.SpTopics,
		}
	} else {
		m.registry = nil
		m.spHandler = nil
		m.cfg = worker.MQTTConfigSnapshot{SpTopics: cfg.SpTopics}
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
		SetKeepAlive(5 * time.Second).
		SetOnConnectHandler(m.onConnect).
		SetConnectionLostHandler(m.onConnectionLost)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username).SetPassword(cfg.Password)
	}

	// Sparkplug B: register LWT as "STATE/{hostID}" = "OFFLINE", retained, QoS 1.
	// Per spec §7.5.1 the STATE payload is plain UTF-8 (not protobuf).
	if cfg.SparkplugEnabled {
		willTopic := sparkplug.StateTopicFor(cfg.SpHostID)
		opts.SetWill(willTopic, "OFFLINE", 1, true)
	}

	c := paho.NewClient(opts)
	m.client = c
	m.subs = map[string]struct{}{}
	m.connected.Store(false)

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

// onConnect is called by paho on every (re)connection.
func (m *Manager) onConnect(c paho.Client) {
	m.connected.Store(true)
	m.bdSeq.Add(1) // advance birth/death sequence on each new session

	m.mu.Lock()
	isSparkplug := m.spHandler != nil
	cfg := m.cfg
	m.mu.Unlock()

	if isSparkplug {
		m.onConnectSparkplug(c, cfg)
	} else {
		m.onConnectJSON(c, cfg)
	}
}

// onConnectSparkplug handles Sparkplug B session establishment:
//  1. Publishes STATE = "ONLINE" (retained, QoS 1) — the Primary Application birth.
//  2. Subscribes to each configured Sparkplug B group wildcard.
func (m *Manager) onConnectSparkplug(c paho.Client, cfg worker.MQTTConfigSnapshot) {
	// Publish Primary Application STATE birth certificate.
	stateTopic := sparkplug.StateTopicFor(cfg.SpHostID)
	tok := c.Publish(stateTopic, 1, true, []byte("ONLINE"))
	go func() {
		tok.Wait()
		if err := tok.Error(); err != nil {
			log.Printf("mqtt sparkplug STATE publish: %v", err)
		}
	}()

	// SpGroupID is newline/comma-separated; subscribe spBv1.0/{groupID}/# for each.
	groupIDs := parseTopicList(cfg.SpGroupID)
	if len(groupIDs) == 0 {
		groupIDs = []string{"#"} // fallback: subscribe all groups
	}
	for _, gid := range groupIDs {
		wildcard := sparkplug.WildcardFor(gid)
		log.Printf("mqtt sparkplug connected, subscribing %s", wildcard)
		wc := wildcard
		subTok := c.Subscribe(wc, 0, func(_ paho.Client, msg paho.Message) {
			m.onMessage(msg.Topic(), msg.Payload())
		})
		go func() {
			subTok.Wait()
			if err := subTok.Error(); err != nil {
				log.Printf("mqtt sparkplug subscribe %s: %v", wc, err)
			}
		}()
		m.mu.Lock()
		m.subs[wc] = struct{}{}
		m.mu.Unlock()
	}

	m.subscribeExtra(c, cfg)
}

// onConnectJSON handles plain-JSON session establishment (original behaviour).
func (m *Manager) onConnectJSON(c paho.Client, cfg worker.MQTTConfigSnapshot) {
	tqos := m.cache.TopicsQoS()
	log.Printf("mqtt connected, subscribing %d topic(s)", len(tqos))
	for topic, qos := range tqos {
		t, q := topic, qos
		tok := c.Subscribe(t, q, func(_ paho.Client, msg paho.Message) {
			m.onMessage(msg.Topic(), msg.Payload())
		})
		go func() {
			tok.Wait()
			if err := tok.Error(); err != nil {
				log.Printf("mqtt subscribe %s (qos %d): %v", t, q, err)
			}
		}()
		m.mu.Lock()
		m.subs[t] = struct{}{}
		m.mu.Unlock()
	}
	m.subscribeExtra(c, cfg)
}

// onConnectionLost is called by paho whenever the TCP connection drops.
func (m *Manager) onConnectionLost(_ paho.Client, err error) {
	m.connected.Store(false)
	log.Printf("mqtt connection lost: %v", err)

	// Mark all currently-online Sparkplug B nodes stale in IEC-104.
	m.mu.Lock()
	handler := m.spHandler
	reg := m.registry
	m.mu.Unlock()

	if handler == nil || reg == nil {
		return
	}
	for _, nk := range reg.MarkAllOffline() {
		base := sparkplug.Namespace + "/" + nk.GroupID + "/" + nk.EdgeNodeID
		handler.MarkNodeStale(base)
	}
}

func (m *Manager) onMessage(topic string, payload []byte) {
	m.messages.Add(1)
	m.lastMsg.Store(time.Now().UnixNano())

	m.mu.Lock()
	spHandler := m.spHandler
	ssfvHandler := m.ssfvHandler
	hook := m.monitorHook
	m.mu.Unlock()

	if spHandler != nil {
		t, ok := sparkplug.ParseTopic(topic)
		if !ok {
			// STATE topic or other non-spBv1.0 — fire hook immediately, no dispatch.
			if hook != nil {
				hook(topic, "state", payload, 0)
			}
			return
		}
		ssfvHits := spHandler.Dispatch(t, payload)
		if hook != nil {
			hook(topic, "sparkplug", payload, ssfvHits)
		}
		return
	}

	// JSON mode: SSFV handler intercepts equipment topics first.
	if ssfvHandler != nil && ssfvHandler.Handle(topic, payload) {
		if hook != nil {
			hook(topic, "ssfv", payload, 0)
		}
		return
	}

	if hook != nil {
		hook(topic, "json", payload, 0)
	}

	// Generic JSON dispatch (IEC-104 + history pipeline).
	maps := m.cache.Lookup(topic)
	worker.ParseAndDispatch(topic, payload, maps, m.d)
}

// publishRebirth sends an NCMD message requesting the EoN node to re-publish
// its NBIRTH.  Called by SparkplugHandler on out-of-sequence NDATA.
func (m *Manager) publishRebirth(groupID, nodeID string) {
	m.mu.Lock()
	c := m.client
	connected := m.connected.Load()
	m.mu.Unlock()

	if c == nil || !connected {
		return
	}
	topic := fmt.Sprintf("%s/%s/NCMD/%s", sparkplug.Namespace, groupID, nodeID)
	payload := sparkplug.EncodeNCMDRebirth()
	tok := c.Publish(topic, 0, false, payload)
	go func() {
		tok.Wait()
		if err := tok.Error(); err != nil {
			log.Printf("mqtt NCMD rebirth %s/%s: %v", groupID, nodeID, err)
		}
	}()
	log.Printf("sparkplug: sent NCMD rebirth to %s/%s", groupID, nodeID)
}

// subscribeExtra subscribes to all additional MQTT topic patterns stored in
// cfg.SpTopics (newline/comma-separated). Each pattern is subscribed at QoS 0
// and routes through onMessage — appearing in the broker monitor and triggering
// signal dispatch if a matching mapping exists.
func (m *Manager) subscribeExtra(c paho.Client, cfg worker.MQTTConfigSnapshot) {
	patterns := parseTopicList(cfg.SpTopics)
	if len(patterns) == 0 {
		return
	}
	log.Printf("mqtt: subscribing %d extra topic(s)", len(patterns))
	for _, pat := range patterns {
		p := pat
		tok := c.Subscribe(p, 0, func(_ paho.Client, msg paho.Message) {
			m.onMessage(msg.Topic(), msg.Payload())
		})
		go func() {
			tok.Wait()
			if err := tok.Error(); err != nil {
				log.Printf("mqtt extra subscribe %s: %v", p, err)
			}
		}()
		m.mu.Lock()
		m.subs[p] = struct{}{}
		m.mu.Unlock()
	}
}

// parseTopicList splits a newline/comma-separated topic pattern string,
// trims whitespace from each token, and discards blank entries.
func parseTopicList(raw string) []string {
	var out []string
	for _, tok := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	}) {
		if t := strings.TrimSpace(tok); t != "" {
			out = append(out, t)
		}
	}
	return out
}
