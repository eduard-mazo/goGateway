// Package worker subscribes to MQTT topics, parses incoming JSON payloads, and
// dispatches decoded IEC-104 points to the slave manager.
package worker

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"goGateway/internal/iec104"
)

// ParseAndDispatch = core hot path.
//
// topic = MQTT topic delivered on.
// payload = raw JSON bytes.
// mappings = cached resolved mappings for this topic.
//
// Quality is resolved in two tiers:
//  1. Payload-level: a top-level "quality" key applies to all signals in the
//     message (integer QDS byte or string "GOOD"/"BAD"/"UNCERTAIN"/…).
//  2. Per-signal: if the mapping has a non-empty QualityKey, that JSON key is
//     read instead and overrides the payload-level quality for that signal.
func ParseAndDispatch(
	topic string,
	payload []byte,
	mappings []TopicMapping,
	srv iec104.Server,
	hist *HistoryLogger,
) {
	if len(mappings) == 0 {
		return
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		log.Printf("parse: topic=%s bad json: %v", topic, err)
		return
	}

	// Timestamp: prefer payload "date" (RFC3339), else now.
	ts := time.Now()
	if d, ok := raw["date"]; ok {
		var s string
		if json.Unmarshal(d, &s) == nil {
			if parsed, err := time.Parse(time.RFC3339, s); err == nil {
				ts = parsed
			}
		}
	}

	// Payload-level quality (tier 1): applies to all signals unless overridden.
	payloadQuality := iec104.QualityGood
	if qr, ok := raw["quality"]; ok {
		payloadQuality = parseMQTTQuality(qr)
	}

	for _, m := range mappings {
		rv, ok := raw[m.JSONKey]
		if !ok {
			continue
		}
		val, err := jsonNumber(rv)
		if err != nil {
			log.Printf("parse: topic=%s key=%s: %v", topic, m.JSONKey, err)
			continue
		}
		scaled := val * m.Scale

		// Per-signal quality (tier 2) overrides payload-level quality.
		quality := payloadQuality
		if m.QualityKey != "" {
			if qr, ok := raw[m.QualityKey]; ok {
				quality = parseMQTTQuality(qr)
			}
		}

		srv.Dispatch(m.ServerID, iec104.Point{
			IOA:       m.IOA,
			TypeID:    m.IEC104Type,
			Value:     scaled,
			Quality:   quality,
			Timestamp: ts,
		})
		if !hist.Log(HistoryEvent{
			MappingID: m.MappingID,
			SignalKey: m.SignalKey,
			Value:     scaled,
			Quality:   quality,
			Timestamp: ts,
		}) {
			log.Printf("history buffer full, dropped %s", m.SignalKey)
		}
	}
}

// parseMQTTQuality converts an MQTT JSON quality value to an IEC 60870-5 QDS
// byte. Integers are used directly (masked to defined bits). Strings are mapped
// by common SCADA conventions.
func parseMQTTQuality(raw json.RawMessage) int {
	const validBits = iec104.QualityInvalid | iec104.QualityNotTopical |
		iec104.QualitySubstituted | iec104.QualityBlocked

	var n int
	if json.Unmarshal(raw, &n) == nil {
		return n & validBits
	}

	var s string
	if json.Unmarshal(raw, &s) == nil {
		switch strings.ToUpper(strings.TrimSpace(s)) {
		case "GOOD", "OK", "VALID":
			return iec104.QualityGood
		case "BAD", "INVALID", "FAILURE", "ERROR":
			return iec104.QualityInvalid
		case "UNCERTAIN", "QUESTIONABLE", "STALE":
			return iec104.QualityNotTopical
		case "SUBSTITUTED":
			return iec104.QualitySubstituted
		case "BLOCKED":
			return iec104.QualityBlocked
		}
	}
	return iec104.QualityGood
}

// jsonNumber = extract float from a JSON number OR numeric string.
func jsonNumber(raw json.RawMessage) (float64, error) {
	// try number first
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f, nil
	}
	// try string-encoded number
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strconv.ParseFloat(s, 64)
	}
	// try bool → 0/1 (useful for single-point)
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		if b {
			return 1, nil
		}
		return 0, nil
	}
	return 0, errNotNumeric
}

var errNotNumeric = &parseErr{msg: "not numeric"}

type parseErr struct{ msg string }

func (e *parseErr) Error() string { return e.msg }
