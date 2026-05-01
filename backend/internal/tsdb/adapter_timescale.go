package tsdb

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
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
func (a *TimescaleAdapter) WriteBatch(ctx context.Context, batch []DataPoint) error {
	return a.doCopy(ctx, batch)
}

// WriteBatchSafe uses INSERT ON CONFLICT DO NOTHING — used during WAL replay
// to safely skip rows that were already written before the crash.
func (a *TimescaleAdapter) WriteBatchSafe(ctx context.Context, batch []DataPoint) error {
	return a.doInsertSafe(ctx, batch)
}

func (a *TimescaleAdapter) doCopy(ctx context.Context, batch []DataPoint) error {
	if len(batch) == 0 {
		return nil
	}
	rows := make([][]any, 0, len(batch))
	for _, p := range batch {
		tagsJSON, _ := json.Marshal(p.Tags)
		rows = append(rows, []any{
			p.Timestamp,
			p.Measurement,
			p.Tags["ioa"],
			primaryValue(p.Fields),
			tagsJSON,
		})
	}
	n, err := a.pool.CopyFrom(
		ctx,
		pgx.Identifier{a.cfg.Table},
		[]string{"ts", "measurement", "ioa", "value", "tags"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		a.errorCount.Add(1)
		a.lastErrMsg.Store(err.Error())
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
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("timescale tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := fmt.Sprintf(`INSERT INTO %s (ts,measurement,ioa,value,tags)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (ts,measurement,ioa) DO NOTHING`, a.cfg.Table)

	b := &pgx.Batch{}
	for _, p := range batch {
		tagsJSON, _ := json.Marshal(p.Tags)
		b.Queue(q, p.Timestamp, p.Measurement, p.Tags["ioa"], primaryValue(p.Fields), tagsJSON)
	}
	res := tx.SendBatch(ctx, b)
	for range batch {
		if _, err := res.Exec(); err != nil {
			res.Close()
			a.errorCount.Add(1)
			a.lastErrMsg.Store(err.Error())
			return fmt.Errorf("timescale insert: %w", err)
		}
	}
	res.Close()
	if err := tx.Commit(ctx); err != nil {
		a.errorCount.Add(1)
		a.lastErrMsg.Store(err.Error())
		return fmt.Errorf("timescale commit: %w", err)
	}
	a.writeCount.Add(int64(len(batch)))
	a.rateTracker.record(int64(len(batch)))
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
