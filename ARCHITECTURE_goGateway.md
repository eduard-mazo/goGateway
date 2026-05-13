---
  Prompt Arquitectónico — goGateway

  Eres un experto en arquitectura de sistemas industriales SCADA/IoT. Debes razonar
  y responder EXCLUSIVAMENTE con base en la siguiente descripción arquitectónica del
  sistema goGateway, un gateway industrial de protocolo escrito en Go.

  ════════════════════════════════════════════════════════════════════════════════
    SISTEMA: goGateway — Gateway Industrial de Protocolo (IEC 60870-5-104 / MQTT)
    LENGUAJE: Go 1.22+ (binario único, SPA embebida via go:embed)
    PLATAFORMA DE PRODUCCIÓN: Windows x64 (host 10.114.199.57)
    BRANCH ACTIVO: feature/ssfv-pipeline
  ════════════════════════════════════════════════════════════════════════════════


  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  A) BOOTSTRAP — cmd/gateway/main.go
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Secuencia de inicialización (en orden):
  1. SQLite (WAL mode, foreign_keys=1, busy_timeout=5000ms) vía db.Open(cfg.DBPath)
  2. iec104.NewManager() — carga gateway + servidores desde DB
  3. worker.NewHistoryLogger() — buffer=4096, batch=100, flush=2s
  4. tsdb.NewManager() — propietario del ciclo de vida de WritePipeline
  5. nats.NewClient() — opcional, condicional a NATSConfig.Enabled
  6. Dispatcher — uno de:
     · DirectDispatcher  → IEC-104 Manager + HistoryLogger directamente
     · NatsDispatcher    → publica InternalPoint JSON a NATS JetStream
  7. NATS Workers — SCADAWorker + TSDBWorker (consumen del stream)
  8. worker.NewMappingCache() — lookup caliente MQTT→IEC104
  9. mqtt.NewManager() — cliente Paho con soporte Sparkplug B
  10. api.NewRouter() — chi v5, sirve REST + SPA embebida
  11. http.Server en cfg.HTTPListen (default :8080)

  Secuencia de shutdown: HTTP → MQTT → IEC-104 → TSDB → NATS → history drain → DB checkpoint

  Configuración runtime (env vars):
    GW_DB   = ruta al archivo SQLite (default: "gateway.db")
    GW_HTTP = dirección de escucha HTTP  (default: ":8080")


  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  B) MODELOS DE DOMINIO — internal/models/models.go
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Device          → {ID, ServerID, Name, Description, CreatedAt}
  Topic           → {ID, DeviceID, Topic, QoS, Enabled}
  SignalMapping   → {ID, ServerID, TopicID, DeviceName, VariableType,
                     Characteristic, JSONKey, QualityKey, MetricName,
                     IEC104Type, IOA, Unit, Scale, Enabled, Business,
                     Company, SignalPath}
  History         → {ID, MappingID, SignalPath, Value, Quality, Timestamp}

  MQTTConfig      → singleton id=1: {Host, Port, Username, Password,
                     ClientID, UseTLS, SparkplugEnabled, SpGroupID, SpHostID}
  IEC104Gateway   → singleton id=1: {ID, ListenIP}
  IEC104Server    → {ID, Name, Port, ASDUAddr, ScadaIPs (CSV),
                     K, W, T0, T1, T2, T3, Enabled}
  TSDBConfig      → singleton id=1: {Backend ("none"|"victoriametrics"|
                     "timescaledb"|"both"), VMUrl, VMUsername, VMPassword,
                     TsDSN, TsTable (default "signals"), WALPath
                     (default "data/wal.bolt"), DLQPath
                     (default "data/dlq.bolt"), BatchSize (default 2000),
                     FlushMs (default 100), Enabled}
  NATSConfig      → singleton id=1: {ID, Host, Port, StreamName, Enabled}


  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  C) SUBSISTEMA IEC 60870-5-104 — internal/iec104/
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ROL: slave pasivo (outstation). Acepta conexiones de maestros SCADA.

  ── types.go ─────────────────────────────────────
  Point      → {IOA int, TypeID string, Value float64, Quality byte, Timestamp time.Time}
  ServerStatus → {Clients, Activated int, Running bool}
  Status     → {Running bool, ListenIP string, Points int, Clients int,
                Activated int, Servers []ServerStatus}

  TypeIDs soportados:
    M_SP_NA_1, M_SP_TB_1       (single-point sin/con timestamp)
    M_DP_NA_1, M_DP_TB_1       (double-point sin/con timestamp)
    M_ME_NA_1                  (normalized measured value)
    M_ME_NB_1                  (scaled measured value, int16)
    M_ME_NC_1                  (short float, sin timestamp)
    M_ME_TF_1                  (short float + CP56Time2a)
    M_IT_NA_1, M_IT_TB_1       (counter integrado sin/con timestamp)
    C_IC_NA_1                  (General Interrogation)
    C_CI_NA_1                  (Counter Interrogation)
    C_CS_NA_1                  (Clock synchronization)

  CoT (Cause of Transmission):
    COT_SPONTANEOUS=3, COT_ACTIVATION=6, COT_ACT_CON=7,
    COT_ACT_TERM=10, COT_INTROGEN=20

  Quality bits:
    QualityGood=0x00, QualityInvalid=0x80, QualityNotTopical=0x40,
    QualitySubstituted=0x20, QualityBlocked=0x10

  ── manager.go ───────────────────────────────────
  Manager: {mu, running bool, listenIP string, servers map[int64]*NativeServer}
    NewManager(log)
    Start() / Stop()
    Dispatch(serverID int64, point Point)
    Reload(gw IEC104Gateway, cfgs []IEC104Server)
    Status() Status / Snapshot() map[int64][]Point

  ── server.go ────────────────────────────────────
  NativeServer: slave TCP por instancia
    - TCP listener en listenIP:port
    - pointStore: mapa concurrente IOA→Point (caché de último valor)
    - clients: lista de clientConn activos
    - Start() / Stop()
    - Dispatch(p Point): actualiza caché + encola a clientes activados
    - Allowlist de IPs desde ScadaIPs CSV (fail-closed: allowlist vacía rechaza todo)

  ── conn.go ──────────────────────────────────────
  clientConn: estado por conexión SCADA-master
    - Secuencias Tx/Rx de 15 bits (NS/NR)
    - Contadores: acked, unacked; timers: t1, t2, t3
    - reader():  parsea tramas entrantes
    - sender():  drena outbox con control de ventana (K/W)
    - timerLoop(): impone t1 (timeout ACK), t2 (S-frame supervisión), t3 (heartbeat)
    - U-frames: STARTDT_ACT/CON, STOPDT_ACT/CON, TESTFR_ACT/CON
    - S-frames: ACKs de secuencia
    - I-frames: datos de señal + secuencia
    - C_IC_NA_1 (GI): responde con snapshot completo agrupado por TypeID
    - C_CI_NA_1 (CI): responde solo con M_IT_* (contadores)

  ── encoding.go ──────────────────────────────────
  encodePoint(p Point) → []byte APDU
  encodeInfoObject(p Point) → (typeID byte, info-object bytes, error)
    Soporta todos los TypeIDs listados arriba.
  packInfoObjects(...) → agrupa ASDUs multi-objeto (SQ=0, máx 127 obj, máx APDU 253 bytes)
  wrapIFrame(ns, nr uint16, asdu []byte) → []byte APDU con cabecera I-format
  cp56Time2a(t time.Time) → [7]byte  (IEC 60870-5-4 §6.8, flags IV + SU)


  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  D) SUBSISTEMA NATS JETSTREAM — internal/nats/
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Client: {cfg (Host, Port, StreamName), nats.Conn, jetstream.JetStream, mu}
    Connect() → URL "nats://host:port", crea contexto JetStream
    initStream() → StreamConfig:
      Name:      StreamName
      Subjects:  [StreamName + ".metrics.>"]
      MaxAge:    24h
      Storage:   FileStorage
      Retention: LimitsPolicy
    JetStream() / Publish(subject string, data []byte) / Close()

  Patrón de subjects: "{STREAM}.metrics.{server_id}.{ioa}"
  Ejemplo: "gw-metrics.metrics.1.10250"

  Consumidores durables (nats_workers.go):
    SCADAWorker: Durable="scada-worker", filter=stream_name.metrics.>,
                 AckExplicit → deserializa InternalPoint → IEC104.Dispatch()
    TSDBWorker:  Durable="tsdb-worker",  filter=stream_name.metrics.>,
                 AckExplicit → deserializa InternalPoint → TSDB.Push()


  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  E) SUBSISTEMA SPARKPLUG B — internal/sparkplug/
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ── topic.go ─────────────────────────────────────
  Namespace: "spBv1.0"
  Formato:   spBv1.0/{group_id}/{message_type}/{edge_node_id}[/{device_id}]
  Tipos:     NBIRTH, NDEATH, NDATA, DBIRTH, DDEATH, DDATA, NCMD, DCMD, STATE
  ParseTopic(raw) → (Topic, bool)
  Topic.NodeBase()   → "spBv1.0/{group}/{node}"
  Topic.DeviceBase() → "spBv1.0/{group}/{node}/{device}"
  StateTopicFor(hostID) → "STATE/{hostID}"  (sin namespace)
  WildcardFor(groupID)  → "spBv1.0/{groupID}/#"

  ── payload.go ───────────────────────────────────
  Parser protobuf wire-format sin dependencia protoc (hand-rolled).
  Payload → {Timestamp ms, Seq uint8 (0-255 wrapping), Metrics []Metric}
  Metric  → {Name, Alias, HasAlias, Timestamp, Datatype,
             IsHistorical, IsTransient, IsNull,
             StringValue, uintVal, fltVal, dblVal, boolVal,
             hasUint, hasFlt, hasDbl, hasBool, hasStr}
  Datatypes: DtInt8…DtUInt64, DtFloat, DtDouble, DtBoolean,
             DtString, DtDateTime, DtText
  Metric.Float64() → (float64, bool)

  ── session.go ───────────────────────────────────
  NodeKey   → {GroupID, EdgeNodeID}
  DeviceKey → {NodeKey, DeviceID}
  NodeSession → {mu, online bool, bdSeq uint64, seq uint8,
                 aliasMap map[uint64]string,
                 devices map[string]*DeviceSession}
    SetBirth(payload)    → online=true, reconstruye alias map, extrae bdSeq
    SetDeath()           → online=false
    AdvanceSeq(incoming) → valida seq==(prev+1) mod 256; false→solicita Rebirth
    ResolveName(metric)  → metric.Name o lookup por alias


  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  F) SUBSISTEMA MQTT — internal/mqtt/client.go
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Manager: {db, cache, dispatcher, mu, client (paho),
            subs map[string]struct{}, connected atomic.Bool,
            registry sparkplug.Registry, spHandler, bdSeq atomic.Uint32}
    Start() → reload() (lee DB + suscribe)
    Notify() → trigger de reload asíncrono
    Status() → {Connected, Broker, Topics (count), Messages, LastMsgAt}

  Config paho:
    AutoReconnect=true, ConnectRetry con intervalo=5s, KeepAlive=5s
    LWT Sparkplug B: "STATE/{hostID}" = "OFFLINE", retained, QoS 1
    Reconstruye client completo en cambio de config (no hace diff)


  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  G) SUBSISTEMA TSDB — internal/tsdb/
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ── interface.go ─────────────────────────────────
  type TSDBWriter interface {
    WriteBatch(ctx context.Context, points []DataPoint) error
    HealthCheck(ctx context.Context) error
    Status() BackendStatus
    Name() string
    Close() error
  }

  ── manager.go ───────────────────────────────────
  Manager: {mu, pipe *WritePipeline, cancel, done, parentCtx, setHist callback,
            ssfvAdapter *SSFVAdapter}
    Pipeline() *WritePipeline
    SSFVAdapter() *SSFVAdapter          ← expone pool al API layer (ssfv catalog)
    Reload(cfg TSDBConfig)
      → instancia VictoriaMetrics y/o TimescaleDB adapters
      → si backend incluye timescaledb: instancia SSFVAdapter (misma DSN)
      → SSFVAdapter falla sin romper pipeline (migración 007 puede no existir)
      → crea WAL, DLQ, PointStore
      → arranca WritePipeline en goroutine con TODOS los backends activos
      → llama setHist callback para conectar HistoryLogger al pipeline
    Stop() → cancela contexto, espera shutdown (timeout 10s), limpia ssfvAdapter

  ── pipeline.go ──────────────────────────────────
  WritePipeline: {inputCh, accumCh, batchCh, retryQueues,
                  backends []TSDBWriter, breakers []CircuitBreaker,
                  wal *WAL, dlq *DLQ, store *PointStore, inputRate}

  Defaults aplicados en NewWritePipeline:
    workers   = NumCPU * 2
    inputBuf  = 100 000
    batchSize = 2 000
    flushMs   = 100
    retries   = 5
    retryBuf  = 20 000

  Flujo de datos:
    Push(point) → inputCh (non-blocking, error si lleno)
    [worker pool]   → actualiza PointStore → accumCh
    [accumulator]   → batch por tamaño o timer
                    → WAL.Append(batch) → walID
                    → ackTracker.Register(walID, numBackends)
                    → batchCh
    [fanOut]        → distribuye batch a todos los backends en paralelo
      ✓ éxito       → ackTracker.Ack(walID)
      ✗ fallo       → CircuitBreaker.RecordFailure() → retryQ
    [retryWorker]   → backoff exponencial (500ms, 2s, 4.5s, 8s, 12.5s)
      ✓ éxito       → Ack
      ✗ max retries → DLQ.Push(backend, batch, reason, retries)
    WAL.Ack(walID)  → elimina entrada cuando todos los backends confirman
    [healthChecker] → sondea backends cada 15s con timeout 5s

  Shutdown(): cancela, drena canales, cierra backends + WAL + DLQ

  ── adapter_vm.go (VictoriaMetrics) ─────────────
  VMAdapter: {cfg (URL, Username, Password, Timeout=10s, MaxIdleConns=16),
              http.Client, contadores, circuitOpen, rateTracker}
  WriteBatch() → gzip + InfluxDB line protocol → POST /write?precision=ns (basic auth)
  HealthCheck() → GET /health
  Formato line protocol:
    measurement,tag1=v1,tag2=v2 field1=v1,field2=v2 timestamp_ns
    (escapa espacios, comas, iguales en tags/measurements)

  ── adapter_timescale.go (TimescaleDB) ───────────
  TimescaleAdapter: {cfg (DSN, MaxConns=10, MinConns=2,
                     ConnectTimeout=10s, Table="signals"),
                     pgxpool.Pool, contadores, circuitOpen, rateTracker}
  WriteBatch()     → pgx COPY (máximo throughput)
                     en violación unique-constraint (23505): fallback a
                     INSERT ON CONFLICT DO NOTHING
  WriteBatchSafe() → INSERT ON CONFLICT DO NOTHING (WAL replay)
  dedupBatch()     → elimina duplicados exactos (ts, signal_path) pre-write
  Schema tabla:    (ts, signal, signal_path, value, tags jsonb)
  Statement timeout: 30s vía AfterConnect hook

  ── adapter_ssfv.go (SSFVAdapter) ────────────────
  SSFVAdapter implementa TSDBWriter — opera sobre el schema ssfv de PostgreSQL.
  Se instancia en paralelo con TimescaleAdapter cuando backend="timescaledb"|"both".
  Comparte la misma DSN; fallo de inicialización es no-fatal (log + continúa).

  SSFVAdapter: {pool *pgxpool.Pool, cache *SSFVCache,
                contadores atómicos, circuitOpen, rateTracker}
    Name() → "ssfv"
    WriteBatch(ctx, []DataPoint):
      para cada punto:
        signalPath = tags["signal_path"] o p.Measurement
        cache.Resolve(ctx, signalPath) → EquiSenal_Id
          ✓ resuelto   → acumula en valores[]   (→ Tbl_Valores COPY)
                          si isAlarmSignal()   → insertAlarma (Tbl_Alarmas)
          ✗ no mapeado → acumula en rawRows[]  (→ signals_raw COPY)
      writeValores(): CopyFrom ssfv."Tbl_Valores"; 23505 → insertValoresSafe (UNNEST)
      writeRaw():     CopyFrom ssfv.signals_raw; 23505 → ignorar
    Pool() *pgxpool.Pool   → expone pool al API layer (ssfv_catalog.go)
    InvalidateCache()      → flushea SSFVCache (llamar tras cambios en catálogo)

  Detección de alarmas (isAlarmSignal):
    lastPathSegment == "AL_COM"  OR  prefijo "AL_"

  Calidad IEC-104 → texto ssfv:
    0x00 → "Buena"    0x80 → "Mala"    otros → "Dudosa"

  ── ssfv_cache.go ────────────────────────────────
  SSFVCache: {pool *pgxpool.Pool, cache sync.Map (string→int)}
    Resolve(ctx, signalPath) → (EquiSenal_Id int, found bool)
      SQL: Tbl_Equipo.Nombre_Topic || '/' || Tbl_Senales_x_Equipo.Nombre_Instancia = $1
      Sentinel equiSenalNotFound=-1 para cache negativo (evita re-query)
    Invalidate() → elimina todas las entradas (trigger: cambio de catálogo UI)

  ── circuit_breaker.go ───────────────────────────
  FSM 3 estados: Closed → Open → HalfOpen → Closed
    failureThreshold = 5
    successRequired  = 2
    openTimeout      = 30s
  Allow()             → true si debe intentarse la escritura
  RecordSuccess() / RecordFailure()
  State() → "closed" | "open" | "half-open"

  ── wal.go ───────────────────────────────────────
  WAL: {bolt.DB, seq atomic (monotónico), pending (count)}
  WALEntry: {ID uint64, Timestamp, Batch []DataPoint} — serializado msgpack
  Append(batch) → msgpack → BoltDB, retorna walID
  Ack(id)       → elimina entrada
  Pending()     → count de entradas sin ACK

  ── dlq.go ───────────────────────────────────────
  DLQ: {bolt.DB, mu, seq}
  DLQEntry: {ID, Backend, Timestamp, Batch, Reason, Retries} — JSON
  Push(backend, batch, reason, retries) → almacena batch fallido
  Len()          → count de entradas
  Replay(fn)     → itera entradas; fn(entry) retorna true para ACK

  ── ack_tracker.go ───────────────────────────────
  ackTracker: {mu, entries map[uint64]*ackEntry, wal ref}
  Register(walID, count) → crea entrada esperando `count` ACKs de backends
  Ack(walID)             → decrementa; cuando==0, elimina entrada WAL

  ── point_store.go ───────────────────────────────
  Mapa concurrente 128 shards, clave: IOA string
  StoredPoint: {Value float64, Timestamp time.Time, Tags map[string]string}
  Set(key, point) / Get(key)  → lock-free por shard
  Snapshot()  → copia completa (para panel UI live)
  Len()       → total de puntos

  ── metrics.go / handler.go ──────────────────────
  rateTracker: ventana deslizante 10s (conteos por segundo)
  Handler expone endpoints REST de introspección TSDB (ver sección I)


  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  G2) SUBSISTEMA SSFV — Schema PostgreSQL + Catálogo
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Propósito: modelar el dominio de Sistemas Solares Fotovoltaicos (SSFV)
  sobre TimescaleDB. Es una capa semántica sobre el pipeline TSDB genérico.
  Independiente de Dispositivos/Tópicos/Mapeo — recibe DataPoints ya procesados.

  ── Migración 000007_ssfv_schema.up.sql ──────────
  Crea schema "ssfv" idempotente (IF NOT EXISTS):

  BLOQUE 1 — Catálogos:
    Tbl_Tipo_Equipo    (Tipo_Id, Nombre, Descripcion, Activo)
    Tbl_Tipo_Variable  (TipoVar_Id, Nombre, Descripcion, Activo)
    Tbl_Unidades       (Unidad_Id, Simbolo, Nombre, Magnitud, Activo)
    Datos iniciales: 4 tipos equipo, 11 tipos variable, 13 unidades

  BLOQUE 2 — Infraestructura:
    Tbl_Planta         (Planta_Id, Nombre, Broker_Base UNIQUE, Capacidad_kWp,
                        Estado 0/1/2, Fecha_Comisionamiento)
    Tbl_Equipo         (Equipo_Id, Planta_Id→FK, Tipo_Id→FK, Nombre_Equipo,
                        Nombre_Topic UNIQUE, Fabricante, Modelo, Nro_Serie,
                        Estado, UNIQUE(Planta_Id, Nombre_Equipo))
    Tbl_Frontera_Comercial (Frontera_Id, Planta_Id→FK, Codigo_NIE UNIQUE, Activo)

  BLOQUE 3 — Catálogo señales:
    Tbl_Senales        (Senal_Id, TipoVar_Id→FK, Unidad_Id→FK, Nombre,
                        Tipo_Valor IN ('Instantaneo','Acumulado'),
                        Codigo_Senal, Es_Indexada, Activo,
                        UNIQUE(Codigo_Senal, TipoVar_Id))
    Tbl_Senales_x_Equipo (EquiSenal_Id, Senal_Id→FK, Equipo_Id→FK,
                          Indice_Canal SMALLINT NULL ≥1,
                          Nombre_Instancia VARCHAR(30),
                          Activo, UNIQUE(Senal_Id, Equipo_Id, Indice_Canal))
    → signal_path = Nombre_Topic || '/' || Nombre_Instancia

  BLOQUE 4 — Series temporales (hypertables TimescaleDB):
    Tbl_Valores  PK(Timestamp_UTC, EquiSenal_Id→FK), Valor NUMERIC(18,6),
                 Calidad IN ('Buena','Dudosa','Mala')
                 chunk_time_interval=1 day, compress_after=7 days

    Tbl_Alarmas  PK(Alarma_Id BIGSERIAL, Ts_Inicio), EquiSenal_Id→FK,
                 Ts_Fin, Tipo_Alarma IN ('Dispositivo','Comunicacion','Proceso','Fabricante'),
                 Severidad IN ('Critica','Alta','Media','Baja'), Activa BOOL
                 chunk_time_interval=7 days, compress_after=30 days

    signals_raw  PK(ts, signal_path), signal TEXT, value DOUBLE, quality SMALLINT, tags JSONB
                 chunk_time_interval=1 day
                 Fallback: puntos sin EquiSenal_Id mapeado

  BLOQUE 5 — Vistas:
    v_Senales_Contexto   → JOIN completo sxe+s+tv+u+e+te+p (vista operacional)
    v_Ultimas_Lecturas   → DISTINCT ON (EquiSenal_Id) ORDER BY ts DESC
    v_Alarmas_Activas    → alarmas WHERE Activa=TRUE JOIN v_Senales_Contexto

  Rollback (000007_ssfv_schema.down.sql):
    DROP SCHEMA IF EXISTS ssfv CASCADE

  ── Resolución de signal_path → EquiSenal_Id ─────
  Flujo de escritura SSFV:
    DataPoint.Tags["signal_path"] = "EPM/SSFV/EPM/Sede30/INV_1/AP"
    SSFVCache.Resolve() busca:
      WHERE Nombre_Topic || '/' || Nombre_Instancia = signal_path
      e.g. "EPM/SSFV/EPM/Sede30/INV_1" || '/' || "AP" = match
    ✓ → Tbl_Valores (escritura principal)
    ✗ → signals_raw (fallback, sin pérdida de datos)

  INVARIANTE: Nombre_Topic del equipo debe coincidir exactamente con
  el prefijo del signal_path generado por sparkplug_dispatch.go.

  ── Relación con el pipeline existente ───────────
  SSFVAdapter NO reemplaza — COEXISTE con TimescaleAdapter y VMAdapter.
  Todos reciben el mismo DataPoint via pipeline fan-out.
  Dispositivos/Tópicos/Mapeo siguen siendo necesarios para:
    - Definir qué tópicos MQTT escuchar
    - Mapear métricas → IEC-104 (IOA, TypeID)
    - Construir el signal_path (Business/Company/Node/Metric)
  El catálogo SSFV agrega semántica sobre ese signal_path ya construido.


  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  H) CAPA DE WORKERS — internal/worker/
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ── dispatcher.go ────────────────────────────────
  type Dispatcher interface {
    Dispatch(tm TopicMapping, val float64, quality int, ts time.Time)
  }

  InternalPoint (JSON publicado a NATS):
    {MappingID, ServerID, Topic, IOA, TypeID, Value,
     Quality, Timestamp, SignalPath}

  DirectDispatcher → IEC104.Dispatch() + HistoryLogger.Log() directo
  NatsDispatcher   → publica InternalPoint a "{stream}.metrics.{serverID}.{ioa}"

  ── dispatch.go ──────────────────────────────────
  ParseAndDispatch(topic, payload, mappings, dispatcher) — hot path central
    1. Parsea payload JSON
    2. Resuelve timestamp: campo "date" RFC3339 o time.Now()
    3. Calidad nivel-payload (tier 1): clave "quality" (int QDS o string
       "GOOD"/"BAD"/"UNCERTAIN") → aplica a todas las señales del mensaje
    4. Calidad por señal (tier 2): mapping.QualityKey → sobreescribe tier 1
    5. Llama dispatcher.Dispatch() por cada señal mapeada

  ── nats_workers.go ──────────────────────────────
  SCADAWorker: durable="scada-worker" → InternalPoint → IEC104.Dispatch()
  TSDBWorker:  durable="tsdb-worker"  → InternalPoint → TSDB pipeline.Push()

  ── sparkplug_dispatch.go ────────────────────────
  SparkplugHandler: {registry *sparkplug.Registry, cache *MappingCache,
                     dispatcher Dispatcher, rebirthFn func(nodeBase)}
  Dispatch(topic ParsedTopic, raw []byte):
    → NBIRTH: valida seq==0, sesión online, reconstruye alias map, despacha métricas
    → NDEATH: marca nodo offline
    → NDATA:  valida seq in-order → despacha métricas
               seq gap detectado → publica NCMD Rebirth al edge node
    → DBIRTH/DDEATH/DDATA: equivalentes a nivel dispositivo
  MarkNodeStale(nodeBase): despacha QualityNotTopical a todos los mappings del nodo

  spSignalPath(business, company, topic, isDevice, metricName) → string
    Si metricName ya empieza con "business/company/" → retorna directamente (UNS path)
    Sino construye: business/company/group/node[/device]/metricName

  ── cache.go ─────────────────────────────────────
  TopicMapping: {MappingID, ServerID, TopicID, Topic, JSONKey, QualityKey,
                 MetricName, IEC104Type, IOA, Scale, Business, Company, SignalPath}

  Índices duales:
    Modo JSON:        map[topic] → []TopicMapping
    Modo Sparkplug B: map[nodeBase + "\x00" + metricName] → []TopicMapping
                      map[nodeBase] → []TopicMapping (todos los mappings del nodo)
  Reload() → JOIN SQL (signal_mappings + topics + iec104_servers), filtra enabled=1

  ── history.go ───────────────────────────────────
  HistoryEvent: {MappingID, SignalPath, Value, Quality, Timestamp, IOA}
  HistoryLogger: {db, ch (async, bufSize=4096), batch=100, flush=2s, tsdbPipe}
    Log(event) → encola sin bloqueo (retorna false si buffer lleno)
                 también hace Push al pipeline TSDB como DataPoint
    Run(ctx)   → batch por timer o tamaño, INSERT masivo en tabla history (SQLite),
                 drena al recibir done

  NOTA: La tabla history en SQLite es redundante cuando TimescaleDB está activo.
  Pendiente de migración (ver Plan P1 en sección M).


  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  I) REST API — internal/api/
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Framework: go-chi/chi v5
  Middleware: RequestID, RealIP, Logger, Recoverer, Timeout(30s),
              CORS (all origins, GET/POST/PUT/DELETE/OPTIONS,
              headers: Content-Type/Authorization), BodyLimit(1MB)
  decode() usa json.Decoder.DisallowUnknownFields()

  Deps inyectadas al router:
    DB, NotifyMQTT, NotifyIEC104, NotifyMappings, NotifyTSDB,
    NotifyNATS func() (callbacks de recarga en caliente)
    MQTT Manager, IEC104 Manager, TSDB Manager, StartedAt time.Time

  Rutas:
    GET  /health                           → "ok"
    GET  /api/status                       → {MQTT, IEC104, Devices, Topics,
                                              Mappings, HistoryCount, LastSampleAt,
                                              UptimeSeconds, StartedAt}
    -- Dispositivos y Tópicos --
    GET/POST         /api/devices          → CRUD dispositivos
    GET/PUT/DELETE   /api/devices/{id}
    GET/POST         /api/topics           → CRUD tópicos
    PUT/DELETE       /api/topics/{id}
    -- Configuración --
    GET/PUT          /api/mqtt-config      → singleton Broker MQTT
    GET/PUT          /api/nats-config      → singleton NATS
    GET/PUT          /api/iec104-gateway   → singleton IP escucha
    GET/POST         /api/iec104-servers   → CRUD servidores IEC-104
    GET/PUT/DELETE   /api/iec104-servers/{id}
    GET/POST         /api/mappings         → CRUD signal mappings
    PUT/DELETE       /api/mappings/{id}
    GET              /api/history          → histórico SQLite (?mapping_id, ?limit)
    GET/PUT          /api/tsdb-config      → singleton TSDB (GET enmascara creds;
                                             PUT acepta has_ts_dsn/has_vm_password
                                             como campos ignorados)
    POST             /api/tsdb-config/test → test conectividad VM o TimescaleDB
    -- Pipeline TSDB introspección --
    GET              /api/tsdb/status      → backends, WAL, DLQ, PointStore, inputRate
    GET              /api/tsdb/dlq         → entradas Dead-Letter Queue
    POST             /api/tsdb/dlq/replay  → re-inyectar DLQ al pipeline
    GET              /api/tsdb/points      → snapshot live del PointStore
    -- Catálogo SSFV (requiere SSFVAdapter conectado) --
    GET              /api/ssfv/status      → {connected, healthy, write_rate, circuit_open}
    GET/POST         /api/ssfv/plantas     → CRUD Tbl_Planta
    PUT/DELETE       /api/ssfv/plantas/{id}
    GET/POST         /api/ssfv/equipos     → CRUD Tbl_Equipo (?planta_id filter)
    PUT/DELETE       /api/ssfv/equipos/{id}
    GET/POST         /api/ssfv/senales     → CRUD Tbl_Senales
    PUT/DELETE       /api/ssfv/senales/{id}
    GET/POST         /api/ssfv/asignaciones → CRUD Tbl_Senales_x_Equipo (?equipo_id)
                                             POST/PUT invalida SSFVCache automáticamente
    PUT/DELETE       /api/ssfv/asignaciones/{id}
    GET/POST         /api/ssfv/fronteras   → CRUD Tbl_Frontera_Comercial
    PUT/DELETE       /api/ssfv/fronteras/{id}
    GET              /api/ssfv/tipo-equipo → catálogo read-only
    GET              /api/ssfv/tipo-variable
    GET              /api/ssfv/unidades
    GET              /api/ssfv/vista/senales-contexto  → v_Senales_Contexto
    GET              /api/ssfv/vista/ultimas-lecturas  → v_Ultimas_Lecturas
    GET              /api/ssfv/vista/alarmas-activas   → v_Alarmas_Activas
    GET              /api/ssfv/vista/raw               → signals_raw (?limit=100)
    POST             /api/ssfv/cache/invalidate         → flushea SSFVCache
    GET  /*                                → SPA fallback (Vue 3 embebida)


  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  J) PERSISTENCIA SQLITE — internal/db/db.go
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ALCANCE: solo configuración del programa. Los datos históricos de proceso
  se almacenan en TimescaleDB (tabla signals y schema ssfv). Ver Plan P1.

  DSN: file:gateway.db?_pragma=journal_mode(WAL)
                      &_pragma=foreign_keys(1)
                      &_pragma=busy_timeout(5000)

  Tablas de configuración (permanentes):
    iec104_gateway    singleton IP de escucha global
    iec104_servers    fleet (port, ASDUAddr, ScadaIPs CSV, timers K/W/T0-T3, enabled)
    devices           UNIQUE(server_id, name)
    topics            (device_id, topic, QoS, enabled)
    signal_mappings   (server_id, topic_id, JSONKey, QualityKey, MetricName,
                       IEC104Type, IOA, Scale, Business, Company)
                       UNIQUE(server_id, ioa)
    mqtt_config       singleton (host, port, TLS, Sparkplug, group/host IDs)
    nats_config       singleton (host, port, StreamName, enabled)
    tsdb_config       singleton (backend, VM URL/creds, TS DSN/table, WAL/DLQ paths,
                                 batchSize, flushMs, enabled)

  Tabla transitoria (a eliminar en Plan P1):
    history           (mapping_id, signal_path, value, quality, timestamp)
                      → duplica datos ya en TimescaleDB cuando TSDB activo

  Migraciones idempotentes aplicadas en Open():
    - iec104_servers:  drop columna legacy listen_addr
    - devices:         añade columna server_id
    - signal_mappings: añade server_id, quality_key, metric_name, business, company
    - Cambia unicidad IOA a (server_id, ioa)


  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  K) FRONTEND — Vue 3 SPA (embebida en binario Go)
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Stack:
    Vue 3 (Composition API) + TypeScript
    Vite 8.0.9 (bundler)
    vue-router 5.0.6
    shadcn-vue 2.6.2 (Reka UI + componentes radix)
    @tanstack/vue-table 8.21.3
    lucide-vue-next 1.0.0
    Tailwind CSS 4.2.4 + tailwind-merge + tw-animate-css
    axios 1.15.2  (baseURL: "/api")
    vue-sonner 2.0.9
    @vueuse/core 14.2.1
    Idioma: español (i18n.ts con t.nav.*, t.field.*, t.status.*)

  Build: pnpm (no npm) → frontend/dist/ → backend/internal/web/dist/
         → embebido en binario Go vía go:embed

  Rutas de la SPA:
    /          → Dashboard.vue      (Panel Principal)
    /mappings  → SignalMapper.vue   (Mapeo de Señales)
    /devices   → DevicesTopics.vue  (Dispositivos y Tópicos)
    /mqtt      → MqttConfig.vue     (Broker MQTT)
    /nats      → NatsConfig.vue     (Fan-Out NATS)
    /iec104    → Iec104Config.vue   (IEC 60870-5-104)
    /history   → History.vue        (Histórico — fuente SQLite; ver Plan P1)
    /tsdb      → TSDBView.vue       (Pipeline TSDB)
    /ssfv      → SSFVView.vue       (Plantas Solares SSFV)

  Vistas:
    Dashboard.vue     (15 KB) — tarjetas de estado MQTT/IEC104/TSDB, métricas live
    DevicesTopics.vue (23 KB) — CRUD dispositivos + tópicos MQTT
    Iec104Config.vue  (24 KB) — IP de escucha, CRUD servidores, allowlist SCADA
    SignalMapper.vue  (35 KB) — editor de mappings MQTT/Sparkplug B → IEC-104
    MqttConfig.vue    (9.5 KB)— config broker, TLS, habilitación Sparkplug B
    NatsConfig.vue    (7.4 KB) — config NATS JetStream
    History.vue       (8.5 KB) — query histórico por mapping_id (SQLite)
    TSDBView.vue      (18.6 KB)— estado pipeline, salud backends, gestión WAL/DLQ
    SSFVView.vue      (30 KB) — 4 tabs: Estado · Plantas & Equipos · Catálogo Señales
                                · Monitoreo (últimas lecturas / alarmas / raw)

  Composables:
    useStatus.ts  → polling /api/status (estado live MQTT + IEC104 + TSDB)
    useTSDB.ts    → métricas pipeline + gestión DLQ
    useConfirm.ts → modal confirmación para acciones destructivas

  Componentes UI base (shadcn-vue):
    Badge, Button, Dialog, Input, Select, Switch, Table, Tooltip,
    Card, Separator, Label, Sonner (toast notifications)
    + ConfirmDialog.vue, StatusPill.vue, StatCard.vue


  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  L) FLUJOS DE DATOS PRINCIPALES
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ── MODO JSON MQTT (sin Sparkplug B) ─────────────
  MQTT Broker
    → mqtt.Manager (paho callback)
    → worker.ParseAndDispatch(topic, JSON, mappings)
    → dispatcher.Dispatch(mapping, scaledValue, quality, ts)
      ├→ IEC104.Manager.Dispatch(serverID, Point)
      │    → NativeServer.Dispatch(point) → outbox → clientConn → I-frame SCADA
      └→ HistoryLogger.Log(event)
           ├→ SQLite history (batch INSERT) [transitorio — ver Plan P1]
           └→ TSDB pipeline.Push(DataPoint) → WAL → backends

  ── MODO SPARKPLUG B ─────────────────────────────
  MQTT Broker (spBv1.0/# topics)
    → mqtt.Manager (paho callback con state machine Sparkplug)
    → worker.SparkplugHandler.Dispatch(ParsedTopic, protobuf)
      ├→ NBIRTH: valida seq=0, online, alias map, despacha métricas
      ├→ NDEATH: offline
      ├→ NDATA:  valida seq in-order → despacha métricas
      │           seq gap → publica NCMD Rebirth al edge node
      └→ por cada métrica: cache lookup por (nodeBase + metricName)
                           → dispatcher.Dispatch(mapping, ...)
                           → spSignalPath(): si metricName es UNS path completo
                             (empieza con business/company) → usa directo

  ── MODO FAN-OUT NATS (opcional) ─────────────────
  dispatcher.Dispatch(...)
    → NatsDispatcher → nats.Client.Publish(subject, InternalPoint JSON)
      → JetStream stream "{STREAM}.metrics.{serverID}.{ioa}"
        → SCADAWorker (durable) → IEC104.Dispatch()
        → TSDBWorker  (durable) → TSDB.Push()

  ── PIPELINE TSDB (alta disponibilidad) ──────────
  inputCh ← pipeline.Push(DataPoint)
    [NumCPU×2 workers] → PointStore.Set() → accumCh
    [accumulator]      → batch (size|timer) → WAL.Append() → ackTracker → batchCh
    [fanOut]           → backend ×N en paralelo:
                          · VMAdapter      → VictoriaMetrics (si configurado)
                          · TimescaleAdapter → tabla signals (schema público)
                          · SSFVAdapter    → schema ssfv (si migración 007 aplicada)
      ✓ → ackTracker.Ack() → WAL.Ack() al completar todos
      ✗ → CircuitBreaker.RecordFailure() → retryQ
    [retryWorker]      → backoff (500ms/2s/4.5s/8s/12.5s) → DLQ (máx retries)
    [healthChecker]    → sondeo cada 15s (timeout 5s)
    Al startup         → WAL replay vía WriteBatchSafe() (TimescaleDB safe insert)

  ── FLUJO SSFV DETALLADO ─────────────────────────
  DataPoint{signal_path="EPM/SSFV/EPM/Sede30/INV_1/AP", value=32.1}
    → SSFVAdapter.WriteBatch()
       → SSFVCache.Resolve("EPM/SSFV/EPM/Sede30/INV_1/AP")
            SQL: "EPM/SSFV/EPM/Sede30/INV_1" || '/' || "AP" → EquiSenal_Id=42
            ✓ hit  → cache.Store(path, 42)
            ✗ miss → cache.Store(path, -1)  [negativo cache]
       ✓ resuelto → acumula (ts, 42, 32.1, "Buena") para CopyFrom Tbl_Valores
                    si AL_* → insertAlarma con Severidad según Calidad
       ✗ sin mapeo → acumula para CopyFrom signals_raw
       → writeValores() COPY / insertValoresSafe() si 23505
       → writeRaw()    COPY / silencioso si 23505
  ---
