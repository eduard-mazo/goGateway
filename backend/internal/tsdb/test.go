package tsdb

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestResult is returned by connection test helpers.
type TestResult struct {
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Message   string `json:"message"`
}

// TestVMConnection checks that the VictoriaMetrics /health endpoint responds.
func TestVMConnection(ctx context.Context, cfg VMConfig) TestResult {
	a := NewVMAdapter(cfg)
	defer a.Close()
	start := time.Now()
	if err := a.HealthCheck(ctx); err != nil {
		return TestResult{OK: false, Message: err.Error()}
	}
	return TestResult{OK: true, LatencyMs: time.Since(start).Milliseconds(), Message: "connected"}
}

// TestTimescaleConnection opens a transient pool and runs SELECT 1.
func TestTimescaleConnection(ctx context.Context, dsn string) TestResult {
	if dsn == "" {
		return TestResult{OK: false, Message: "DSN is empty"}
	}
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return TestResult{OK: false, Message: "invalid DSN: " + err.Error()}
	}
	poolCfg.MaxConns = 2
	poolCfg.MinConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return TestResult{OK: false, Message: err.Error()}
	}
	defer pool.Close()
	start := time.Now()
	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return TestResult{OK: false, Message: err.Error()}
	}
	return TestResult{OK: true, LatencyMs: time.Since(start).Milliseconds(), Message: "connected"}
}
