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
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"goGateway/internal/models"
)

// DefaultDebug can be flipped at build time:
//   -ldflags "-X goGateway/internal/iec104.DefaultDebug=1"
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

func (cc *clientConn) trace(dir, kind string, b []byte) {
	if !debug {
		return
	}
	cc.srv.log.Printf("iec104[%s] %s %s %s %s",
		cc.srv.cfg.Name, dir, cc.conn.RemoteAddr(), kind, hex.EncodeToString(b))
}

// --- Type IDs (monitoring direction) --------------------------------------

const (
	M_SP_NA_1 byte = 1  // single-point, no time
	M_DP_NA_1 byte = 3  // double-point, no time
	M_ME_NA_1 byte = 9  // normalized value
	M_ME_NB_1 byte = 11 // scaled value (int16)
	M_ME_NC_1 byte = 13 // short float, no time
	M_IT_NA_1 byte = 15 // integrated totals
	M_SP_TB_1 byte = 30 // single-point + CP56Time2a
	M_ME_TF_1 byte = 36 // short float + CP56Time2a
	M_IT_TB_1 byte = 37 // integrated totals + CP56Time2a

	C_IC_NA_1 byte = 100 // general interrogation command
	C_CS_NA_1 byte = 103 // clock sync command
)

// --- Cause of Transmission ------------------------------------------------

const (
	COT_SPONTANEOUS byte = 3
	COT_ACTIVATION  byte = 6
	COT_ACT_CON     byte = 7
	COT_ACT_TERM    byte = 10
	COT_INTROGEN    byte = 20
)

// --- Quality bits (QDS / SIQ / DIQ) ---------------------------------------

const (
	QualityGood        = 0x00
	QualityInvalid     = 0x80 // IV — value not usable
	QualityNotTopical  = 0x40 // NT — value is old/stale
	QualitySubstituted = 0x20 // SB — value was manually substituted
	QualityBlocked     = 0x10 // BL — value update blocked
)

// seqMask enforces the 15-bit IEC-104 sequence number space (N(S)/N(R)).
const seqMask uint16 = 0x7FFF

// --- U-frame fixed encodings ----------------------------------------------

var (
	uSTARTDT_CON = []byte{0x68, 0x04, 0x0B, 0x00, 0x00, 0x00}
	uSTOPDT_CON  = []byte{0x68, 0x04, 0x23, 0x00, 0x00, 0x00}
	uTESTFR      = []byte{0x68, 0x04, 0x43, 0x00, 0x00, 0x00}
	uTESTFR_CON  = []byte{0x68, 0x04, 0x83, 0x00, 0x00, 0x00}
)

// --- Public domain types --------------------------------------------------

type Point struct {
	IOA       int
	TypeID    string
	Value     float64
	Quality   int
	Timestamp time.Time
}

// ServerStatus = per-instance runtime snapshot.
//
// Clients counts every TCP-accepted connection. Activated counts only the
// subset where the master has completed STARTDT — i.e., the IEC-104 protocol
// link is actually up and exchanging frames. UI uses Activated for the
// "linked" indication; Clients alone means "TCP only, protocol not started".
type ServerStatus struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Listen    string `json:"listen"`
	Port      int    `json:"port"`
	ASDUAddr  int    `json:"asdu_addr"`
	Clients   int    `json:"clients"`
	Activated int    `json:"activated"`
	Running   bool   `json:"running"`
	Enabled   bool   `json:"enabled"`
}

// Status = fleet summary returned by Manager.Status.
type Status struct {
	Running   bool           `json:"running"`   // any instance up
	ListenIP  string         `json:"listen_ip"` // gateway-wide bind IP
	Points    int            `json:"points"`    // cached points (shared)
	Clients   int            `json:"clients"`   // sum across instances (TCP)
	Activated int            `json:"activated"` // sum of protocol-active links
	Servers   []ServerStatus `json:"servers"`
}

