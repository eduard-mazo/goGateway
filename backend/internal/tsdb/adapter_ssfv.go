package tsdb

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SSFVAdapter is a TSDBWriter that routes DataPoints to the ssfv schema:
//
//   - Points whose signal_path resolves to an EquiSenal_Id → ssfv.Tbl_Valores
//   - Points without a catalog match → ssfv.signals_raw (fallback)
//   - Points whose signal code is AL_COM or ends in alarm suffix → ssfv.Tbl_Alarmas
//
// It implements the same TSDBWriter interface as TimescaleAdapter and can
// coexist with it in the same pipeline (each receiving all DataPoints).
type SSFVAdapter struct {
	pool        *pgxpool.Pool
	cache       *SSFVCache
	writeCount  atomic.Int64
	errorCount  atomic.Int64
	bytesEst    atomic.Int64
	lastErrMsg  atomic.Value
	circuitOpen atomic.Bool
	rateTracker *rateTracker
}

// NewSSFVAdapter opens a connection pool to the ssfv schema on the given DSN.
func NewSSFVAdapter(ctx context.Context, dsn string) (*SSFVAdapter, error) {
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("ssfv_adapter dsn: %w", err)
	}
	poolCfg.MaxConns = 10
	poolCfg.MinConns = 2
	poolCfg.ConnConfig.ConnectTimeout = 15 * time.Second
	poolCfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET search_path TO ssfv, public; SET statement_timeout = '30s'")
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

	return &SSFVAdapter{
		pool:        pool,
		cache:       newSSFVCache(pool),
		rateTracker: newRateTracker(10 * time.Second),
	}, nil
}

func (a *SSFVAdapter) Name() string { return "ssfv" }

// WriteBatch routes each point to the correct ssfv table.
func (a *SSFVAdapter) WriteBatch(ctx context.Context, batch []DataPoint) error {
	if len(batch) == 0 {
		return nil
	}

	var valores  [][]any // (Timestamp_UTC, EquiSenal_Id, Valor, Calidad)
	var rawRows  [][]any // (ts, signal_path, signal, value, quality, tags)

	for _, p := range batch {
		signalPath := p.Tags["signal_path"]
		if signalPath == "" {
			signalPath = p.Measurement
		}
		val := primaryValue(p.Fields)
		calidad := qualityFromTags(p.Tags)

		if equiID, ok := a.cache.Resolve(ctx, signalPath); ok {
			valores = append(valores, []any{p.Timestamp, equiID, val, calidad})

			// Alarm signal → also insert into Tbl_Alarmas when value > 0
			if isAlarmSignal(signalPath) && val > 0 {
				if err := a.insertAlarma(ctx, equiID, p.Timestamp, calidad); err != nil {
					log.Printf("ssfv: alarm insert %q: %v", signalPath, err)
				}
			}
		} else {
			tagsJSON, _ := json.Marshal(p.Tags)
			rawRows = append(rawRows, []any{
				p.Timestamp, signalPath, p.Measurement, val,
				qualityInt(p.Tags), tagsJSON,
			})
		}
	}

	var firstErr error
	if err := a.writeValores(ctx, valores); err != nil {
		a.recordError(err)
		firstErr = err
	}
	if err := a.writeRaw(ctx, rawRows); err != nil {
		a.recordError(err)
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// WriteBatchSafe uses INSERT ON CONFLICT DO NOTHING — WAL replay path.
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
	}
}

func (a *SSFVAdapter) Close() error {
	a.pool.Close()
	return nil
}

// InvalidateCache flushes the EquiSenal_Id cache (call after catalog changes).
func (a *SSFVAdapter) InvalidateCache() {
	a.cache.Invalidate()
}

// Pool exposes the pool for the ssfv catalog API handlers.
func (a *SSFVAdapter) Pool() *pgxpool.Pool {
	return a.pool
}

// --- internal helpers ---

func (a *SSFVAdapter) writeValores(ctx context.Context, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	n, err := a.pool.CopyFrom(
		ctx,
		pgx.Identifier{"ssfv", "Tbl_Valores"},
		[]string{"Timestamp_UTC", "EquiSenal_Id", "Valor", "Calidad"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return a.insertValoresSafe(ctx, rows)
		}
		return fmt.Errorf("ssfv copy valores: %w", err)
	}
	a.writeCount.Add(n)
	a.bytesEst.Add(n * 60)
	a.rateTracker.record(n)
	return nil
}

func (a *SSFVAdapter) insertValoresSafe(ctx context.Context, rows [][]any) error {
	tss := make([]time.Time, len(rows))
	ids := make([]int, len(rows))
	vals := make([]float64, len(rows))
	cals := make([]string, len(rows))
	for i, r := range rows {
		tss[i] = r[0].(time.Time)
		ids[i] = r[1].(int)
		vals[i] = r[2].(float64)
		cals[i] = r[3].(string)
	}
	_, err := a.pool.Exec(ctx, `
		INSERT INTO ssfv."Tbl_Valores" ("Timestamp_UTC","EquiSenal_Id","Valor","Calidad")
		SELECT unnest($1::timestamptz[]), unnest($2::int[]),
		       unnest($3::numeric[]), unnest($4::text[])
		ON CONFLICT ("Timestamp_UTC","EquiSenal_Id") DO NOTHING`,
		tss, ids, vals, cals)
	return err
}

func (a *SSFVAdapter) writeRaw(ctx context.Context, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	n, err := a.pool.CopyFrom(
		ctx,
		pgx.Identifier{"ssfv", "signals_raw"},
		[]string{"ts", "signal_path", "signal", "value", "quality", "tags"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil // duplicate raw point is OK
		}
		return fmt.Errorf("ssfv copy raw: %w", err)
	}
	a.writeCount.Add(n)
	a.bytesEst.Add(n * 80)
	return nil
}

func (a *SSFVAdapter) insertAlarma(ctx context.Context, equiID int, ts time.Time, calidad string) error {
	tipoAlarma := "Comunicacion"
	severidad := "Alta"
	if calidad == "Mala" {
		severidad = "Critica"
	}
	_, err := a.pool.Exec(ctx, `
		INSERT INTO ssfv."Tbl_Alarmas"
		    ("EquiSenal_Id","Ts_Inicio","Tipo_Alarma","Severidad","Activa")
		VALUES ($1,$2,$3,$4,TRUE)
		ON CONFLICT DO NOTHING`,
		equiID, ts, tipoAlarma, severidad)
	return err
}

func (a *SSFVAdapter) recordError(err error) {
	a.errorCount.Add(1)
	a.lastErrMsg.Store(err.Error())
}

func isAlarmSignal(path string) bool {
	seg := lastPathSegment(path)
	return seg == "AL_COM" || len(seg) > 3 && seg[:3] == "AL_"
}

func lastPathSegment(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

func qualityInt(tags map[string]string) int {
	if q, ok := tags["quality"]; ok {
		var qi int
		fmt.Sscanf(q, "%d", &qi) //nolint:errcheck
		return qi
	}
	return 0
}
