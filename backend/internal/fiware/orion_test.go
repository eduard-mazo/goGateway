package fiware

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClientUpsert(t *testing.T) {
	var gotPath, gotService, gotCT string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotService = r.Header.Get("Fiware-Service")
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL, "", "solar", "/plant1") // empty type → default
	ts := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	err := c.Upsert(context.Background(), Reading{
		Entity: "EPM_SOAK/edge-1/meter-01", Codigo: "Energy_kWh",
		Instance: "Medidas", Value: 42.5, Quality: 192, Time: ts,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if gotPath != "/v2/op/update" {
		t.Errorf("path = %q, want /v2/op/update", gotPath)
	}
	if gotService != "solar" {
		t.Errorf("Fiware-Service = %q, want solar", gotService)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q", gotCT)
	}
	if body["actionType"] != "append" {
		t.Errorf("actionType = %v, want append", body["actionType"])
	}
	ents, _ := body["entities"].([]any)
	if len(ents) != 1 {
		t.Fatalf("entities len = %d, want 1", len(ents))
	}
	e := ents[0].(map[string]any)
	if e["id"] != "urn:ngsi-ld:SSFVEquipo:EPM_SOAK:edge-1:meter-01" {
		t.Errorf("entity id = %v", e["id"])
	}
	if e["type"] != "SSFVEquipo" {
		t.Errorf("entity type = %v, want SSFVEquipo (default)", e["type"])
	}
	attr, ok := e["Energy_kWh"].(map[string]any)
	if !ok {
		t.Fatalf("missing attribute Energy_kWh in %v", e)
	}
	if attr["value"].(float64) != 42.5 {
		t.Errorf("attr value = %v, want 42.5", attr["value"])
	}
	md := attr["metadata"].(map[string]any)
	if md["instance"].(map[string]any)["value"] != "Medidas" {
		t.Errorf("instance metadata = %v", md["instance"])
	}
}

func TestHTTPClientUpsertError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"BadRequest"}`))
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL, "Inversor", "", "")
	err := c.Upsert(context.Background(), Reading{Entity: "g/n", Codigo: "X", Instance: "default", Value: 1})
	if err == nil {
		t.Fatal("expected error on HTTP 400")
	}
	if id := c.EntityID("g/n"); id != "urn:ngsi-ld:Inversor:g:n" {
		t.Errorf("EntityID = %q", id)
	}
}