// Server is the fleet-level facade consumed by MQTT/API layers.
//
// Dispatch routes a point to exactly one IEC-104 slave (identified by its
// row id in iec104_servers). This matches the shape of signal_mappings,
// where every mapping is pinned to a single server: the worker resolves the
// target server_id from the cache and the manager forwards there only.
type Server interface {
	Start() error
	Stop() error
	Dispatch(serverID int64, p Point)
	Reload(gw models.IEC104Gateway, cfgs []models.IEC104Server) error
	Status() Status
	Snapshot() []Point
}

// --- NativeServer ---------------------------------------------------------

// NativeServer is a single passive slave endpoint.
type NativeServer struct {
	mu       sync.RWMutex
	cfg      models.IEC104Server
	listenIP string // gateway-wide bind IP, set by Manager before Start
	allow    map[string]struct{} // remote-IP allowlist parsed from cfg.ScadaIPs
	points   *pointStore         // shared snapshot of latest values
	clients  map[*clientConn]struct{}
	listener net.Listener
	log      *log.Logger

	running bool
	quit    chan struct{}
	wg      sync.WaitGroup
}

// pointStore = shared map of IOA → latest Point. Manager passes one instance
// to all NativeServers so every SCADA master sees the same snapshot.
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

// --- Listener / accept loop -----------------------------------------------

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

// --- Per-connection state -------------------------------------------------

type clientConn struct {
	srv     *NativeServer
	cfg     models.IEC104Server // frozen at accept
	conn    net.Conn
	rd      *bufio.Reader
	writeMu sync.Mutex

	mu         sync.Mutex
	tx         uint16 // N(S) of next I-frame to send (15-bit)
	rx         uint16 // next expected N(S) = count of I-frames received
	ackedTx    uint16 // peer-acknowledged up to (exclusive)
	unacked    uint16 // tx - ackedTx, bounded by k
	rxSinceAck uint16
	started    bool

	outbox    chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

func newClientConn(s *NativeServer, c net.Conn, cfg models.IEC104Server) *clientConn {
	return &clientConn{
		srv:    s,
		cfg:    cfg,
		conn:   c,
		rd:     bufio.NewReader(c),
		outbox: make(chan []byte, 256),
		closed: make(chan struct{}),
	}
}

func (cc *clientConn) run() {
	defer cc.close()
	go cc.sender()
	go cc.timerLoop()
	cc.reader()
}

func (cc *clientConn) close() {
	cc.closeOnce.Do(func() {
		close(cc.closed)
		cc.conn.Close()
		cc.srv.mu.Lock()
		delete(cc.srv.clients, cc)
		cc.srv.mu.Unlock()
		cc.srv.log.Printf("iec104[%s]: TCP closed %s", cc.cfg.Name, cc.conn.RemoteAddr())
	})
}

// send queues an ASDU for transmission as an I-frame. Drops newest if the
// outbox is full (slow peer) to protect the gateway.
func (cc *clientConn) send(asdu []byte) {
	select {
	case cc.outbox <- asdu:
	case <-cc.closed:
	default:
		cc.srv.log.Printf("iec104[%s]: outbox full, dropping I-frame to %s", cc.cfg.Name, cc.conn.RemoteAddr())
	}
}

// --- Reader ---------------------------------------------------------------

func (cc *clientConn) reader() {
	t3 := timerDuration(cc.cfg.T3, 20*time.Second)
	for {
		cc.conn.SetReadDeadline(time.Now().Add(t3))
		start, err := cc.rd.ReadByte()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				cc.writeRaw(uTESTFR)
				continue
			}
			if err != io.EOF {
				cc.srv.log.Printf("iec104[%s]: read: %v", cc.cfg.Name, err)
			}
			return
		}
		if start != 0x68 {
			cc.srv.log.Printf("iec104[%s]: bad start byte 0x%02x, resync", cc.cfg.Name, start)
			continue
		}
		l, err := cc.rd.ReadByte()
		if err != nil {
			return
		}
		if l < 4 || l > 253 {
			cc.srv.log.Printf("iec104[%s]: bad APDU length %d", cc.cfg.Name, l)
			return
		}
		body := make([]byte, l)
		if _, err := io.ReadFull(cc.rd, body); err != nil {
			return
		}
		if debug {
			full := append([]byte{0x68, l}, body...)
			cc.trace("RX", frameKind(body), full)
		}
		cc.handleAPDU(body)
	}
}

