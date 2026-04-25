package iec104

import (
	"fmt"
	"log"
	"sync"

	"goGateway/internal/models"
)

// Manager owns a fleet of passive NativeServer instances. Each row in the
// iec104_servers table becomes one listener bound on the gateway-wide listen
// IP from iec104_gateway. Dispatch broadcasts every point to every enabled
// server so each SCADA master sees the gateway under its own Common ASDU
// Address. Points are cached in a shared store.
type Manager struct {
	log    *log.Logger
	points *pointStore

	mu       sync.RWMutex
	running  bool
	listenIP string
	servers  map[int64]*NativeServer // keyed by config row id
}

func NewManager(l *log.Logger) *Manager {
	if l == nil {
		l = log.Default()
	}
	return &Manager{
		log:      l,
		points:   newPointStore(),
		listenIP: "0.0.0.0",
		servers:  make(map[int64]*NativeServer),
	}
}

// Start brings up any already-configured (enabled) servers. No-op on first
// boot before Reload has been called.
func (m *Manager) Start() error {
	m.mu.Lock()
	m.running = true
	items := make([]*NativeServer, 0, len(m.servers))
	for _, s := range m.servers {
		items = append(items, s)
	}
	m.mu.Unlock()

	var firstErr error
	for _, s := range items {
		s.mu.RLock()
		enabled := s.cfg.Enabled
		s.mu.RUnlock()
		if !enabled {
			continue
		}
		if err := s.Start(); err != nil {
			m.log.Printf("iec104: start %q: %v", s.cfg.Name, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Stop tears down every server instance. Idempotent.
func (m *Manager) Stop() error {
	m.mu.Lock()
	m.running = false
	items := make([]*NativeServer, 0, len(m.servers))
	for _, s := range m.servers {
		items = append(items, s)
	}
	m.mu.Unlock()

	for _, s := range items {
		_ = s.Stop()
	}
	return nil
}

// Dispatch fans out a point to every enabled server. Each server re-encodes
// with its own ASDU address.
func (m *Manager) Dispatch(p Point) {
	m.points.put(p)

	m.mu.RLock()
	targets := make([]*NativeServer, 0, len(m.servers))
	for _, s := range m.servers {
		targets = append(targets, s)
	}
	m.mu.RUnlock()

	for _, s := range targets {
		s.mu.RLock()
		enabled := s.cfg.Enabled
		running := s.running
		s.mu.RUnlock()
		if !enabled || !running {
			continue
		}
		s.Dispatch(p)
	}
}

// Reload synchronizes the live fleet with the desired gateway-wide listen IP
// and the desired set of slave configs.
//
//   - listen IP changed → stop all, set new IP, start enabled
//   - new row           → create + start (if enabled)
//   - removed row       → stop + delete
//   - port changed      → restart
//   - enabled toggle    → start/stop
//   - other fields      → applied; allowlist takes effect on next Accept;
//     timers take effect on next client connection.
func (m *Manager) Reload(gw models.IEC104Gateway, cfgs []models.IEC104Server) error {
	wanted := make(map[int64]models.IEC104Server, len(cfgs))
	for _, c := range cfgs {
		wanted[c.ID] = c
	}

	newIP := gw.ListenIP
	if newIP == "" {
		newIP = "0.0.0.0"
	}

	m.mu.Lock()
	ipChanged := m.listenIP != newIP
	m.listenIP = newIP
	running := m.running
	// Snapshot for IP-change restart.
	all := make([]*NativeServer, 0, len(m.servers))
	for _, s := range m.servers {
		all = append(all, s)
	}
	m.mu.Unlock()

	if ipChanged {
		for _, s := range all {
			_ = s.Stop()
			s.setListenIP(newIP)
		}
	}

	// Remove servers no longer present.
	m.mu.Lock()
	toStop := make([]*NativeServer, 0)
	for id, s := range m.servers {
		if _, ok := wanted[id]; !ok {
			toStop = append(toStop, s)
			delete(m.servers, id)
		}
	}
	m.mu.Unlock()
	for _, s := range toStop {
		_ = s.Stop()
	}

	var firstErr error
	for id, cfg := range wanted {
		m.mu.Lock()
		cur, exists := m.servers[id]
		m.mu.Unlock()

		if !exists {
			s := NewNativeServer(m.log, m.points)
			s.setListenIP(newIP)
			s.applyConfig(cfg)
			m.mu.Lock()
			m.servers[id] = s
			m.mu.Unlock()
			if running && cfg.Enabled {
				if err := s.Start(); err != nil {
					m.log.Printf("iec104: start %q: %v", cfg.Name, err)
					if firstErr == nil {
						firstErr = err
					}
				}
			}
			continue
		}

		cur.mu.RLock()
		old := cur.cfg
		wasRunning := cur.running
		cur.mu.RUnlock()

		portChanged := old.Port != cfg.Port
		cur.applyConfig(cfg)

		switch {
		case ipChanged && cfg.Enabled && running:
			// Already stopped above for IP change; bring back up.
			if err := cur.Start(); err != nil {
				m.log.Printf("iec104: restart %q: %v", cfg.Name, err)
				if firstErr == nil {
					firstErr = err
				}
			}
		case !cfg.Enabled && wasRunning:
			_ = cur.Stop()
		case cfg.Enabled && !wasRunning && running:
			if err := cur.Start(); err != nil {
				m.log.Printf("iec104: start %q: %v", cfg.Name, err)
				if firstErr == nil {
					firstErr = err
				}
			}
		case cfg.Enabled && wasRunning && portChanged:
			_ = cur.Stop()
			if err := cur.Start(); err != nil {
				m.log.Printf("iec104: restart %q: %v", cfg.Name, err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}

	if firstErr != nil {
		return fmt.Errorf("iec104 reload: %w", firstErr)
	}
	return nil
}

// Status returns a fleet summary.
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := Status{
		ListenIP: m.listenIP,
		Points:   m.points.size(),
		Servers:  make([]ServerStatus, 0, len(m.servers)),
	}
	for _, s := range m.servers {
		st := s.Status()
		out.Servers = append(out.Servers, st)
		if st.Running {
			out.Running = true
		}
		out.Clients += st.Clients
	}
	return out
}

// Snapshot returns every cached point (deduplicated — shared store).
func (m *Manager) Snapshot() []Point {
	return m.points.snapshot()
}
