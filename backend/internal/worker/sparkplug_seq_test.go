package worker

import (
	"sync/atomic"
	"testing"
	"time"

	"goGateway/internal/sparkplug"
)

// ── minimal Sparkplug B protobuf encoder (test-local) ────────────────────────
// Field numbers mirror internal/sparkplug/payload.go's decoder:
//   Payload: 1=timestamp, 2=metrics (repeated), 3=seq
//   Metric:  1=name, 2=alias, 4=datatype, 10=int_value

func pVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

func pFieldVarint(buf []byte, fn uint32, v uint64) []byte {
	buf = pVarint(buf, uint64(fn)<<3|0) // wire type 0 (varint)
	return pVarint(buf, v)
}

func pFieldBytes(buf []byte, fn uint32, b []byte) []byte {
	buf = pVarint(buf, uint64(fn)<<3|2) // wire type 2 (len-delimited)
	buf = pVarint(buf, uint64(len(b)))
	return append(buf, b...)
}

func pFieldStr(buf []byte, fn uint32, s string) []byte {
	return pFieldBytes(buf, fn, []byte(s))
}

// namedMetric builds a metric carrying an explicit name (so ResolveName needs
// no alias map) plus a trivial Int32 value.
func namedMetric(name string) []byte {
	var m []byte
	m = pFieldStr(m, 1, name)
	m = pFieldVarint(m, 4, 3) // datatype = Int32
	m = pFieldVarint(m, 10, 1) // int_value = 1
	return m
}

// frame builds a payload with the given seq and metrics.
func frame(seq uint64, metrics ...[]byte) []byte {
	now := uint64(time.Now().UnixMilli())
	var p []byte
	p = pFieldVarint(p, 1, now)
	for _, m := range metrics {
		p = pFieldBytes(p, 2, m)
	}
	p = pFieldVarint(p, 3, seq)
	return p
}

// noopDispatcher satisfies Dispatcher; it is never expected to be called in
// these tests because the empty MappingCache yields no IEC-104 mappings.
type noopDispatcher struct{}

func (noopDispatcher) Dispatch(_ TopicMapping, _ float64, _ int, _ time.Time) {}

func newSeqTestHandler() (*SparkplugHandler, *int32) {
	h := NewSparkplugHandler(sparkplug.NewRegistry(), NewMappingCache(nil), noopDispatcher{})
	var rebirths int32
	h.SetRebirthFn(func(_, _ string) { atomic.AddInt32(&rebirths, 1) })
	return h, &rebirths
}

func topicOf(mt sparkplug.MessageType, device string) sparkplug.Topic {
	return sparkplug.Topic{GroupID: "g", MsgType: mt, EdgeNodeID: "n", DeviceID: device}
}

// TestSeq_SingleNodeCounterAcrossDeviceMessages is the regression test for the
// Sparkplug B sequence-tracking fix. Sparkplug maintains ONE seq per EoN node,
// shared by NBIRTH/NDATA/DBIRTH/DDATA. The previous code kept a separate
// per-device counter, so the first NDATA after a DBIRTH (which consumed a node
// seq number) always looked out-of-sequence → endless rebirth storm and DDATA
// never dispatching. This test drives the producer's real interleaving and
// asserts NO rebirth is requested while sequence numbers are contiguous.
func TestSeq_SingleNodeCounterAcrossDeviceMessages(t *testing.T) {
	h, rebirths := newSeqTestHandler()

	// Producer's single shared counter: NBIRTH(0), DBIRTH(1), NDATA(2),
	// DDATA(3), NDATA(4), DDATA(5).
	h.Dispatch(topicOf(sparkplug.MsgNBIRTH, ""), frame(0, namedMetric("nm")))
	h.Dispatch(topicOf(sparkplug.MsgDBIRTH, "d"), frame(1, namedMetric("dm")))
	h.Dispatch(topicOf(sparkplug.MsgNDATA, ""), frame(2, namedMetric("nm")))
	h.Dispatch(topicOf(sparkplug.MsgDDATA, "d"), frame(3, namedMetric("dm")))
	h.Dispatch(topicOf(sparkplug.MsgNDATA, ""), frame(4, namedMetric("nm")))
	h.Dispatch(topicOf(sparkplug.MsgDDATA, "d"), frame(5, namedMetric("dm")))

	if got := atomic.LoadInt32(rebirths); got != 0 {
		t.Fatalf("contiguous NBIRTH/DBIRTH/NDATA/DDATA must not request rebirth, got %d", got)
	}
}

// TestSeq_GapRequestsRebirth verifies the detector still fires on a genuine gap.
func TestSeq_GapRequestsRebirth(t *testing.T) {
	h, rebirths := newSeqTestHandler()

	h.Dispatch(topicOf(sparkplug.MsgNBIRTH, ""), frame(0, namedMetric("nm")))
	h.Dispatch(topicOf(sparkplug.MsgDBIRTH, "d"), frame(1, namedMetric("dm")))
	h.Dispatch(topicOf(sparkplug.MsgNDATA, ""), frame(2, namedMetric("nm")))
	// Skip seq 3 → next expected is 3, send 5 instead.
	h.Dispatch(topicOf(sparkplug.MsgDDATA, "d"), frame(5, namedMetric("dm")))

	if got := atomic.LoadInt32(rebirths); got != 1 {
		t.Fatalf("a seq gap on DDATA must request exactly one rebirth, got %d", got)
	}
}

// TestSeq_DDATAValidatesAgainstNodeNotDevice pins the specific fix: a DDATA must
// be validated against the node's running seq, not a device-local counter. Here
// the node seq has advanced to 4 via NDATA, so the next DDATA is seq 5 — even
// though this is the device's *first* DDATA. The old per-device counter expected
// the device's "next" to be DBIRTH_seq+1 and would have rebirthed.
func TestSeq_DDATAValidatesAgainstNodeNotDevice(t *testing.T) {
	h, rebirths := newSeqTestHandler()

	h.Dispatch(topicOf(sparkplug.MsgNBIRTH, ""), frame(0, namedMetric("nm")))
	h.Dispatch(topicOf(sparkplug.MsgDBIRTH, "d"), frame(1, namedMetric("dm")))
	h.Dispatch(topicOf(sparkplug.MsgNDATA, ""), frame(2, namedMetric("nm")))
	h.Dispatch(topicOf(sparkplug.MsgNDATA, ""), frame(3, namedMetric("nm")))
	h.Dispatch(topicOf(sparkplug.MsgNDATA, ""), frame(4, namedMetric("nm")))
	// Device's first DDATA, but at the node-level seq 5.
	h.Dispatch(topicOf(sparkplug.MsgDDATA, "d"), frame(5, namedMetric("dm")))

	if got := atomic.LoadInt32(rebirths); got != 0 {
		t.Fatalf("device's first DDATA at node seq 5 must validate, got %d rebirth(s)", got)
	}
}