func (cc *clientConn) handleAPDU(body []byte) {
	switch body[0] & 0x03 {
	case 0x03:
		cc.handleU(body)
	case 0x01:
		peerNR := (uint16(body[2]) | uint16(body[3])<<8) >> 1
		cc.ackUpTo(peerNR & seqMask)
	default:
		peerNS := (uint16(body[0]) | uint16(body[1])<<8) >> 1
		peerNR := (uint16(body[2]) | uint16(body[3])<<8) >> 1
		cc.ackUpTo(peerNR & seqMask)
		cc.onIFrame(peerNS&seqMask, body[4:])
	}
}

func (cc *clientConn) handleU(body []byte) {
	switch body[0] {
	case 0x07: // STARTDT act
		cc.mu.Lock()
		cc.started = true
		cc.mu.Unlock()
		cc.writeRaw(uSTARTDT_CON)
		cc.srv.log.Printf("iec104[%s]: master %s activated link (STARTDT)", cc.cfg.Name, cc.conn.RemoteAddr())
	case 0x13: // STOPDT act
		cc.mu.Lock()
		cc.started = false
		cc.mu.Unlock()
		cc.writeRaw(uSTOPDT_CON)
	case 0x43: // TESTFR act
		cc.writeRaw(uTESTFR_CON)
	case 0x0B, 0x23, 0x83:
		// CON frames — ignore. Server never initiates STARTDT/STOPDT/TESTFR
		// because it is strictly passive.
	default:
		cc.srv.log.Printf("iec104[%s]: unknown U-frame 0x%02x", cc.cfg.Name, body[0])
	}
}

func (cc *clientConn) onIFrame(peerNS uint16, asdu []byte) {
	cc.mu.Lock()
	expected := cc.rx
	cc.rx = (peerNS + 1) & seqMask
	cc.rxSinceAck++
	needAck := cc.rxSinceAck >= cc.wThreshold()
	if needAck {
		cc.rxSinceAck = 0
	}
	cc.mu.Unlock()

	if peerNS != expected {
		cc.srv.log.Printf("iec104[%s]: N(S) gap: got %d expected %d", cc.cfg.Name, peerNS, expected)
	}
	if needAck {
		cc.sendSFrame()
	}

	if len(asdu) < 6 {
		return
	}
	typeID := asdu[0]
	cot := asdu[2] & 0x3F
	body := asdu[6:]

	switch typeID {
	case C_IC_NA_1:
		cc.handleInterrogation(cot, body)
	case C_CS_NA_1:
		// Clock sync: mirror back as ACT_CON. No internal effect — gateway
		// keeps its own wall clock.
		cc.replyAck(typeID, COT_ACT_CON, body)
	default:
		cc.srv.log.Printf("iec104[%s]: unhandled command type %d", cc.cfg.Name, typeID)
	}
}

// handleInterrogation replies ACT_CON, dumps cached points with the matching
// INRO* COT, then sends ACT_TERM. Per IEC 60870-5-101 §7.2.6.22, station GI
// (QOI=20) returns every point; group GIs (QOI 21..36 = groups 1..16) would
// normally filter by group assignment — we don't model groups, so they fall
// back to the full dump but still echo the requested QOI in the response COT.
//
// Points are grouped by Type ID and packed into multi-object ASDUs (SQ=0,
// up to ~120 objects per APDU) so the snapshot streams in a handful of
// frames instead of one frame per point.
func (cc *clientConn) handleInterrogation(cot byte, body []byte) {
	if cot != COT_ACTIVATION {
		return
	}
	if len(body) < 4 {
		return
	}
	qoi := body[3]
	// Reject QOI outside the station/group range with a negative ACT_CON.
	if qoi < 20 || qoi > 36 {
		// P/N=1 → negative confirmation per IEC 60870-5-101 §7.2.3.
		cc.replyAck(C_IC_NA_1, COT_ACT_CON|0x40, body)
		return
	}
	respCOT := COT_INTROGEN + (qoi - 20)

	cc.replyAck(C_IC_NA_1, COT_ACT_CON, body)

	cc.srv.mu.RLock()
	asduAddr := uint16(cc.srv.cfg.ASDUAddr)
	cc.srv.mu.RUnlock()

	// Bucket the snapshot by TypeID so each ASDU carries one type only.
	buckets := make(map[byte][][]byte)
	order := make([]byte, 0, 8)
	for _, p := range cc.srv.points.snapshot() {
		typeID, obj, err := encodeInfoObject(p)
		if err != nil {
			continue
		}
		if _, seen := buckets[typeID]; !seen {
			order = append(order, typeID)
		}
		buckets[typeID] = append(buckets[typeID], obj)
	}

	for _, typeID := range order {
		for _, asdu := range packInfoObjects(typeID, buckets[typeID], respCOT, asduAddr) {
			cc.send(asdu)
		}
	}
	cc.replyAck(C_IC_NA_1, COT_ACT_TERM, body)
}

