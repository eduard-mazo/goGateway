// Package sparkplug implements the Sparkplug™ B MQTT topic namespace and
// payload codec (Eclipse Sparkplug Specification Rev 2.2).
//
// The decoder is a hand-rolled protobuf wire-format parser so the build
// requires no protoc toolchain and no external protobuf library.
package sparkplug

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"goGateway/internal/iec104"
)

// ─── Sparkplug B datatype constants (Appendix 1 §15.2.1) ────────────────────

const (
	DtUnknown  uint32 = 0
	DtInt8     uint32 = 1
	DtInt16    uint32 = 2
	DtInt32    uint32 = 3
	DtInt64    uint32 = 4
	DtUInt8    uint32 = 5
	DtUInt16   uint32 = 6
	DtUInt32   uint32 = 7
	DtUInt64   uint32 = 8
	DtFloat    uint32 = 9
	DtDouble   uint32 = 10
	DtBoolean  uint32 = 11
	DtString   uint32 = 12
	DtDateTime uint32 = 13
	DtText     uint32 = 14
)

// ─── Decoded types ───────────────────────────────────────────────────────────

// Payload is a decoded Sparkplug B protobuf payload.
type Payload struct {
	Timestamp  uint64            // milliseconds since Unix epoch (UTC)
	Seq        uint64            // 0–255 wrapping sequence number
	Metrics    []Metric
	Properties map[string]string // field 9: node-level PropertySet (entity_type, fiware_entity_id, …)
}

// Metric is one decoded entry from a Sparkplug B payload.
// Only the fields relevant to numeric SCADA dispatch are decoded;
// DataSet / Template / bytes values are skipped.
type Metric struct {
	Name         string
	Alias        uint64
	HasAlias     bool
	Timestamp    uint64
	Datatype     uint32
	IsHistorical bool
	IsTransient  bool
	IsNull       bool
	StringValue  string
	Properties   map[string]string // field 9: per-metric PropertySet (engUnit, tipo_variable, …)

	// numeric storage — set by the oneof fields 10–14
	uintVal  uint64
	fltVal   float64
	dblVal   float64
	boolVal  bool
	hasUint  bool
	hasFlt   bool
	hasDbl   bool
	hasBool  bool
	hasStr   bool
}

