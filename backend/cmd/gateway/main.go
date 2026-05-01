package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"

	"goGateway/internal/api"
	"goGateway/internal/config"
	"goGateway/internal/db"
	"goGateway/internal/iec104"
	"goGateway/internal/mqtt"
	"goGateway/internal/tsdb"
	"goGateway/internal/worker"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	cfg := config.Load()

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	log.Printf("sqlite ready: %s", cfg.DBPath)

	ctx, cancel := context.WithCancel(context.Background())

	// IEC 60870-5-104 passive fleet. One Manager owns N slave endpoints; all
	// share the gateway-wide listen IP from iec104_gateway. Each row in
	// iec104_servers is a port + ASDU + SCADA-IP allowlist.
	iecMgr := iec104.NewManager(log.Default())
	if err := loadIEC104(database, iecMgr); err != nil {
		log.Printf("iec104 load: %v", err)
	}
	if err := iecMgr.Start(); err != nil {
		log.Fatalf("iec104 start: %v", err)
	}

	// History logger — async batched writer.
	hist := worker.NewHistoryLogger(database, 4096, 100, 2*time.Second)

	// TSDB pipeline (optional). Only started when at least one backend is configured.
	var tsdbPipeline *tsdb.WritePipeline
	tsdbPipeline = initTSDB(ctx, cfg.TSDB)
	if tsdbPipeline != nil {
		hist.SetTSDB(tsdbPipeline)
	}

	histDone := make(chan struct{})
	go func() {
		hist.Run(ctx)
		close(histDone)
	}()

	// Mapping cache + MQTT manager.
	cache := worker.NewMappingCache(database)
	if err := cache.Reload(); err != nil {
		log.Printf("initial cache reload: %v", err)
	}
	mqttMgr := mqtt.NewManager(database, cache, iecMgr, hist)
	if err := mqttMgr.Start(ctx); err != nil {
		log.Printf("mqtt start: %v", err)
	}

	// REST API.
	startedAt := time.Now()
	handler := api.NewRouter(api.Deps{
		TSDB: tsdbPipeline,
		DB:         database,
		NotifyMQTT: mqttMgr.Notify,
		NotifyIEC104: func() {
			if err := loadIEC104(database, iecMgr); err != nil {
				log.Printf("iec104 reload: %v", err)
			}
		},
		NotifyMappings: mqttMgr.Notify, // mapping change = resubscribe + refresh cache
		MQTT:           mqttMgr,
		IEC104:         iecMgr,
		StartedAt:      startedAt,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPListen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("http listen %s", cfg.HTTPListen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutdown...")

	// Orderly shutdown. Sequence matters:
	//   1. stop accepting HTTP   → no new writes from the API
	//   2. stop MQTT             → no new samples from the bus
	//   3. stop IEC-104 fleet    → disconnect SCADA masters
	//   4. cancel ctx            → history drains its buffer via ctx.Done
	//   5. wait for drain        → all rows flushed
	//   6. checkpoint + close DB → WAL merged, no truncated tail
	shCtx, shCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shCancel()
	if err := srv.Shutdown(shCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	mqttMgr.Stop()
	_ = iecMgr.Stop()

	cancel()
	select {
	case <-histDone:
	case <-time.After(5 * time.Second):
		log.Println("history drain timed out")
	}

	closeDatabase(database)
}

// loadIEC104 reloads the manager from DB: gateway-wide listen IP + every
// slave row. Bind errors per server are logged inside the manager and not
// fatal — a misconfigured row should not bring the gateway down.
func loadIEC104(database *sqlx.DB, mgr *iec104.Manager) error {
	gw, err := worker.LoadIEC104Gateway(database)
	if err != nil {
		return err
	}
	servers, err := worker.LoadIEC104Servers(database)
	if err != nil {
		return err
	}
	return mgr.Reload(gw, servers)
}

// initTSDB builds and starts the write pipeline when at least one backend is
// configured. Returns nil (no-op) when both connection strings are empty.
func initTSDB(ctx context.Context, cfg config.TSDBConfig) *tsdb.WritePipeline {
	var backends []tsdb.TSDBWriter

	if cfg.VMUrl != "" {
		backends = append(backends, tsdb.NewVMAdapter(tsdb.VMConfig{URL: cfg.VMUrl}))
		log.Printf("tsdb: VictoriaMetrics → %s", cfg.VMUrl)
	}
	if cfg.TimescaleDSN != "" {
		a, err := tsdb.NewTimescaleAdapter(ctx, tsdb.TimescaleConfig{DSN: cfg.TimescaleDSN})
		if err != nil {
			log.Printf("tsdb: timescale init failed: %v", err)
		} else {
			backends = append(backends, a)
			log.Printf("tsdb: TimescaleDB connected")
		}
	}
	if len(backends) == 0 {
		return nil
	}

	os.MkdirAll("data", 0755) //nolint:errcheck
	p, err := tsdb.NewWritePipeline(tsdb.PipelineConfig{
		Backends: backends,
		WALPath:  cfg.WALPath,
		DLQPath:  cfg.DLQPath,
	})
	if err != nil {
		log.Printf("tsdb: pipeline init failed: %v", err)
		return nil
	}
	go p.Run(ctx)
	log.Printf("tsdb: pipeline started (%d backends)", len(backends))
	return p
}

// closeDatabase checkpoints the WAL into the main DB file and closes the
// connection pool. Called exactly once at shutdown after all writers have
// stopped; ensures gateway.db is a self-contained snapshot and no -wal/-shm
// fragment is left for the next boot to replay under lock contention.
func closeDatabase(d *sqlx.DB) {
	if _, err := d.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		log.Printf("db wal checkpoint: %v", err)
	}
	if err := d.Close(); err != nil {
		log.Printf("db close: %v", err)
		return
	}
	log.Println("db closed")
}
