// Package iec104 = pure-Go IEC 60870-5-104 slave implementation.
//
// The gateway is strictly PASSIVE: each NativeServer binds a TCP listener and
// waits for SCADA masters to connect. No outbound connections are initiated by
// this package. A Manager owns N NativeServers, all bound on a gateway-wide
// listen IP, so the same point set can be exposed to multiple masters under
// different Common ASDU Addresses (one per row).
//
// Per-server SCADA IP allowlist: Accept rejects any remote whose IP is not in
// cfg.ScadaIPs (CSV). An empty allowlist rejects everything — fail closed.
//
// Scope: monitoring-direction (T1 path) only. Supports STARTDT/STOPDT/TESTFR,
// S-frame ACK, t2/t3 timers, k-window backpressure, General Interrogation
// (C_IC_NA_1 station, QOI=20), and encoders for the measurement type IDs
// handled in encodeInfoObject.
package iec104

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"goGateway/internal/models"
)

// DefaultDebug can be flipped at build time:
//
//	-ldflags "-X goGateway/internal/iec104.DefaultDebug=1"
//
// GW_IEC_DEBUG env var still overrides at runtime.
var DefaultDebug = "0"

var debug = resolveDebug()

func resolveDebug() bool {
	if v := os.Getenv("GW_IEC_DEBUG"); v != "" {
		return v == "1" || v == "true"
	}
	return DefaultDebug == "1"
}

// Debug reports whether IEC-104 frame tracing is enabled for this process.
func Debug() bool { return debug }

// --- Point store ----------------------------------------------------------

// pointStore is a thread-safe map of IOA → latest Point, shared across all
// clients of one NativeServer so every SCADA master sees the same snapshot.
type pointStore struct {
	mu sync.RWMutex
	m  map[int]Point
}

func newPointStore() *pointStore {
	return &pointStore{m: make(map[int]Point)}
}

func (s *pointStore) put(p Point) {
	s.mu.Lock()
	s.m[p.IOA] = p
	s.mu.Unlock()
}

func (s *pointStore) snapshot() []Point {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Point, 0, len(s.m))
	for _, p := range s.m {
		out = append(out, p)
	}
	return out
}

func (s *pointStore) size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.m)
}

// --- NativeServer ---------------------------------------------------------

// NativeServer is a single passive slave endpoint.
type NativeServer struct {
	mu       sync.RWMutex
	cfg      models.IEC104Server
	listenIP string              // gateway-wide bind IP, set by Manager before Start
	allow    map[string]struct{} // remote-IP allowlist parsed from cfg.ScadaIPs
	points   *pointStore         // per-server snapshot of latest values
	clients  map[*clientConn]struct{}
	listener net.Listener
	log      *log.Logger

	running bool
	quit    chan struct{}
	wg      sync.WaitGroup
}

// NewNativeServer creates one passive slave. Points is an optional shared
// store; if nil, the server owns its own.
func NewNativeServer(l *log.Logger, points *pointStore) *NativeServer {
	if l == nil {
		l = log.Default()
	}
	if points == nil {
		points = newPointStore()
	}
	return &NativeServer{
		points:  points,
		clients: make(map[*clientConn]struct{}),
		log:     l,
		quit:    make(chan struct{}),
	}
}

// applyConfig sets the endpoint parameters and refreshes the allowlist. Safe
// to call while running — running connections keep their snapshot, new ones
// see the new allowlist on Accept.
func (s *NativeServer) applyConfig(cfg models.IEC104Server) {
	s.mu.Lock()
	s.cfg = cfg
	s.allow = parseAllowlist(cfg.ScadaIPs)
	s.mu.Unlock()
}

// setListenIP updates the gateway-wide bind IP used on the next Start.
func (s *NativeServer) setListenIP(ip string) {
	s.mu.Lock()
	s.listenIP = ip
	s.mu.Unlock()
}

func (s *NativeServer) Status() ServerStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	activated := 0
	for cc := range s.clients {
		cc.mu.Lock()
		if cc.started {
			activated++
		}
		cc.mu.Unlock()
	}
	return ServerStatus{
		ID:        s.cfg.ID,
		Name:      s.cfg.Name,
		Listen:    s.listenIP,
		Port:      s.cfg.Port,
		ASDUAddr:  s.cfg.ASDUAddr,
		Clients:   len(s.clients),
		Activated: activated,
		Running:   s.running,
		Enabled:   s.cfg.Enabled,
	}
}

