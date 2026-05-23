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
	"goGateway/internal/auth"
	"goGateway/internal/config"
	"goGateway/internal/db"
	"goGateway/internal/iec104"
	"goGateway/internal/models"
	"goGateway/internal/mqtt"
	"goGateway/internal/nats"
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
	seedDefaultAdmin(database)

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

	// TSDB manager — owns the pipeline lifecycle; reloaded from DB on config save.
	tsdbMgr := tsdb.NewManager(ctx, hist.SetTSDB)

	// Load initial TSDB config from DB and start pipeline if enabled.
	var tsdbCfg models.TSDBConfig
	if err := database.Get(&tsdbCfg, `SELECT id,backend,vm_url,vm_username,vm_password,ts_dsn,ts_table,wal_path,dlq_path,batch_size,flush_ms,enabled FROM tsdb_config WHERE id=1`); err != nil {
		log.Printf("tsdb: load config: %v", err)
	} else {
		tsdbMgr.Reload(tsdbCfg)
	}

	// NATS Fan-out integration.
	var natsCfg models.NATSConfig
	if err := database.Get(&natsCfg, `SELECT id,host,port,stream_name,enabled FROM nats_config WHERE id=1`); err != nil {
		log.Printf("nats: load config: %v", err)
	}

	// Edge-compute engines (signal filtering, virtual signals, alarms).
	deadband := worker.NewDeadbandFilter()
	calcEng := worker.NewCalcEngine(database, iecMgr)
	threshEng := worker.NewThresholdEngine(database, iecMgr)
	if err := calcEng.Reload(); err != nil {
		log.Printf("calc engine: load: %v", err)
	}
	if err := threshEng.Reload(); err != nil {
		log.Printf("threshold engine: load: %v", err)
	}
	go calcEng.Run(ctx)
	go threshEng.Run(ctx)

	var natsClient *nats.Client
	var innerDispatcher worker.Dispatcher = worker.NewDirectDispatcher(iecMgr, hist)

	if natsCfg.Enabled {
		natsClient = nats.NewClient(natsCfg)
		if err := natsClient.Connect(); err != nil {
			log.Printf("nats: connect error (falling back to direct dispatch): %v", err)
		} else {
			innerDispatcher = worker.NewNatsDispatcher(natsClient, natsCfg.StreamName)
			// Start NATS workers.
			scadaWorker := worker.NewSCADAWorker(natsClient, natsCfg.StreamName, iecMgr)
			go func() {
				if err := scadaWorker.Run(ctx); err != nil {
					log.Printf("scada worker: %v", err)
				}
			}()

			tsdbWorker := worker.NewTSDBWorker(natsClient, natsCfg.StreamName, hist)
			go func() {
				if err := tsdbWorker.Run(ctx); err != nil {
					log.Printf("tsdb worker: %v", err)
				}
			}()
		}
	}

	// Wrap the chosen inner dispatcher with edge-compute filtering.
	dispatcher := worker.NewFilteringDispatcher(innerDispatcher, deadband, calcEng, threshEng)

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
	mqttMgr := mqtt.NewManager(database, cache, dispatcher)

	// SSFV subsystem: JSON handler routes solar equipment topics into TimescaleDB.
	ssfvCache := worker.NewSSFVMappingCache()
	ssfvHandler := worker.NewSSFVHandler(ssfvCache, tsdbMgr.Pipeline(), nil)
	// Wire pool, alarm manager, and miss callback if SSFV adapter is already connected.
	if sa := tsdbMgr.SSFVAdapter(); sa != nil {
		alarmMgr := worker.NewAlarmManager(sa.Pool())
		ssfvHandler.SetAlarmManager(alarmMgr)
		ssfvHandler.SetMissFn(sa.RecordMiss)
		if err := ssfvCache.Reload(sa.Pool()); err != nil {
			log.Printf("ssfv cache reload: %v", err)
		}
	}
	mqttMgr.SetSSFVHandler(ssfvHandler)

	// Auto-discovery: records unknown Sparkplug B nodes/devices in SQLite.
	autoDisc := worker.NewAutoDiscoveryService(database)
	mqttMgr.SetAutoDiscovery(autoDisc)

	// Broker monitor — ring buffer + SSE fan-out for the UI monitor view.
	brokerMon := api.NewBrokerMonitor()
	mqttMgr.SetMonitorHook(brokerMon.Push)

	if err := mqttMgr.Start(ctx); err != nil {
		log.Printf("mqtt start: %v", err)
	}

	// REST API.
	startedAt := time.Now()
	authCfg := auth.DefaultConfig(cfg.JWTSecret)
	handler := api.NewRouter(api.Deps{
		DB:         database,
		NotifyMQTT: mqttMgr.Notify,
		NotifyIEC104: func() {
			if err := loadIEC104(database, iecMgr); err != nil {
				log.Printf("iec104 reload: %v", err)
			}
		},
		NotifyMappings: mqttMgr.Notify, // mapping change = resubscribe + refresh cache
		NotifyTSDB: func() {
			var reloadCfg models.TSDBConfig
			if err := database.Get(&reloadCfg, `SELECT id,backend,vm_url,vm_username,vm_password,ts_dsn,ts_table,wal_path,dlq_path,batch_size,flush_ms,enabled FROM tsdb_config WHERE id=1`); err != nil {
				log.Printf("tsdb: reload config: %v", err)
				return
			}
			tsdbMgr.Reload(reloadCfg)
			// Update SSFVHandler with the new pipeline — the old one is closed.
			ssfvHandler.SetPipeline(tsdbMgr.Pipeline())
			if sa := tsdbMgr.SSFVAdapter(); sa != nil {
				ssfvHandler.SetAlarmManager(worker.NewAlarmManager(sa.Pool()))
				ssfvHandler.SetMissFn(sa.RecordMiss)
				if err := ssfvCache.Reload(sa.Pool()); err != nil {
					log.Printf("ssfv: mapping cache reload after tsdb reload: %v", err)
				}
			} else {
				ssfvHandler.SetAlarmManager(nil)
				ssfvHandler.SetMissFn(nil)
			}
		},
		NotifyNATS: func() {
			log.Printf("nats: config changed (restart required to apply)")
		},
		NotifySSFV: func() {
			sa := tsdbMgr.SSFVAdapter()
			if sa == nil {
				return
			}
			if err := ssfvCache.Reload(sa.Pool()); err != nil {
				log.Printf("ssfv: mapping cache reload: %v", err)
			}
		},
		MQTT:      mqttMgr,
		IEC104:    iecMgr,
		TSDBMgr:   tsdbMgr,
		BrokerMon: brokerMon,
		StartedAt: startedAt,
		AuthCfg:   authCfg,
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
	//   0. close SSE connections → unblocks http.Shutdown (SSE are long-lived)
	//   1. stop accepting HTTP   → no new writes from the API
	//   2. stop MQTT             → no new samples from the bus
	//   3. stop IEC-104 fleet    → disconnect SCADA masters
	//   4. stop TSDB pipeline    → flush remaining points
	//   5. cancel ctx            → history drains its buffer via ctx.Done
	//   6. wait for drain        → all rows flushed
	//   7. checkpoint + close DB → WAL merged, no truncated tail
	brokerMon.Close() // terminate SSE streams so Shutdown doesn't time out
	shCtx, shCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shCancel()
	if err := srv.Shutdown(shCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	mqttMgr.Stop()
	_ = iecMgr.Stop()
	tsdbMgr.Stop()
	if natsClient != nil {
		natsClient.Close()
	}

	cancel()
	select {
	case <-histDone:
	case <-time.After(5 * time.Second):
		log.Println("history drain timed out")
	}

	closeDatabase(database)
}

// seedDefaultAdmin creates an admin/admin superadmin account if the users table
// is empty. The operator should change this password immediately after first
// login — the warning log makes that hard to miss.
func seedDefaultAdmin(database *sqlx.DB) {
	var count int
	if err := database.Get(&count, `SELECT COUNT(*) FROM users`); err != nil {
		log.Printf("seed: check users: %v", err)
		return
	}
	if count > 0 {
		return
	}
	hash, err := auth.HashPassword("admin")
	if err != nil {
		log.Printf("seed: hash password: %v", err)
		return
	}
	if _, err := database.Exec(
		`INSERT INTO users (username, password_hash, full_name, role, enabled)
		 VALUES ('admin', ?, 'Administrador', 'superadmin', 1)`, hash,
	); err != nil {
		log.Printf("seed: insert admin: %v", err)
		return
	}
	log.Println("AUTH: default user created — username: admin, password: admin — CHANGE IMMEDIATELY")
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
