// Package worker subscribes to MQTT topics, parses incoming JSON payloads, and
// dispatches decoded IEC-104 points to the slave manager.
package worker

import (
	"encoding/json"
	"log"
	"strconv"
	"time"

	"goGateway/internal/iec104"
)

// ParseAndDispatch = core hot path.
//
// topic = MQTT topic delivered on.
// payload = raw JSON bytes.
// mappings = cached resolved mappings for this topic.
//
// For each mapping: extract JSON key, coerce to float64, apply scale, push
// to IEC 104 server + history logger. Missing keys skipped silently (payload
// variant per device). Bad types logged once per-msg at debug level.
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

		srv.Dispatch(m.ServerID, iec104.Point{
			IOA:       m.IOA,
			TypeID:    m.IEC104Type,
			Value:     scaled,
			Quality:   iec104.QualityGood,
			Timestamp: ts,
		})
		if !hist.Log(HistoryEvent{
			MappingID: m.MappingID,
			SignalKey: m.SignalKey,
			Value:     scaled,
			Quality:   iec104.QualityGood,
			Timestamp: ts,
		}) {
			// buffer full → drop + warn (throttled later if needed).
			log.Printf("history buffer full, dropped %s", m.SignalKey)
		}
	}
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