// packInfoObjects builds one or more ASDUs (SQ=0) carrying objects of a
// single TypeID. Each ASDU stays under the 253-byte APDU payload cap and the
// 7-bit NumObjects limit.
func packInfoObjects(typeID byte, objs [][]byte, cot byte, asduAddr uint16) [][]byte {
	if len(objs) == 0 {
		return nil
	}
	const maxAPDUPayload = 253 // APDU body excl. 0x68+len header
	const maxASDUBody = maxAPDUPayload - 4 - 6 // - APCI ctrl - ASDU hdr
	const maxObjects = 127

	var out [][]byte
	i := 0
	for i < len(objs) {
		j := i
		size := 0
		for j < len(objs) && (j-i) < maxObjects && size+len(objs[j]) <= maxASDUBody {
			size += len(objs[j])
			j++
		}
		if j == i {
			// Single object exceeds the cap — emit it solo and skip.
			j = i + 1
			size = len(objs[i])
		}
		asdu := make([]byte, 6, 6+size)
		asdu[0] = typeID
		asdu[1] = byte(j - i) // SQ=0, NumObjects = j-i
		asdu[2] = cot
		asdu[3] = 0
		binary.LittleEndian.PutUint16(asdu[4:6], asduAddr)
		for k := i; k < j; k++ {
			asdu = append(asdu, objs[k]...)
		}
		out = append(out, asdu)
		i = j
	}
	return out
}

// replyAck sends a single-object reply (ACT_CON or ACT_TERM). body is the
// original info-object bytes which we echo verbatim.
func (cc *clientConn) replyAck(typeID, cot byte, body []byte) {
	cc.srv.mu.RLock()
	asduAddr := uint16(cc.srv.cfg.ASDUAddr)
	cc.srv.mu.RUnlock()

	asdu := make([]byte, 6+len(body))
	asdu[0] = typeID
	asdu[1] = 0x01
	asdu[2] = cot
	asdu[3] = 0
	binary.LittleEndian.PutUint16(asdu[4:6], asduAddr)
	copy(asdu[6:], body)
	cc.send(asdu)
}

// --- Sender + timers ------------------------------------------------------

func (cc *clientConn) sender() {
	for {
		select {
		case <-cc.closed:
			return
		case asdu := <-cc.outbox:
			// Wait for STARTDT and window slack.
			for {
				cc.mu.Lock()
				started := cc.started
				room := cc.unacked < cc.kThreshold()
				cc.mu.Unlock()
				if started && room {
					break
				}
				select {
				case <-cc.closed:
					return
				case <-time.After(50 * time.Millisecond):
				}
			}

			cc.mu.Lock()
			ns := cc.tx
			nr := cc.rx
			cc.tx = (cc.tx + 1) & seqMask
			cc.unacked++
			cc.rxSinceAck = 0 // I-frame carries our N(R).
			cc.mu.Unlock()

			cc.writeRaw(wrapIFrame(ns, nr, asdu))
		}
	}
}

