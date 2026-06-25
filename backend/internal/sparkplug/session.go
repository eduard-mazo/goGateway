package sparkplug

import "sync"

// NodeKey uniquely identifies an EoN node within the MQTT infrastructure.
type NodeKey struct {
	GroupID    string
	EdgeNodeID string
}

// DeviceKey uniquely identifies a device attached to an EoN node.
type DeviceKey struct {
	NodeKey
	DeviceID string
}

// NodeSession tracks the live Sparkplug B session state for one EoN node.
// It is the authoritative source for:
//   - whether the node is online (NBIRTH received, no subsequent NDEATH)
//   - the alias→name map seeded by the most recent NBIRTH
//   - the current sequence number for out-of-order detection
//   - per-device session state
type NodeSession struct {
	mu       sync.RWMutex
	online   bool
	bdSeq    uint64            // birth/death sequence from the most recent NBIRTH
	seq      uint64            // last received payload sequence number
	aliasMap map[uint64]string // alias → metric name (seeded from NBIRTH)
	devices  map[string]*DeviceSession
}

// DeviceSession tracks session state for one Sparkplug B device.
type DeviceSession struct {
	mu       sync.RWMutex
	online   bool
	seq      uint64
	aliasMap map[uint64]string
}

// Online reports whether this node has sent a NBIRTH and not yet an NDEATH.
func (s *NodeSession) Online() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.online
}

// SetBirth records a successful NBIRTH, seeds the alias map, and advances
// the birth/death sequence counter.
//
// It also reports a bdSeq REGRESSION while the node was still considered online:
// a commanded rebirth reuses the same MQTT session (same bdSeq), and a genuine
// reconnect first delivers the Last-Will NDEATH (→ offline) then an NBIRTH with
// a higher bdSeq — so a *lower* bdSeq arriving while we still think the node is
// online means a second producer is publishing under the same node id (its
// independent bdSeq counter started lower). This is a strong duplicate-identity
// signal (caller logs/records it). Wrap-around (255→0) can rarely false-positive;
// it is a warning only, never a hard action.
func (s *NodeSession) SetBirth(payload *Payload) (bdSeq uint64, regressedWhileOnline bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wasOnline := s.online
	prevBd := s.bdSeq

	// Extract the incoming bdSeq before mutating session state.
	newBd := prevBd
	s.aliasMap = make(map[uint64]string, len(payload.Metrics))
	for _, m := range payload.Metrics {
		if m.HasAlias && m.Name != "" {
			s.aliasMap[m.Alias] = m.Name
		}
		// bdSeq metric carries the birth/death sequence number.
		if m.Name == "bdSeq" {
			if v, ok := m.Float64(); ok {
				newBd = uint64(v)
			}
		}
	}

	regressedWhileOnline = wasOnline && newBd < prevBd
	s.online = true
	s.seq = payload.Seq
	s.bdSeq = newBd
	return newBd, regressedWhileOnline
}

// SetDeath marks the node offline.
func (s *NodeSession) SetDeath() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.online = false
}

// AdvanceSeq validates and stores the incoming sequence number.
// Returns true when the seq is the expected next value (previous+1 mod 256).
// Returns false when the seq is out of order — caller should request rebirth.
func (s *NodeSession) AdvanceSeq(incoming uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	expected := (s.seq + 1) % 256
	if incoming != expected {
		return false
	}
	s.seq = incoming
	return true
}

// SyncSeq unconditionally sets the node's last-seen sequence number.
// Sparkplug B maintains ONE seq counter per EoN node, shared across NBIRTH,
// NDATA, DBIRTH, DDATA and DDEATH. DBIRTH is a sequenced message published in
// the birth burst right after NBIRTH; rather than strict-validating it (which
// would false-trigger rebirths on benign NBIRTH/DBIRTH delivery races), the
// caller syncs the node seq forward to the DBIRTH's value so the following
// NDATA/DDATA validate against the correct expected value. A genuinely lost
// DBIRTH still surfaces as a gap on the next AdvanceSeq → rebirth.
func (s *NodeSession) SyncSeq(seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq = seq
}

// ResolveName returns the metric name for a given alias.
// If the metric has an explicit Name, it is returned directly.
// Falls back to the alias map seeded from the last NBIRTH.
func (s *NodeSession) ResolveName(m *Metric) string {
	if m.Name != "" {
		return m.Name
	}
	if !m.HasAlias {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.aliasMap[m.Alias]
}

// DeviceSession returns or creates the session for a device attached to this node.
func (s *NodeSession) Device(deviceID string) *DeviceSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.devices == nil {
		s.devices = make(map[string]*DeviceSession)
	}
	d, ok := s.devices[deviceID]
	if !ok {
		d = &DeviceSession{aliasMap: make(map[uint64]string)}
		s.devices[deviceID] = d
	}
	return d
}

// ── DeviceSession methods ────────────────────────────────────────────────────

func (d *DeviceSession) SetBirth(payload *Payload) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.online = true
	d.seq = payload.Seq
	d.aliasMap = make(map[uint64]string, len(payload.Metrics))
	for _, m := range payload.Metrics {
		if m.HasAlias && m.Name != "" {
			d.aliasMap[m.Alias] = m.Name
		}
	}
}

func (d *DeviceSession) SetDeath() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.online = false
}

func (d *DeviceSession) AdvanceSeq(incoming uint64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	expected := (d.seq + 1) % 256
	if incoming != expected {
		return false
	}
	d.seq = incoming
	return true
}

func (d *DeviceSession) ResolveName(m *Metric) string {
	if m.Name != "" {
		return m.Name
	}
	if !m.HasAlias {
		return ""
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.aliasMap[m.Alias]
}

// ── Registry ─────────────────────────────────────────────────────────────────

// Registry maintains session state for all known EoN nodes.
// It is safe for concurrent access.
type Registry struct {
	mu    sync.RWMutex
	nodes map[NodeKey]*NodeSession
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{nodes: make(map[NodeKey]*NodeSession)}
}

// Session returns the NodeSession for the given key, creating one if it does
// not exist yet.
func (r *Registry) Session(k NodeKey) *NodeSession {
	r.mu.RLock()
	s, ok := r.nodes[k]
	r.mu.RUnlock()
	if ok {
		return s
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok = r.nodes[k]; ok {
		return s
	}
	s = &NodeSession{aliasMap: make(map[uint64]string)}
	r.nodes[k] = s
	return s
}

// MarkAllOffline sets every tracked node to offline without dispatching
// IEC-104 quality — the caller is responsible for dispatching stale points.
func (r *Registry) MarkAllOffline() []NodeKey {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]NodeKey, 0)
	for k, s := range r.nodes {
		s.mu.Lock()
		wasOnline := s.online
		s.online = false
		s.mu.Unlock()
		if wasOnline {
			out = append(out, k)
		}
	}
	return out
}