// Float64 extracts the metric value as float64.
// Returns (0, false) when the metric is null or carries no numeric value.
func (m *Metric) Float64() (float64, bool) {
	if m.IsNull {
		return 0, false
	}
	switch {
	case m.hasDbl:
		return m.dblVal, true
	case m.hasFlt:
		return m.fltVal, true
	case m.hasUint:
		return float64(m.uintVal), true
	case m.hasBool:
		if m.boolVal {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// IEC104Quality maps Sparkplug B metric flags to an IEC 60870-5-104 QDS byte.
//
//	is_null       → QualityInvalid   (IV bit)
//	is_historical → QualityNotTopical (NT bit — value is stale/batched)
//	online node   → QualityGood
func (m *Metric) IEC104Quality() int {
	if m.IsNull {
		return iec104.QualityInvalid
	}
	if m.IsHistorical {
		return iec104.QualityNotTopical
	}
	return iec104.QualityGood
}

// ─── Decoder ─────────────────────────────────────────────────────────────────

// DecodePayload decodes a Sparkplug B protobuf binary payload.
func DecodePayload(data []byte) (*Payload, error) {
	p := &Payload{}
	if err := parsePayload(data, p); err != nil {
		return nil, err
	}
	return p, nil
}

// ─── Monitor JSON serialisation ──────────────────────────────────────────────

// metricView is a JSON-serialisable snapshot of one Metric for monitor display.
type metricView struct {
	Name  string      `json:"name"`
	Alias uint64      `json:"alias,omitempty"`
	Dt    uint32      `json:"dt"`
	Ts    uint64      `json:"ts,omitempty"`
	Null  bool        `json:"null,omitempty"`
	Hist  bool        `json:"hist,omitempty"`
	V     interface{} `json:"v"`
}

// payloadView is a JSON-serialisable snapshot of a Payload for monitor display.
type payloadView struct {
	Ts      uint64       `json:"ts"`
	Seq     uint64       `json:"seq"`
	Metrics []metricView `json:"metrics"`
}

// ToJSON returns a compact JSON encoding of the decoded payload for human-
// readable display in the broker monitor. Returns (nil, err) on failure.
func (p *Payload) ToJSON() ([]byte, error) {
	pv := payloadView{
		Ts:      p.Timestamp,
		Seq:     p.Seq,
		Metrics: make([]metricView, len(p.Metrics)),
	}
	for i := range p.Metrics {
		m := &p.Metrics[i]
		mv := metricView{
			Name: m.Name,
			Dt:   m.Datatype,
			Ts:   m.Timestamp,
			Null: m.IsNull,
			Hist: m.IsHistorical,
		}
		if m.HasAlias {
			mv.Alias = m.Alias
		}
		switch {
		case m.hasDbl:
			mv.V = m.dblVal
		case m.hasFlt:
			mv.V = m.fltVal
		case m.hasUint:
			mv.V = m.uintVal
		case m.hasBool:
			mv.V = m.boolVal
		case m.hasStr:
			mv.V = m.StringValue
		}
		pv.Metrics[i] = mv
	}
	return json.Marshal(pv)
}

// ─── Encoder (outgoing messages) ─────────────────────────────────────────────

// EncodeNCMDRebirth returns a protobuf-encoded NCMD payload carrying a single
// boolean metric "Node Control/Rebirth" = true.  Used to request that an EoN
// node re-publish its NBIRTH when the gateway detects an out-of-sequence NDATA.
func EncodeNCMDRebirth() []byte {
	now := uint64(time.Now().UnixMilli())

	// Build the metric sub-message.
	var metric []byte
	metric = appendStrField(metric, 1, "Node Control/Rebirth") // name
	metric = appendVarintField(metric, 3, now)                  // timestamp
	metric = appendVarintField(metric, 4, uint64(DtBoolean))    // datatype
	metric = appendVarintField(metric, 14, 1)                   // boolean_value = true

	// Build the payload.
	var buf []byte
	buf = appendVarintField(buf, 1, now) // timestamp
	buf = appendBytesField(buf, 2, metric) // metrics[0]
	buf = appendVarintField(buf, 3, 0)   // seq = 0
	return buf
}

// ─── Protobuf wire-format helpers ────────────────────────────────────────────

const (
	wireVarint = 0
	wire64bit  = 1
	wireLenDel = 2
	wire32bit  = 5
)

func parseVarint(b []byte, pos int) (uint64, int, error) {
	var v uint64
	var shift uint
	for {
		if pos >= len(b) {
			return 0, pos, fmt.Errorf("sparkplug: varint: unexpected EOF at byte %d", pos)
		}
		byt := b[pos]
		pos++
		v |= uint64(byt&0x7f) << shift
		if byt < 0x80 {
			return v, pos, nil
		}
		shift += 7
		if shift >= 64 {
			return 0, pos, fmt.Errorf("sparkplug: varint: overflow")
		}
	}
}

func parseTag(b []byte, pos int) (fieldNum uint32, wireType int, newPos int, err error) {
	v, p, e := parseVarint(b, pos)
	if e != nil {
		return 0, 0, pos, e
	}
	return uint32(v >> 3), int(v & 0x7), p, nil
}

func parseLenDelim(b []byte, pos int) ([]byte, int, error) {
	length, p, err := parseVarint(b, pos)
	if err != nil {
		return nil, pos, err
	}
	end := p + int(length)
	if end > len(b) {
		return nil, pos, fmt.Errorf("sparkplug: length-delimited field truncated (need %d, have %d)", end, len(b))
	}
	return b[p:end], end, nil
}

func skipField(b []byte, pos, wireType int) (int, error) {
	switch wireType {
	case wireVarint:
		_, p, err := parseVarint(b, pos)
		return p, err
	case wire64bit:
		if pos+8 > len(b) {
			return pos, fmt.Errorf("sparkplug: skip 64-bit: truncated")
		}
		return pos + 8, nil
	case wireLenDel:
		_, p, err := parseLenDelim(b, pos)
		return p, err
	case wire32bit:
		if pos+4 > len(b) {
			return pos, fmt.Errorf("sparkplug: skip 32-bit: truncated")
		}
		return pos + 4, nil
	default:
		return pos, fmt.Errorf("sparkplug: unknown wire type %d", wireType)
	}
}

func parsePayload(data []byte, p *Payload) error {
	pos := 0
	for pos < len(data) {
		fn, wt, np, err := parseTag(data, pos)
		if err != nil {
			return err
		}
		pos = np
		switch fn {
		case 1: // timestamp (uint64 varint)
			if wt != wireVarint {
				pos, err = skipField(data, pos, wt)
				break
			}
			p.Timestamp, pos, err = parseVarint(data, pos)
		case 2: // metrics (repeated length-delimited)
			if wt != wireLenDel {
				pos, err = skipField(data, pos, wt)
				break
			}
			var raw []byte
			raw, pos, err = parseLenDelim(data, pos)
			if err != nil {
				return err
			}
			var m Metric
			if e2 := parseMetric(raw, &m); e2 != nil {
				return e2
			}
			p.Metrics = append(p.Metrics, m)
		case 3: // seq (uint64 varint)
			if wt != wireVarint {
				pos, err = skipField(data, pos, wt)
				break
			}
			p.Seq, pos, err = parseVarint(data, pos)
		case 9: // properties (PropertySet, length-delimited)
			if wt != wireLenDel {
				pos, err = skipField(data, pos, wt)
				break
			}
			var raw []byte
			raw, pos, err = parseLenDelim(data, pos)
			if err == nil {
				p.Properties = parsePropertySetStrings(raw)
			}
		default:
			pos, err = skipField(data, pos, wt)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func parseMetric(data []byte, m *Metric) error {
	pos := 0
	for pos < len(data) {
		fn, wt, np, err := parseTag(data, pos)
		if err != nil {
			return err
		}
		pos = np
		switch fn {
		case 1: // name (string)
			if wt != wireLenDel {
				pos, err = skipField(data, pos, wt)
				break
			}
			var raw []byte
			raw, pos, err = parseLenDelim(data, pos)
			if err == nil {
				m.Name = string(raw)
			}
		case 2: // alias (uint64 varint)
			if wt != wireVarint {
				pos, err = skipField(data, pos, wt)
				break
			}
			m.Alias, pos, err = parseVarint(data, pos)
			if err == nil {
				m.HasAlias = true
			}
		case 3: // timestamp (uint64 varint)
			if wt != wireVarint {
				pos, err = skipField(data, pos, wt)
				break
			}
			m.Timestamp, pos, err = parseVarint(data, pos)
		case 4: // datatype (uint32 varint)
			if wt != wireVarint {
				pos, err = skipField(data, pos, wt)
				break
			}
			var v uint64
			v, pos, err = parseVarint(data, pos)
			if err == nil {
				m.Datatype = uint32(v)
			}
		case 5: // is_historical (bool varint)
			if wt != wireVarint {
				pos, err = skipField(data, pos, wt)
				break
			}
			var v uint64
			v, pos, err = parseVarint(data, pos)
			if err == nil {
				m.IsHistorical = v != 0
			}
		case 6: // is_transient (bool varint)
			if wt != wireVarint {
				pos, err = skipField(data, pos, wt)
				break
			}
			var v uint64
			v, pos, err = parseVarint(data, pos)
			if err == nil {
				m.IsTransient = v != 0
			}
		case 7: // is_null (bool varint)
			if wt != wireVarint {
				pos, err = skipField(data, pos, wt)
				break
			}
			var v uint64
			v, pos, err = parseVarint(data, pos)
			if err == nil {
				m.IsNull = v != 0
			}
		case 8: // metadata — skip
			pos, err = skipField(data, pos, wt)
		case 9: // properties (PropertySet)
			if wt != wireLenDel {
				pos, err = skipField(data, pos, wt)
				break
			}
			var raw []byte
			raw, pos, err = parseLenDelim(data, pos)
			if err == nil {
				m.Properties = parsePropertySetStrings(raw)
			}
		case 10: // int_value (uint32 → stored as uint64)
			if wt != wireVarint {
				pos, err = skipField(data, pos, wt)
				break
			}
			m.uintVal, pos, err = parseVarint(data, pos)
			if err == nil {
				m.hasUint = true
			}
		case 11: // long_value (uint64)
			if wt != wireVarint {
				pos, err = skipField(data, pos, wt)
				break
			}
			m.uintVal, pos, err = parseVarint(data, pos)
			if err == nil {
				m.hasUint = true
			}
		case 12: // float_value (32-bit fixed)
			if wt != wire32bit {
				pos, err = skipField(data, pos, wt)
				break
			}
			if pos+4 > len(data) {
				return fmt.Errorf("sparkplug: float_value: truncated")
			}
			bits := binary.LittleEndian.Uint32(data[pos:])
			m.fltVal = float64(math.Float32frombits(bits))
			m.hasFlt = true
			pos += 4
		case 13: // double_value (64-bit fixed)
			if wt != wire64bit {
				pos, err = skipField(data, pos, wt)
				break
			}
			if pos+8 > len(data) {
				return fmt.Errorf("sparkplug: double_value: truncated")
			}
			bits := binary.LittleEndian.Uint64(data[pos:])
			m.dblVal = math.Float64frombits(bits)
			m.hasDbl = true
			pos += 8
		case 14: // boolean_value (varint 0/1)
			if wt != wireVarint {
				pos, err = skipField(data, pos, wt)
				break
			}
			var v uint64
			v, pos, err = parseVarint(data, pos)
			if err == nil {
				m.boolVal = v != 0
				m.hasBool = true
			}
		case 15: // string_value
			if wt != wireLenDel {
				pos, err = skipField(data, pos, wt)
				break
			}
			var raw []byte
			raw, pos, err = parseLenDelim(data, pos)
			if err == nil {
				m.StringValue = string(raw)
				m.hasStr = true
			}
		default: // bytes_value (16), dataset_value (17), template_value (18) — skip
			pos, err = skipField(data, pos, wt)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// parsePropertySetStrings decodes a Sparkplug B PropertySet message and returns
// a map of key → string value. Non-string PropertyValues are silently skipped.
//
// Wire layout (from sparkplug_b.proto):
//
//	message PropertySet {
//	    repeated string        keys   = 1;  // parallel arrays, same index
//	    repeated PropertyValue values = 2;
//	}
//	message PropertyValue {
//	    uint32 type         = 1;
//	    ...
//	    string string_value = 8;  // field 8 inside PropertyValue
//	}
func parsePropertySetStrings(data []byte) map[string]string {
	var keys []string
	var vals []string // one entry per PropertyValue, "" if not a string

	pos := 0
	for pos < len(data) {
		fn, wt, np, err := parseTag(data, pos)
		if err != nil {
			break
		}
		pos = np
		switch fn {
		case 1: // key (string)
			if wt != wireLenDel {
				pos, _ = skipField(data, pos, wt)
				continue
			}
			raw, np2, e := parseLenDelim(data, pos)
			if e != nil {
				break
			}
			pos = np2
			keys = append(keys, string(raw))
		case 2: // PropertyValue (length-delimited sub-message)
			if wt != wireLenDel {
				pos, _ = skipField(data, pos, wt)
				continue
			}
			raw, np2, e := parseLenDelim(data, pos)
			if e != nil {
				break
			}
			pos = np2
			vals = append(vals, parsePropertyValueString(raw))
		default:
			pos, _ = skipField(data, pos, wt)
		}
	}

	if len(keys) == 0 {
		return nil
	}
	m := make(map[string]string, len(keys))
	for i, k := range keys {
		if i < len(vals) {
			m[k] = vals[i]
		} else {
			m[k] = ""
		}
	}
	return m
}

// parsePropertyValueString extracts a PropertyValue's value as a string,
// reading whichever typed value field is present and stringifying it:
//
//	int_value    (3, uint32)  → decimal       e.g. "192"
//	long_value   (4, uint64)  → decimal
//	float_value  (5, 32-bit)  → shortest float
//	double_value (6, 64-bit)  → shortest float
//	boolean_value(7, varint)  → "true" / "false"
//	string_value (8, string)  → as-is
//
// Producers (e.g. the goMqttDnp3 gateway) carry numeric metadata such as the
// SCADA `quality` code and `dnp3.*` flags as typed PropertyValues, not strings;
// reading only string_value (field 8) silently dropped them. Returns "" when the
// value is null/absent.
func parsePropertyValueString(data []byte) string {
	pos := 0
	for pos < len(data) {
		fn, wt, np, err := parseTag(data, pos)
		if err != nil {
			break
		}
		pos = np
		switch fn {
		case 3, 4: // int_value / long_value (varint)
			if wt != wireVarint {
				pos, _ = skipField(data, pos, wt)
				continue
			}
			v, _, e := parseVarint(data, pos)
			if e != nil {
				return ""
			}
			return strconv.FormatUint(v, 10)
		case 5: // float_value (32-bit fixed)
			if wt != wire32bit || pos+4 > len(data) {
				pos, _ = skipField(data, pos, wt)
				continue
			}
			bits := binary.LittleEndian.Uint32(data[pos:])
			return strconv.FormatFloat(float64(math.Float32frombits(bits)), 'g', -1, 32)
		case 6: // double_value (64-bit fixed)
			if wt != wire64bit || pos+8 > len(data) {
				pos, _ = skipField(data, pos, wt)
				continue
			}
			bits := binary.LittleEndian.Uint64(data[pos:])
			return strconv.FormatFloat(math.Float64frombits(bits), 'g', -1, 64)
		case 7: // boolean_value (varint)
			if wt != wireVarint {
				pos, _ = skipField(data, pos, wt)
				continue
			}
			v, _, e := parseVarint(data, pos)
			if e != nil {
				return ""
			}
			if v != 0 {
				return "true"
			}
			return "false"
		case 8: // string_value
			if wt != wireLenDel {
				pos, _ = skipField(data, pos, wt)
				continue
			}
			raw, _, e := parseLenDelim(data, pos)
			if e != nil {
				return ""
			}
			return string(raw)
		default: // type (1), is_null (2), property_set (9), …
			pos, _ = skipField(data, pos, wt)
		}
	}
	return ""
}

// ─── Encoder helpers ─────────────────────────────────────────────────────────

func appendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

func appendVarintField(buf []byte, fieldNum uint32, v uint64) []byte {
	tag := uint64(fieldNum)<<3 | wireVarint
	buf = appendVarint(buf, tag)
	return appendVarint(buf, v)
}

func appendStrField(buf []byte, fieldNum uint32, s string) []byte {
	tag := uint64(fieldNum)<<3 | wireLenDel
	buf = appendVarint(buf, tag)
	buf = appendVarint(buf, uint64(len(s)))
	return append(buf, s...)
}

func appendBytesField(buf []byte, fieldNum uint32, data []byte) []byte {
	tag := uint64(fieldNum)<<3 | wireLenDel
	buf = appendVarint(buf, tag)
	buf = appendVarint(buf, uint64(len(data)))
	return append(buf, data...)
}