func (s *NativeServer) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("iec104: already running")
	}
	ip := s.listenIP
	if ip == "" {
		ip = "0.0.0.0"
	}
	addr := fmt.Sprintf("%s:%d", ip, s.cfg.Port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("iec104 listen %s: %w", addr, err)
	}
	s.listener = l
	s.running = true
	s.quit = make(chan struct{})
	name := s.cfg.Name
	asdu := s.cfg.ASDUAddr
	allowCount := len(s.allow)
	s.mu.Unlock()

	dbgState := "off"
	if debug {
		dbgState = "on"
	}
	if allowCount == 0 {
		s.log.Printf("iec104[%s]: WARNING empty SCADA allowlist — all incoming connections will be rejected", name)
	}
	s.log.Printf("iec104[%s]: listening on %s (ASDU=%d, allowlist=%d, frame-trace=%s)", name, addr, asdu, allowCount, dbgState)
	s.wg.Add(1)
	go s.acceptLoop(l)
	return nil
}

// Stop tears down the listener and all live client connections. Idempotent.
func (s *NativeServer) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	close(s.quit)
	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
	conns := make([]*clientConn, 0, len(s.clients))
	for cc := range s.clients {
		conns = append(conns, cc)
	}
	s.mu.Unlock()

	// Close outside the lock so cc.close() can acquire it to deregister.
	for _, cc := range conns {
		cc.conn.Close()
	}
	s.wg.Wait()
	return nil
}

// Dispatch caches the point and, for every client whose link is activated,
// queues a spontaneous I-frame.
func (s *NativeServer) Dispatch(p Point) {
	s.points.put(p)

	s.mu.RLock()
	targets := make([]*clientConn, 0, len(s.clients))
	for cc := range s.clients {
		targets = append(targets, cc)
	}
	asduAddr := uint16(s.cfg.ASDUAddr)
	name := s.cfg.Name
	s.mu.RUnlock()

	asdu, err := encodePoint(p, COT_SPONTANEOUS, asduAddr)
	if err != nil {
		s.log.Printf("iec104[%s]: dispatch encode %s IOA=%d: %v", name, p.TypeID, p.IOA, err)
		return
	}
	for _, cc := range targets {
		cc.send(asdu)
	}
}

// --- Accept loop ----------------------------------------------------------

func (s *NativeServer) acceptLoop(l net.Listener) {
	defer s.wg.Done()
	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				s.log.Printf("iec104[%s]: accept: %v", s.cfg.Name, err)
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}
		// Snapshot cfg so per-connection timers are race-free against Reload.
		s.mu.RLock()
		cfgSnap := s.cfg
		allowSnap := s.allow
		s.mu.RUnlock()

		remoteIP := remoteHost(conn.RemoteAddr())
		if !allowed(allowSnap, remoteIP) {
			s.log.Printf("iec104[%s]: REJECT %s (not in SCADA allowlist)", cfgSnap.Name, conn.RemoteAddr())
			conn.Close()
			continue
		}
		s.log.Printf("iec104[%s]: TCP accept from %s (awaiting STARTDT)", cfgSnap.Name, conn.RemoteAddr())
		cc := newClientConn(s, conn, cfgSnap)
		s.mu.Lock()
		s.clients[cc] = struct{}{}
		s.mu.Unlock()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			cc.run()
		}()
	}
}

// parseAllowlist turns a CSV string into a set of canonical IP strings.
// Invalid entries are dropped (caller logs at start time via allowCount).
func parseAllowlist(csv string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, raw := range strings.Split(csv, ",") {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		ip := net.ParseIP(s)
		if ip == nil {
			continue
		}
		out[ip.String()] = struct{}{}
	}
	return out
}

func remoteHost(addr net.Addr) string {
	ta, ok := addr.(*net.TCPAddr)
	if !ok {
		host, _, err := net.SplitHostPort(addr.String())
		if err != nil {
			return ""
		}
		return host
	}
	return ta.IP.String()
}

func allowed(set map[string]struct{}, ip string) bool {
	if len(set) == 0 || ip == "" {
		return false
	}
	_, ok := set[ip]
	return ok
}
