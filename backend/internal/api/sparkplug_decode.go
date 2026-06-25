package api

import (
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"goGateway/internal/sparkplug"
)

// SparkplugDecodeHandler exposes a stateless decoder for Sparkplug B protobuf
// frames. It is a diagnostic aid: paste a raw payload (captured off the broker,
// e.g. `mosquitto_sub -F '%x'`) and get the decoded metrics back as JSON, using
// the exact same codec the live consumer runs. No DB or broker access.
type SparkplugDecodeHandler struct{}

func (h *SparkplugDecodeHandler) Mount(r chi.Router) {
	r.Post("/decode", h.decode)
}

// decodeReq carries the frame to decode. Supply the payload as either hex
// (`payload_hex`) or standard base64 (`payload_base64`). `topic` is optional —
// when given, the response echoes the parsed topic elements (group/type/node/
// device) so the message scope is unambiguous.
type decodeReq struct {
	Topic         string `json:"topic,omitempty"`
	PayloadHex    string `json:"payload_hex,omitempty"`
	PayloadBase64 string `json:"payload_base64,omitempty"`
}

type decodeTopic struct {
	Group   string `json:"group_id"`
	MsgType string `json:"message_type"`
	Node    string `json:"edge_node_id"`
	Device  string `json:"device_id,omitempty"`
}

type decodeMetric struct {
	Name         string            `json:"name,omitempty"`
	Alias        uint64            `json:"alias,omitempty"`
	Datatype     uint32            `json:"datatype"`
	DatatypeName string            `json:"datatype_name"`
	Value        any               `json:"value,omitempty"`
	IsNull       bool              `json:"is_null,omitempty"`
	IsHistorical bool              `json:"is_historical,omitempty"`
	Properties   map[string]string `json:"properties,omitempty"`
}

type decodeResp struct {
	Topic     *decodeTopic      `json:"topic,omitempty"`
	Timestamp uint64            `json:"timestamp"`
	Seq       uint64            `json:"seq"`
	NodeProps map[string]string `json:"node_properties,omitempty"`
	Count     int               `json:"metric_count"`
	Metrics   []decodeMetric    `json:"metrics"`
}

// datatypeNames maps Sparkplug B datatype codes (Appendix-1 §15.2.1) to labels.
var datatypeNames = map[uint32]string{
	1: "Int8", 2: "Int16", 3: "Int32", 4: "Int64",
	5: "UInt8", 6: "UInt16", 7: "UInt32", 8: "UInt64",
	9: "Float", 10: "Double", 11: "Boolean", 12: "String",
	13: "DateTime", 14: "Text", 15: "UUID",
}

func (h *SparkplugDecodeHandler) decode(w http.ResponseWriter, r *http.Request) {
	var req decodeReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	var raw []byte
	var err error
	switch {
	case req.PayloadHex != "":
		raw, err = hex.DecodeString(strings.TrimSpace(req.PayloadHex))
	case req.PayloadBase64 != "":
		raw, err = base64.StdEncoding.DecodeString(strings.TrimSpace(req.PayloadBase64))
	default:
		writeErr(w, http.StatusBadRequest, "provide payload_hex or payload_base64")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "decode payload bytes: "+err.Error())
		return
	}

	pl, err := sparkplug.DecodePayload(raw)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "decode sparkplug payload: "+err.Error())
		return
	}

	resp := decodeResp{
		Timestamp: pl.Timestamp,
		Seq:       pl.Seq,
		NodeProps: pl.Properties,
		Count:     len(pl.Metrics),
		Metrics:   make([]decodeMetric, 0, len(pl.Metrics)),
	}
	if req.Topic != "" {
		if t, ok := sparkplug.ParseTopic(req.Topic); ok {
			resp.Topic = &decodeTopic{
				Group:   t.GroupID,
				MsgType: string(t.MsgType),
				Node:    t.EdgeNodeID,
				Device:  t.DeviceID,
			}
		}
	}

	for i := range pl.Metrics {
		m := &pl.Metrics[i]
		dm := decodeMetric{
			Name:         m.Name,
			Alias:        m.Alias,
			Datatype:     m.Datatype,
			DatatypeName: datatypeNames[m.Datatype],
			IsNull:       m.IsNull,
			IsHistorical: m.IsHistorical,
			Properties:   m.Properties,
		}
		if !m.IsNull {
			switch m.Datatype {
			case 11: // Boolean
				if v, ok := m.Float64(); ok {
					dm.Value = v != 0
				}
			case 12, 14, 15: // String / Text / UUID
				dm.Value = m.StringValue
			default:
				if v, ok := m.Float64(); ok {
					dm.Value = v
				} else if m.StringValue != "" {
					dm.Value = m.StringValue
				}
			}
		}
		resp.Metrics = append(resp.Metrics, dm)
	}

	writeJSON(w, http.StatusOK, resp)
}
