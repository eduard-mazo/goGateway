package iec104

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sort"
	"sync"
	"time"

	"goGateway/internal/models"
)

// clientConn holds per-connection state for one SCADA master.
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
	t1Start    time.Time // when first unacked I-frame was sent; zero when all acked

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

// send queues an ASDU for transmission as an I-frame. When the outbox is full
// the oldest queued frame is discarded to make room, keeping the SCADA master
// up to date with the most recent values rather than stale ones.
func (cc *clientConn) send(asdu []byte) {
	select {
	case cc.outbox <- asdu:
		return
	case <-cc.closed:
		return
	default:
	}
	// Outbox full — drop oldest, enqueue newest.
	select {
	case <-cc.outbox:
		cc.srv.log.Printf("iec104[%s]: outbox full, dropped oldest I-frame to %s", cc.cfg.Name, cc.conn.RemoteAddr())
	default:
	}
	select {
	case cc.outbox <- asdu:
	default:
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
		cc.srv.log.Printf("iec104[%s]: N(S) gap: got %d expected %d — closing link", cc.cfg.Name, peerNS, expected)
		cc.conn.Close()
		return
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
	case C_CI_NA_1:
		cc.handleCounterInterrogation(cot, body)
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

	// Sort by IOA for deterministic reply order, then bucket by TypeID.
	pts := cc.srv.points.snapshot()
	sort.Slice(pts, func(i, j int) bool { return pts[i].IOA < pts[j].IOA })

	buckets := make(map[byte][][]byte)
	order := make([]byte, 0, 8)
	for _, p := range pts {
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

// handleCounterInterrogation handles C_CI_NA_1: replies with only M_IT_* points.
func (cc *clientConn) handleCounterInterrogation(cot byte, body []byte) {
	if cot != COT_ACTIVATION {
		return
	}
	if len(body) < 4 {
		return
	}
	cc.replyAck(C_CI_NA_1, COT_ACT_CON, body)

	cc.srv.mu.RLock()
	asduAddr := uint16(cc.srv.cfg.ASDUAddr)
	cc.srv.mu.RUnlock()

	pts := cc.srv.points.snapshot()
	sort.Slice(pts, func(i, j int) bool { return pts[i].IOA < pts[j].IOA })

	buckets := make(map[byte][][]byte)
	order := make([]byte, 0, 2)
	for _, p := range pts {
		if p.TypeID != "M_IT_NA_1" && p.TypeID != "M_IT_TB_1" {
			continue
		}
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
		for _, asdu := range packInfoObjects(typeID, buckets[typeID], COT_INTROGEN, asduAddr) {
			cc.send(asdu)
		}
	}
	cc.replyAck(C_CI_NA_1, COT_ACT_TERM, body)
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
			if cc.unacked == 0 {
				cc.t1Start = time.Now() // arm t1 on first unacked frame
			}
			cc.unacked++
			cc.rxSinceAck = 0 // I-frame carries our N(R).
			cc.mu.Unlock()

			cc.writeRaw(wrapIFrame(ns, nr, asdu))
		}
	}
}

// timerLoop flushes pending S-frame acks on t2 and enforces the t1 send
// timeout. Read-deadline already handles t3 from the reader side.
func (cc *clientConn) timerLoop() {
	t1 := timerDuration(cc.cfg.T1, 15*time.Second)
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
			unacked := cc.unacked
			t1Start := cc.t1Start
			cc.mu.Unlock()

			if pending {
				cc.sendSFrame()
			}
			if unacked > 0 && !t1Start.IsZero() && time.Since(t1Start) > t1 {
				cc.srv.log.Printf("iec104[%s]: T1 timeout (%s without ACK), closing %s",
					cc.cfg.Name, t1, cc.conn.RemoteAddr())
				cc.conn.Close()
				return
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
	if cc.unacked == 0 {
		cc.t1Start = time.Time{} // all acked — reset t1 clock
	}
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

func (cc *clientConn) trace(dir, kind string, b []byte) {
	if !debug {
		return
	}
	cc.srv.log.Printf("iec104[%s] %s %s %s %s",
		cc.srv.cfg.Name, dir, cc.conn.RemoteAddr(), kind, hex.EncodeToString(b))
}
