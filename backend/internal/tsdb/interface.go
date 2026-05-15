// Package tsdb provides a write pipeline for time-series databases.
// Supports VictoriaMetrics (line protocol over HTTP) and TimescaleDB (pgx COPY).
// Guarantees no data loss via WAL + per-backend retry queues + DLQ.
package tsdb

import (
	"context"
	"time"
)

// DataPoint is a single measurement sample.
type DataPoint struct {
	Measurement string
	Tags        map[string]string
	Fields      map[string]float64
	Timestamp   time.Time
}

// BackendStatus is the JSON payload returned to the UI.
type BackendStatus struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Healthy     bool    `json:"healthy"`
	WriteRate   float64 `json:"writeRate"`
	ErrorRate   float64 `json:"errorRate"`
	BytesSent   int64   `json:"bytesSent"`
	CircuitOpen bool    `json:"circuitOpen"`
	LastError   string  `json:"lastError,omitempty"`
	SkippedRows int64   `json:"skippedRows,omitempty"` // unmapped points dropped (ssfv only)
}

// TSDBWriter is the backend adapter interface.
type TSDBWriter interface {
	WriteBatch(ctx context.Context, batch []DataPoint) error
	// WriteBatchSafe is the idempotent variant used during WAL replay.
	// Implementations must use ON CONFLICT DO NOTHING (or equivalent) so that
	// rows written before a crash are silently skipped on replay.
	WriteBatchSafe(ctx context.Context, batch []DataPoint) error
	HealthCheck(ctx context.Context) error
	Status() BackendStatus
	Name() string
	Close() error
}
