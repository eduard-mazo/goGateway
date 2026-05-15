package tsdb

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const missRingCap = 300

// MissedSignal records a signal_path that had no catalog match in tbl_senales_x_equipo.
// These are actionable: the signal is flowing from the broker but has not been
// configured in the SSFV catalog.
type MissedSignal struct {
	At         time.Time `json:"at"`
	SignalPath string    `json:"signal_path"`
	Equipo     string    `json:"equipo"`
}

// SSFVAdapter is a TSDBWriter that persists DataPoints exclusively to
// ssfv.tbl_valores when the signal_path resolves to a known equisenal_id.
// Points without a catalog match are silently dropped — no fallback table.
//
// Write strategy: unnest batch INSERT ON CONFLICT DO NOTHING.
// - Safe under at-least-once delivery (broker re-sends → DO NOTHING).
// - In-batch dedup collapses (timestamp_utc, equisenal_id) duplicates before
//   hitting the DB, minimising WAL churn from redundant points.
// - COPY FROM is never used: any duplicate in a COPY batch aborts the whole
//   batch, which is incompatible with at-least-once delivery.
type SSFVAdapter struct {
	pool         *pgxpool.Pool
	cache        *SSFVCache
	writeCount   atomic.Int64
	skippedCount atomic.Int64 // unmapped signals dropped
	errorCount   atomic.Int64
	bytesEst     atomic.Int64
	lastErrMsg   atomic.Value
	circuitOpen  atomic.Bool
	rateTracker  *rateTracker

	missMu    sync.Mutex
	missRing  [missRingCap]MissedSignal
	missHead  int
	missCount int
}

// NewSSFVAdapter opens a pgxpool to the ssfv schema on the given DSN, runs
// pending migrations, and primes the in-memory cache.
func NewSSFVAdapter(ctx context.Context, dsn string) (*SSFVAdapter, error) {
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("ssfv_adapter dsn: %w", err)
	}
	poolCfg.MaxConns = 10
	poolCfg.MinConns = 2
	poolCfg.ConnConfig.ConnectTimeout = 15 * time.Second
	poolCfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx,
			"SET search_path TO ssfv, public; "+
				"SET statement_timeout = '30s'; "+
				"SET client_encoding = 'UTF8'")
		return err
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("ssfv_adapter pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ssfv_adapter ping: %w", err)
	}
	resetStaleSsfvMarkers(ctx, pool)
	if err := applyMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ssfv_adapter migrations: %w", err)
	}

	return &SSFVAdapter{
		pool:        pool,
		cache:       newSSFVCache(pool),
		rateTracker: newRateTracker(10 * time.Second),
	}, nil
}

func (a *SSFVAdapter) Name() string { return "ssfv" }

// WriteBatch resolves each point to an equisenal_id and inserts mapped rows
// into ssfv.tbl_valores using ON CONFLICT DO NOTHING.
// Unmapped points are dropped silently. Alarm signals are handled exclusively
// by the worker.AlarmManager — this adapter does not touch tbl_alarmas.
func (a *SSFVAdapter) WriteBatch(ctx context.Context, batch []DataPoint) error {
	if len(batch) == 0 {
		return nil
	}

	type row struct {
		ts      time.Time
		equiID  int
		valor   float64
		calidad string
	}

	seen := make(map[[2]int64]struct{}, len(batch)) // (ts_nano, equiID)
	rows := make([]row, 0, len(batch))
	dropped := int64(0)

	for _, p := range batch {
		// IEC-104 history DataPoints (from HistoryLogger) carry no "equipo" tag.
		// These are expected noise — skip silently, do NOT count as SSFV misses.
		if p.Tags["equipo"] == "" {
			continue
		}
		signalPath := p.Tags["signal_path"]
		if signalPath == "" {
			signalPath = p.Measurement
		}
		equiID, ok := a.cache.Resolve(ctx, signalPath)
		if !ok {
			// Genuine SSFV signal with no catalog match: actionable miss.
			dropped++
			a.recordMiss(signalPath, p.Tags["equipo"])
			continue
		}

		// In-batch dedup: collapse identical (ts, equiID) pairs before the INSERT.
		k := [2]int64{p.Timestamp.UnixNano(), int64(equiID)}
		if _, dup := seen[k]; dup {
			dropped++
			continue
		}
		seen[k] = struct{}{}

		rows = append(rows, row{
			ts:      p.Timestamp,
			equiID:  equiID,
			valor:   primaryValue(p.Fields),
			calidad: qualityFromTags(p.Tags),
		})
	}

	a.skippedCount.Add(dropped)

	if len(rows) == 0 {
		return nil
	}

	tss  := make([]time.Time, len(rows))
	ids  := make([]int,       len(rows))
	vals := make([]float64,   len(rows))
	cals := make([]string,    len(rows))
	for i, r := range rows {
		tss[i]  = r.ts
		ids[i]  = r.equiID
		vals[i] = r.valor
		cals[i] = r.calidad
	}

	tag, err := a.pool.Exec(ctx, `
		INSERT INTO ssfv.tbl_valores (timestamp_utc, equisenal_id, valor, calidad)
		SELECT unnest($1::timestamptz[]), unnest($2::int[]),
		       unnest($3::numeric[]),    unnest($4::text[])
		ON CONFLICT (timestamp_utc, equisenal_id) DO NOTHING`,
		tss, ids, vals, cals)
	if err != nil {
		a.errorCount.Add(1)
		a.lastErrMsg.Store(err.Error())
		return fmt.Errorf("ssfv write valores: %w", err)
	}

	n := tag.RowsAffected()
	a.writeCount.Add(n)
	a.bytesEst.Add(n * 60)
	a.rateTracker.record(n)
	return nil
}

