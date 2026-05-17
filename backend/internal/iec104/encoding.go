package iec104

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

// encodePoint wraps one Point into a single-object ASDU.
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

// encodeInfoObject returns (TypeID byte, info-object bytes, error).
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
		// DPI bits: 0=intermediate, 1=OFF, 2=ON, 3=indeterminate (IEC 60870-5-101 §7.2.6.4)
		dpi := byte(math.Round(p.Value)) & 0x03
		diq |= dpi
		b := make([]byte, 0, 4)
		b = append(b, ioa...)
		b = append(b, diq)
		return M_DP_NA_1, b, nil

	case "M_DP_TB_1":
		diq := q & 0xF0
		dpi := byte(math.Round(p.Value)) & 0x03
		diq |= dpi
		b := make([]byte, 0, 11)
		b = append(b, ioa...)
		b = append(b, diq)
		b = append(b, ts...)
		return M_DP_TB_1, b, nil

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

// packInfoObjects builds one or more ASDUs (SQ=0) carrying objects of a
// single TypeID. Each ASDU stays under the 253-byte APDU payload cap and the
// 7-bit NumObjects limit.
func packInfoObjects(typeID byte, objs [][]byte, cot byte, asduAddr uint16) [][]byte {
	if len(objs) == 0 {
		return nil
	}
	const maxAPDUPayload = 253              // APDU body excl. 0x68+len header
	const maxASDUBody = maxAPDUPayload - 10 // - APCI ctrl (4) - ASDU hdr (6)
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

// wrapIFrame builds an I-format APDU around an ASDU payload.
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

// cp56Time2a encodes a 7-byte millisecond timestamp (IEC 60870-5-4 §6.8).
// Sets IV (byte 2 bit 7) when t is zero — SCADA must not use an invalid timestamp.
// Sets SU (byte 3 bit 7) when t is in DST (summer time).
func cp56Time2a(t time.Time) []byte {
	b := make([]byte, 7)
	if t.IsZero() {
		b[2] |= 0x80 // IV — timestamp invalid
		return b
	}
	ms := uint16(t.Second()*1000 + t.Nanosecond()/1_000_000)
	binary.LittleEndian.PutUint16(b[0:2], ms)
	b[2] = byte(t.Minute()) & 0x3F
	b[3] = byte(t.Hour()) & 0x1F
	if t.IsDST() {
		b[3] |= 0x80 // SU — summer time active
	}
	dow := int(t.Weekday())
	if dow == 0 {
		dow = 7
	}
	b[4] = (byte(t.Day()) & 0x1F) | byte(dow<<5)
	b[5] = byte(t.Month()) & 0x0F
	b[6] = byte(t.Year()%100) & 0x7F
	return b
}
