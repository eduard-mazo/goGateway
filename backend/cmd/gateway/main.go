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

	// IEC 60870-5-104 passive fleet. One Manager owns N slave endpoints;
	// each row in iec104_servers is a listener with its own ASDU address.
	iecMgr := iec104.NewManager(log.Default())
	if servers, err := worker.LoadIEC104Servers(database); err == nil {
		if err := iecMgr.Reload(servers); err != nil {
			log.Printf("iec104 reload: %v", err)
		}
	} else {
		log.Printf("iec104 load: %v", err)
	}
	if err := iecMgr.Start(); err != nil {
		log.Fatalf("iec104 start: %v", err)
	}

	// History logger — async batched writer.
	hist := worker.NewHistoryLogger(database, 4096, 100, 2*time.Second)
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
		DB:         database,
		NotifyMQTT: mqttMgr.Notify,
		NotifyIEC104: func() {
			if servers, err := worker.LoadIEC104Servers(database); err == nil {
				if err := iecMgr.Reload(servers); err != nil {
					log.Printf("iec104 reload: %v", err)
				}
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
