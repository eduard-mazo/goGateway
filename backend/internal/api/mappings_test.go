package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"goGateway/internal/db"
	"goGateway/internal/models"
)

// newTestMappingHandler opens a fresh migrated DB in a temp dir and seeds one
// device + topic on the default iec104_servers row (id=1), returning the handler
// and the topic id to assign against.
func newTestMappingHandler(t *testing.T) (*MappingHandler, int64) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if _, err := d.Exec(`INSERT INTO devices(server_id,name,description) VALUES(1,'edge-1-node','')`); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	var topicID int64
	if err := d.Get(&topicID, `INSERT INTO topics(device_id,topic,qos,enabled,payload_format)
		VALUES((SELECT id FROM devices WHERE name='edge-1-node'),'spBv1.0/EPM_SOAK/edge-1',1,1,'sparkplug') RETURNING id`); err != nil {
		t.Fatalf("seed topic: %v", err)
	}
	return &MappingHandler{DB: d}, topicID
}

// assign posts an assignReq and returns the created mapping.
func doAssign(t *testing.T, h *MappingHandler, body map[string]any) (*httptest.ResponseRecorder, models.SignalMapping) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/mappings/assign", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	h.assign(rec, req)
	var m models.SignalMapping
	if rec.Code == http.StatusCreated {
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
		}
	}
	return rec, m
}

func TestDeriveIEC104Type(t *testing.T) {
	cases := map[string]string{
		"analog": "M_ME_NC_1", "Analog": "M_ME_NC_1", "instantaneo": "M_ME_NC_1",
		"acumulado": "M_ME_NC_1", "digital": "M_SP_NA_1", "bool": "M_SP_NA_1",
		"": "", "weird": "",
	}
	for in, want := range cases {
		if got := deriveIEC104Type(in); got != want {
			t.Errorf("deriveIEC104Type(%q)=%q, want %q", in, got, want)
		}
	}
}

// TestAssign_DerivesTypeAndAutoIOA: a minimal assignment (topic + metric +
// variable_type) derives server_id, the wire type, and the next free IOA, and
// enables the point — without the caller touching any non-wire field.
func TestAssign_DerivesTypeAndAutoIOA(t *testing.T) {
	h, topicID := newTestMappingHandler(t)

	rec, m := doAssign(t, h, map[string]any{
		"topic_id": topicID, "metric_name": "Medidas/Energy_kWh",
		"variable_type": "analog", "unit": "kWh",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("assign: want 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	if m.ServerID != 1 {
		t.Errorf("server_id: want 1 (derived from topic), got %d", m.ServerID)
	}
	if m.IEC104Type != "M_ME_NC_1" {
		t.Errorf("iec104_type: want M_ME_NC_1 (derived), got %q", m.IEC104Type)
	}
	if m.IOA != 1 {
		t.Errorf("ioa: want 1 (first auto-assigned), got %d", m.IOA)
	}
	if !m.Enabled {
		t.Errorf("assigned point should be enabled")
	}
	if m.Scale != 1.0 {
		t.Errorf("scale: want default 1.0, got %v", m.Scale)
	}
	// Non-wire fields stay at their DB defaults — only the necessary fields are set.
	if m.Business != "" || m.Company != "" || m.JSONKey != "" || m.DeviceName != "" {
		t.Errorf("non-wire fields should be empty defaults, got %+v", m)
	}

	// Second assignment auto-increments the IOA on the same server.
	_, m2 := doAssign(t, h, map[string]any{
		"topic_id": topicID, "metric_name": "Medidas/Power_kW", "variable_type": "digital",
	})
	if m2.IOA != 2 {
		t.Errorf("second ioa: want 2, got %d", m2.IOA)
	}
	if m2.IEC104Type != "M_SP_NA_1" {
		t.Errorf("digital type: want M_SP_NA_1, got %q", m2.IEC104Type)
	}
}

// TestAssign_ExplicitIOAAndType: explicit values are respected over derivation.
func TestAssign_ExplicitIOAAndType(t *testing.T) {
	h, topicID := newTestMappingHandler(t)
	_, m := doAssign(t, h, map[string]any{
		"topic_id": topicID, "metric_name": "PLC/tank_level",
		"iec104_type": "M_ME_TF_1", "ioa": 5000, "scale": 0.1,
	})
	if m.IOA != 5000 || m.IEC104Type != "M_ME_TF_1" || m.Scale != 0.1 {
		t.Fatalf("explicit values not respected: %+v", m)
	}
}

// TestAssign_Validation: missing essentials are rejected with 400.
func TestAssign_Validation(t *testing.T) {
	h, topicID := newTestMappingHandler(t)
	cases := []map[string]any{
		{"metric_name": "x", "variable_type": "analog"},                 // no topic_id
		{"topic_id": topicID, "variable_type": "analog"},                // no metric_name
		{"topic_id": topicID, "metric_name": "x"},                       // no type and no variable_type
		{"topic_id": topicID, "metric_name": "x", "variable_type": "?"}, // underivable type
	}
	for i, body := range cases {
		rec, _ := doAssign(t, h, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("case %d: want 400, got %d (%s)", i, rec.Code, rec.Body.String())
		}
	}
	// Unknown topic id → 400.
	rec, _ := doAssign(t, h, map[string]any{"topic_id": 9999, "metric_name": "x", "variable_type": "analog"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown topic: want 400, got %d", rec.Code)
	}
}