// WriteBatchSafe is identical to WriteBatch: ON CONFLICT DO NOTHING makes
// the write idempotent regardless of whether it is a normal write or WAL replay.
func (a *SSFVAdapter) WriteBatchSafe(ctx context.Context, batch []DataPoint) error {
	return a.WriteBatch(ctx, batch)
}

func (a *SSFVAdapter) HealthCheck(ctx context.Context) error {
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var one int
	if err := a.pool.QueryRow(ctx2, "SELECT 1").Scan(&one); err != nil {
		a.circuitOpen.Store(true)
		a.lastErrMsg.Store(err.Error())
		return fmt.Errorf("ssfv health: %w", err)
	}
	a.circuitOpen.Store(false)
	return nil
}

func (a *SSFVAdapter) Status() BackendStatus {
	lastErr, _ := a.lastErrMsg.Load().(string)
	return BackendStatus{
		Name:        a.Name(),
		Type:        "ssfv_timescaledb",
		Healthy:     !a.circuitOpen.Load(),
		WriteRate:   a.rateTracker.rate(),
		ErrorRate:   float64(a.errorCount.Load()),
		BytesSent:   a.bytesEst.Load(),
		CircuitOpen: a.circuitOpen.Load(),
		LastError:   lastErr,
		SkippedRows: a.skippedCount.Load(),
	}
}

func (a *SSFVAdapter) Close() error {
	a.pool.Close()
	return nil
}

// InvalidateCache flushes the equisenal_id cache. Call after catalog mutations.
func (a *SSFVAdapter) InvalidateCache() {
	a.cache.Invalidate()
}

// Pool exposes the underlying pool for the ssfv catalog API handlers.
func (a *SSFVAdapter) Pool() *pgxpool.Pool {
	return a.pool
}

func (a *SSFVAdapter) recordError(err error) {
	a.errorCount.Add(1)
	a.lastErrMsg.Store(err.Error())
	log.Printf("ssfv: write error: %v", err)
}

// recordMiss stores a signal_path that had no catalog match into the ring buffer.
func (a *SSFVAdapter) recordMiss(signalPath, equipo string) {
	a.missMu.Lock()
	a.missRing[a.missHead] = MissedSignal{At: time.Now(), SignalPath: signalPath, Equipo: equipo}
	a.missHead = (a.missHead + 1) % missRingCap
	if a.missCount < missRingCap {
		a.missCount++
	}
	a.missMu.Unlock()
}

// RecentMisses returns signal paths that recently had no catalog match,
// in chronological order (oldest first), deduplicated by signal_path.
func (a *SSFVAdapter) RecentMisses() []MissedSignal {
	a.missMu.Lock()
	count := a.missCount
	head := a.missHead
	ring := a.missRing // copy under lock
	a.missMu.Unlock()

	if count == 0 {
		return nil
	}
	// Reconstruct in chrono order
	raw := make([]MissedSignal, count)
	if count < missRingCap {
		copy(raw, ring[:count])
	} else {
		n := copy(raw, ring[head:])
		copy(raw[n:], ring[:head])
	}
	// Deduplicate by signal_path, keeping the most recent occurrence per path.
	seen := make(map[string]int, len(raw)) // path → index in out
	out := make([]MissedSignal, 0, len(raw))
	for _, m := range raw {
		if idx, exists := seen[m.SignalPath]; exists {
			out[idx] = m // update to more recent
		} else {
			seen[m.SignalPath] = len(out)
			out = append(out, m)
		}
	}
	return out
}
