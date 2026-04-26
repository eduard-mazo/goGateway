package iec104

import (
	"fmt"
	"log"
	"sync"

	"goGateway/internal/models"
)

// Manager owns a fleet of passive NativeServer instances. Each row in the
// iec104_servers table becomes one listener bound on the gateway-wide listen
// IP from iec104_gateway. Each server keeps its own point cache; Dispatch
// routes one point to one server (selected by signal_mappings.server_id) so
// each SCADA master only sees the slice of the point set assigned to it.
type Manager struct {
	log *log.Logger

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

// Dispatch routes a point to the slave identified by serverID. Drops the
// point silently if the target is unknown, disabled or down — the worker
// reads a snapshot from the cache, so a brief race during reload is normal.
func (m *Manager) Dispatch(serverID int64, p Point) {
	m.mu.RLock()
	s, ok := m.servers[serverID]
	m.mu.RUnlock()
	if !ok {
		return
	}
	s.mu.RLock()
	enabled := s.cfg.Enabled
	running := s.running
	s.mu.RUnlock()
	if !enabled || !running {
		return
	}
	s.Dispatch(p)
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
			// Each server gets its own point store; mappings carry a server_id.
			s := NewNativeServer(m.log, nil)
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
		Servers:  make([]ServerStatus, 0, len(m.servers)),
	}
	for _, s := range m.servers {
		st := s.Status()
		out.Servers = append(out.Servers, st)
		if st.Running {
			out.Running = true
		}
		out.Clients += st.Clients
		out.Activated += st.Activated
		out.Points += s.points.size()
	}
	return out
}

// Snapshot returns the union of every server's cached point set (one entry
// per (server, IOA) — same IOA on different servers is preserved).
func (m *Manager) Snapshot() []Point {
	m.mu.RLock()
	servers := make([]*NativeServer, 0, len(m.servers))
	for _, s := range m.servers {
		servers = append(servers, s)
	}
	m.mu.RUnlock()
	var out []Point
	for _, s := range servers {
		out = append(out, s.points.snapshot()...)
	}
	return out
}