// timerLoop flushes pending S-frame acks on t2. Read-deadline already handles
// t3 from the reader side.
func (cc *clientConn) timerLoop() {
	t2 := timerDuration(cc.cfg.T2, 10*time.Second)
	tick := time.NewTicker(t2 / 2)
	defer tick.Stop()
	for {
		select {
		case <-cc.closed:
			return
		case <-tick.C:
			cc.mu.Lock()
			pending := cc.rxSinceAck > 0
			cc.mu.Unlock()
			if pending {
				cc.sendSFrame()
			}
		}
	}
}

func (cc *clientConn) sendSFrame() {
	cc.mu.Lock()
	nr := cc.rx
	cc.rxSinceAck = 0
	cc.mu.Unlock()
	s := []byte{0x68, 0x04, 0x01, 0x00, byte(nr << 1), byte((nr << 1) >> 8)}
	cc.writeRaw(s)
}

// ackUpTo advances ackedTx to nr, updates unacked. Uses 15-bit modular math.
func (cc *clientConn) ackUpTo(nr uint16) {
	cc.mu.Lock()
	diff := (nr - cc.ackedTx) & seqMask
	if diff > cc.unacked {
		diff = cc.unacked
	}
	cc.unacked -= diff
	cc.ackedTx = nr
	cc.mu.Unlock()
}

// --- Threshold helpers ----------------------------------------------------

func (cc *clientConn) kThreshold() uint16 {
	if cc.cfg.K > 0 {
		return uint16(cc.cfg.K)
	}
	return 12
}

func (cc *clientConn) wThreshold() uint16 {
	if cc.cfg.W > 0 {
		return uint16(cc.cfg.W)
	}
	return 8
}

func timerDuration(configSec int, fallback time.Duration) time.Duration {
	if configSec > 0 {
		return time.Duration(configSec) * time.Second
	}
	return fallback
}

// --- Low-level writes -----------------------------------------------------

func (cc *clientConn) writeRaw(b []byte) {
	cc.writeMu.Lock()
	defer cc.writeMu.Unlock()
	cc.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if debug && len(b) >= 3 {
		cc.trace("TX", frameKind(b[2:]), b)
	}
	if _, err := cc.conn.Write(b); err != nil {
		cc.srv.log.Printf("iec104[%s]: write: %v", cc.cfg.Name, err)
		cc.conn.Close()
	}
}

func frameKind(body []byte) string {
	if len(body) < 1 {
		return "?"
	}
	switch body[0] & 0x03 {
	case 0x03:
		switch body[0] {
		case 0x07:
			return "U:STARTDT.act"
		case 0x0B:
			return "U:STARTDT.con"
		case 0x13:
			return "U:STOPDT.act"
		case 0x23:
			return "U:STOPDT.con"
		case 0x43:
			return "U:TESTFR.act"
		case 0x83:
			return "U:TESTFR.con"
		}
		return "U:?"
	case 0x01:
		return "S"
	}
	if len(body) >= 5 {
		return fmt.Sprintf("I type=%d", body[4])
	}
	return "I"
}

// --- Frame wrappers -------------------------------------------------------

func wrapIFrame(ns, nr uint16, asdu []byte) []byte {
	apduLen := 4 + len(asdu)
	f := make([]byte, 2+apduLen)
	f[0] = 0x68
	f[1] = byte(apduLen)
	binary.LittleEndian.PutUint16(f[2:4], ns<<1)
	binary.LittleEndian.PutUint16(f[4:6], nr<<1)
	copy(f[6:], asdu)
	return f
}

// --- ASDU encoding --------------------------------------------------------

func encodePoint(p Point, cot byte, asduAddr uint16) ([]byte, error) {
	typeID, body, err := encodeInfoObject(p)
	if err != nil {
		return nil, err
	}
	asdu := make([]byte, 6+len(body))
	asdu[0] = typeID
	asdu[1] = 0x01
	asdu[2] = cot
	asdu[3] = 0
	binary.LittleEndian.PutUint16(asdu[4:6], asduAddr)
	copy(asdu[6:], body)
	return asdu, nil
}

