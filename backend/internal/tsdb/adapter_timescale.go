package tsdb

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TimescaleConfig holds TimescaleDB (PostgreSQL) connection settings.
type TimescaleConfig struct {
	DSN            string        // postgres://user:pass@host:5432/dbname
	MaxConns       int32         // default 10
	MinConns       int32         // default 2
	ConnectTimeout time.Duration // default 10s
	Table          string        // default "signals"
}

// TimescaleAdapter writes DataPoints to TimescaleDB using pgx COPY (fastest
// bulk insert path). Falls back to INSERT ON CONFLICT DO NOTHING during WAL
// replay to skip duplicates safely.
type TimescaleAdapter struct {
	cfg         TimescaleConfig
	pool        *pgxpool.Pool
	writeCount  atomic.Int64
	errorCount  atomic.Int64
	bytesEst    atomic.Int64
	lastErrMsg  atomic.Value
	circuitOpen atomic.Bool
	rateTracker *rateTracker
}

func NewTimescaleAdapter(ctx context.Context, cfg TimescaleConfig) (*TimescaleAdapter, error) {
	if cfg.MaxConns == 0 {
		cfg.MaxConns = 10
	}
	if cfg.MinConns == 0 {
		cfg.MinConns = 2
	}
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = 10 * time.Second
	}
	if cfg.Table == "" {
		cfg.Table = "signals"
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("timescale dsn: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	poolCfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET statement_timeout = '30s'")
		return err
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("timescale pool: %w", err)
	}

	a := &TimescaleAdapter{
		cfg:         cfg,
		pool:        pool,
		rateTracker: newRateTracker(10 * time.Second),
	}
	if err := a.runMigrations(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("timescale migrations: %w", err)
	}
	return a, nil
}

func (a *TimescaleAdapter) Name() string { return "timescaledb" }

// WriteBatch uses pgx COPY — the fastest bulk insert path, no SQL parsing overhead.
// On unique-constraint violation (same ts+signal_path in the batch), falls back
// to INSERT ON CONFLICT DO NOTHING so duplicate points are silently skipped.
func (a *TimescaleAdapter) WriteBatch(ctx context.Context, batch []DataPoint) error {
	err := a.doCopy(ctx, batch)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return a.doInsertSafe(ctx, batch)
		}
	}
	return err
}

// WriteBatchSafe uses INSERT ON CONFLICT DO NOTHING — used during WAL replay
// to safely skip rows that were already written before the crash.
func (a *TimescaleAdapter) WriteBatchSafe(ctx context.Context, batch []DataPoint) error {
	return a.doInsertSafe(ctx, batch)
}

// dedupBatch drops only exact (ts, signal_path) duplicates — the pair that
// would violate the unique index. Points for the same signal at different
// timestamps are all kept, so high-frequency signals (b1/b2) reach the DB.
func dedupBatch(batch []DataPoint) []DataPoint {
	type key struct {
		path string
		nsec int64
	}
	seen := make(map[key]int, len(batch))
	out := batch[:0:len(batch)]
	for _, p := range batch {
		k := key{path: p.Tags["signal_path"], nsec: p.Timestamp.UnixNano()}
		if idx, ok := seen[k]; ok {
			out[idx] = p // keep last value for true duplicate
		} else {
			seen[k] = len(out)
			out = append(out, p)
		}
	}
	return out
}

func (a *TimescaleAdapter) doCopy(ctx context.Context, batch []DataPoint) error {
	if len(batch) == 0 {
		return nil
	}
	batch = dedupBatch(batch)
	rows := make([][]any, 0, len(batch))
	for _, p := range batch {
		tagsJSON, _ := json.Marshal(p.Tags)
		rows = append(rows, []any{
			p.Timestamp,
			p.Measurement,
			p.Tags["signal_path"],
			primaryValue(p.Fields),
			tagsJSON,
		})
	}
	n, err := a.pool.CopyFrom(
		ctx,
		pgx.Identifier{a.cfg.Table},
		[]string{"ts", "signal", "signal_path", "value", "tags"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			// Real error (not a duplicate-key conflict handled by WriteBatch fallback).
			a.errorCount.Add(1)
			a.lastErrMsg.Store(err.Error())
		}
		return fmt.Errorf("timescale copy: %w", err)
	}
	a.writeCount.Add(n)
	a.bytesEst.Add(n * 80)
	a.rateTracker.record(n)
	return nil
}

func (a *TimescaleAdapter) doInsertSafe(ctx context.Context, batch []DataPoint) error {
	if len(batch) == 0 {
		return nil
	}
	// Single-statement UNNEST bulk insert: one round-trip regardless of batch size,
	// no SQL parsing per row, ON CONFLICT skips duplicates silently.
	tss := make([]time.Time, len(batch))
	sigs := make([]string, len(batch))
	paths := make([]string, len(batch))
	vals := make([]float64, len(batch))
	tagsArr := make([][]byte, len(batch))
	for i, p := range batch {
		tss[i] = p.Timestamp
		sigs[i] = p.Measurement
		paths[i] = p.Tags["signal_path"]
		vals[i] = primaryValue(p.Fields)
		tagsArr[i], _ = json.Marshal(p.Tags)
	}
	q := fmt.Sprintf(`
		INSERT INTO %s (ts, signal, signal_path, value, tags)
		SELECT unnest($1::timestamptz[]), unnest($2::text[]), unnest($3::text[]),
		       unnest($4::double precision[]), unnest($5::jsonb[])
		ON CONFLICT (ts, signal_path) DO NOTHING`, a.cfg.Table)

	ct, err := a.pool.Exec(ctx, q, tss, sigs, paths, vals, tagsArr)
	if err != nil {
		a.errorCount.Add(1)
		a.lastErrMsg.Store(err.Error())
		return fmt.Errorf("timescale insert: %w", err)
	}
	n := ct.RowsAffected()
	a.writeCount.Add(n)
	a.bytesEst.Add(n * 80)
	a.rateTracker.record(n)
	return nil
}

func (a *TimescaleAdapter) HealthCheck(ctx context.Context) error {
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var one int
	if err := a.pool.QueryRow(ctx2, "SELECT 1").Scan(&one); err != nil {
		a.circuitOpen.Store(true)
		a.lastErrMsg.Store(err.Error())
		return fmt.Errorf("timescale health: %w", err)
	}
	a.circuitOpen.Store(false)
	return nil
}

func (a *TimescaleAdapter) Status() BackendStatus {
	lastErr, _ := a.lastErrMsg.Load().(string)
	return BackendStatus{
		Name:        a.Name(),
		Type:        "timescaledb",
		Healthy:     !a.circuitOpen.Load(),
		WriteRate:   a.rateTracker.rate(),
		ErrorRate:   float64(a.errorCount.Load()),
		BytesSent:   a.bytesEst.Load(),
		CircuitOpen: a.circuitOpen.Load(),
		LastError:   lastErr,
	}
}

func (a *TimescaleAdapter) Close() error {
	a.pool.Close()
	return nil
}

// primaryValue extracts the first numeric field as the scalar signal value.
func primaryValue(fields map[string]float64) float64 {
	for _, v := range fields {
		return v
	}
	return 0
}
