// Package fiware is a minimal FIWARE Orion (NGSIv2) sink for decoded SSFV
// samples. It is consumed by worker.OrionWorker off the NATS fan-out spine, so
// Orion availability is decoupled from MQTT ingestion and the TimescaleDB write.
package fiware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Reading is one decoded sample to upsert into Orion. Kept package-local so
// fiware does not import worker (avoids an import cycle).
type Reading struct {
	Entity   string // group/node[/device]
	Codigo   string // attribute name (codigo_senal)
	Instance string // folder path or "default"
	Value    float64
	Quality  int
	Time     time.Time
}

// OrionClient upserts a reading into a FIWARE context broker.
type OrionClient interface {
	Upsert(ctx context.Context, r Reading) error
}

// HTTPClient talks NGSIv2 to an Orion context broker via the batch update op
// (POST /v2/op/update, actionType "append") — create-or-update, i.e. idempotent.
type HTTPClient struct {
	base        string // e.g. http://localhost:1026
	entityType  string
	service     string // Fiware-Service tenant header (optional)
	servicePath string // Fiware-ServicePath header (optional)
	hc          *http.Client
}

// NewHTTPClient builds a client. entityType defaults to "SSFVEquipo".
func NewHTTPClient(base, entityType, service, servicePath string) *HTTPClient {
	if entityType == "" {
		entityType = "SSFVEquipo"
	}
	return &HTTPClient{
		base:        strings.TrimRight(base, "/"),
		entityType:  entityType,
		service:     service,
		servicePath: servicePath,
		hc:          &http.Client{Timeout: 10 * time.Second},
	}
}

// EntityID maps an SSFV entity topic (group/node[/device]) to a stable NGSI URN:
// "urn:ngsi-ld:<type>:<group>:<node>[:<device>]". '/' → ':' keeps it within the
// NGSIv2 id character rules (which forbid '/').
func (c *HTTPClient) EntityID(entity string) string {
	return "urn:ngsi-ld:" + c.entityType + ":" + strings.ReplaceAll(entity, "/", ":")
}

func (c *HTTPClient) Upsert(ctx context.Context, r Reading) error {
	attr := map[string]any{
		"type":  "Number",
		"value": r.Value,
		"metadata": map[string]any{
			"TimeInstant": map[string]any{"type": "DateTime", "value": r.Time.UTC().Format(time.RFC3339Nano)},
			"instance":    map[string]any{"type": "Text", "value": r.Instance},
			"quality":     map[string]any{"type": "Number", "value": r.Quality},
		},
	}
	entity := map[string]any{
		"id":      c.EntityID(r.Entity),
		"type":    c.entityType,
		r.Codigo:  attr,
	}
	body, err := json.Marshal(map[string]any{
		"actionType": "append",
		"entities":   []any{entity},
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v2/op/update", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.service != "" {
		req.Header.Set("Fiware-Service", c.service)
	}
	if c.servicePath != "" {
		req.Header.Set("Fiware-ServicePath", c.servicePath)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("orion %s: HTTP %d: %s", c.base, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