func encodeInfoObject(p Point) (byte, []byte, error) {
	q := byte(p.Quality)
	ioa := ioaBytes(p.IOA)
	ts := cp56Time2a(p.Timestamp)
	switch p.TypeID {

	case "M_ME_TF_1":
		b := make([]byte, 0, 15)
		b = append(b, ioa...)
		b = append(b, float32LE(p.Value)...)
		b = append(b, q)
		b = append(b, ts...)
		return M_ME_TF_1, b, nil

	case "M_ME_NC_1":
		b := make([]byte, 0, 8)
		b = append(b, ioa...)
		b = append(b, float32LE(p.Value)...)
		b = append(b, q)
		return M_ME_NC_1, b, nil

	case "M_ME_NB_1":
		b := make([]byte, 0, 6)
		b = append(b, ioa...)
		b = append(b, int16LE(p.Value)...)
		b = append(b, q)
		return M_ME_NB_1, b, nil

	case "M_ME_NA_1":
		// Normalized: float in [-1, +1) scaled to int16.
		v := p.Value
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		n := int16(v * 32767)
		b := make([]byte, 0, 6)
		b = append(b, ioa...)
		b = append(b, byte(n), byte(n>>8))
		b = append(b, q)
		return M_ME_NA_1, b, nil

	case "M_SP_NA_1":
		siq := q & 0xF0
		if p.Value != 0 {
			siq |= 0x01
		}
		b := make([]byte, 0, 4)
		b = append(b, ioa...)
		b = append(b, siq)
		return M_SP_NA_1, b, nil

	case "M_SP_TB_1":
		siq := q & 0xF0
		if p.Value != 0 {
			siq |= 0x01
		}
		b := make([]byte, 0, 11)
		b = append(b, ioa...)
		b = append(b, siq)
		b = append(b, ts...)
		return M_SP_TB_1, b, nil

	case "M_DP_NA_1":
		diq := q & 0xF0
		if p.Value > 0 {
			diq |= 0x02
		} else {
			diq |= 0x01
		}
		b := make([]byte, 0, 4)
		b = append(b, ioa...)
		b = append(b, diq)
		return M_DP_NA_1, b, nil

	case "M_IT_NA_1":
		counter := int32(p.Value)
		qds := byte(0)
		if q&0x80 != 0 {
			qds |= 0x80
		}
		b := make([]byte, 0, 8)
		b = append(b, ioa...)
		b = append(b, byte(counter), byte(counter>>8), byte(counter>>16), byte(counter>>24))
		b = append(b, qds)
		return M_IT_NA_1, b, nil

	case "M_IT_TB_1":
		counter := int32(p.Value)
		qds := byte(0)
		if q&0x80 != 0 {
			qds |= 0x80
		}
		b := make([]byte, 0, 15)
		b = append(b, ioa...)
		b = append(b, byte(counter), byte(counter>>8), byte(counter>>16), byte(counter>>24))
		b = append(b, qds)
		b = append(b, ts...)
		return M_IT_TB_1, b, nil
	}
	return 0, nil, fmt.Errorf("unsupported IEC 104 type: %q", p.TypeID)
}

func ioaBytes(ioa int) []byte {
	return []byte{byte(ioa), byte(ioa >> 8), byte(ioa >> 16)}
}

func float32LE(v float64) []byte {
	bits := math.Float32bits(float32(v))
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, bits)
	return b
}

func int16LE(v float64) []byte {
	n := int16(v)
	return []byte{byte(n), byte(n >> 8)}
}

// cp56Time2a: 7-byte millisecond timestamp (IEC 60870-5-4 §6.8).
func cp56Time2a(t time.Time) []byte {
	b := make([]byte, 7)
	ms := uint16(t.Second()*1000 + t.Nanosecond()/1_000_000)
	binary.LittleEndian.PutUint16(b[0:2], ms)
	b[2] = byte(t.Minute()) & 0x3F
	b[3] = byte(t.Hour()) & 0x1F
	dow := int(t.Weekday())
	if dow == 0 {
		dow = 7
	}
	b[4] = (byte(t.Day()) & 0x1F) | byte(dow<<5)
	b[5] = byte(t.Month()) & 0x0F
	b[6] = byte(t.Year()%100) & 0x7F
	return b
}
