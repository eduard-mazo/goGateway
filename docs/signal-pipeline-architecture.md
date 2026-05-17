# Signal Pipeline Architecture

End-to-end flow from an incoming MQTT message to an IEC-104 spontaneous frame and time-series DB storage.

---

## Overview

```
MQTT broker
    │  raw bytes (JSON or Sparkplug B protobuf)
    ▼
mqtt.Manager  ──────────────────────────────────────────────────────────────────────┐
    │  topic lookup in MappingCache                                                  │
    │                                                                                │
    ├─ JSON mode ──► ParseAndDispatch()                                              │
    │                   extract value by JSONKey, apply Scale, resolve Quality       │
    │                                                                                │
    └─ Sparkplug B ──► SparkplugHandler.Dispatch()                                  │
                        session seq-check, alias→name resolution, SSFV dual-route   │
                                                                                     │
                         worker.Dispatcher (interface)                               │
                        ┌────────────────────────────────────────────────────────┐  │
                        │ DirectDispatcher          NatsDispatcher               │  │
                        │   │    │                     │                         │  │
                        │   │    └──► HistoryLogger     └──► NATS JetStream      │  │
                        │   │              │                   ├─ SCADAWorker    │  │
                        │   ▼             │                   │     └──► IEC104  │  │
                        │ IEC104 Manager  │                   └─ TSDBWorker      │  │
                        │   │             ▼                         └──► History │  │
                        │   │         TSDB Pipeline                              │  │
                        │   ▼              │                                     │  │
                        │ NativeServer(s)  └──► VictoriaMetrics / TimescaleDB   │  │
                        └────────────────────────────────────────────────────────┘  │
                                                                                     │
                        SSFV path ─────────────────────────────────────────────────┘
                          SSFVHandler → TimescaleDB (ssfv schema, ON CONFLICT DO NOTHING)
```

---

## 1. Configuration & Storage (SQLite)

All gateway configuration lives in `gateway.db` (SQLite, WAL mode).

### Core tables

| Table | Purpose |
|---|---|
| `mqtt_config` | Singleton broker connection parameters; `sparkplug_enabled`, `sp_group_id`, `sp_host_id`, `sp_topics` |
| `topics` | MQTT subscriptions scoped to a `device_id`; carries `topic`, `qos`, `enabled` |
| `devices` | Logical device scoped to one `iec104_servers` row |
| `signal_mappings` | **The mapping table** — one row per IEC-104 point |
| `iec104_gateway` | Singleton `listen_ip` (bind NIC for all slave endpoints) |
| `iec104_servers` | One row per passive slave: `port`, `asdu_addr`, `scada_ips` allowlist, k/w/t timers |
| `tsdb_config` | TSDB pipeline parameters (backend, URL/DSN, WAL/DLQ paths, batch tuning) |
| `nats_config` | NATS JetStream fan-out parameters |

### `signal_mappings` column roles

```
id            — surrogate key
server_id     — FK → iec104_servers.id  (which slave receives this point)
topic_id      — FK → topics.id          (which MQTT topic provides the data)
device_name   — human label (no runtime effect)
variable_type — human label
characteristic — human label
json_key      — JSON mode: key extracted from payload object
quality_key   — optional: JSON key holding per-signal quality (overrides payload-level "quality")
metric_name   — Sparkplug B mode: metric name (matched after alias→name resolution)
iec104_type   — IEC-104 TypeID string (e.g. "M_ME_NC_1", "M_SP_NA_1", "M_ME_TF_1")
ioa           — Information Object Address; UNIQUE(server_id, ioa)
unit          — display only
scale         — multiplier applied to raw value before dispatch (default 1.0)
enabled       — 0/1 gate; filtered at cache load time
business      — first segment of signal_path
company       — second segment of signal_path
```

**Uniqueness constraint**: `(server_id, ioa)` — the same IOA may appear on different servers (different ASDU address spaces).

---

## 2. MappingCache (`worker/cache.go`)

Built in memory on startup and reloaded whenever config changes via `Notify`.

```
MappingCache.Reload()
  ├─ SQL JOIN: signal_mappings + topics + iec104_servers
  │   WHERE sm.enabled=1 AND t.enabled=1 AND s.enabled=1
  │
  ├─ JSON mode (metric_name == ""):
  │   m[topic]          → []TopicMapping
  │   qos[topic]        → byte
  │   SignalPath pre-computed: business/company/topic/json_key
  │
  └─ Sparkplug B mode (metric_name != ""):
      sp[nodeBase+NUL+metricName]  → []TopicMapping   (per-metric lookup)
      spAll[nodeBase]              → []TopicMapping   (full-node, for NDEATH stale)
```

`TopicMapping` is the resolved join row carried through the hot path:

```go
type TopicMapping struct {
    MappingID, ServerID, TopicID int64
    Topic, JSONKey, QualityKey, MetricName string
    IEC104Type string
    IOA        int
    Scale      float64
    Business, Company, SignalPath string
}
```

---

## 3. MQTT Ingestion (`mqtt/client.go`)

`mqtt.Manager` holds one paho client. On message arrival the callback:

1. Increments message counter, records timestamp.
2. Fires `monitorHook` (broker monitor SSE ring buffer).
3. If `SparkplugEnabled` → calls `SparkplugHandler.Dispatch(topic, raw)`.
4. Otherwise → calls `worker.ParseAndDispatch(topic, raw, cache.Lookup(topic), dispatcher)`.

### Reload

`Notify()` triggers a full reconnect: drops current subscriptions, re-reads `mqtt_config` and all enabled topics from DB, resubscribes, calls `cache.Reload()`.

---

## 4a. JSON Path — `ParseAndDispatch` (`worker/dispatch.go`)

```
payload = {"date": "2025-01-01T00:00:00Z", "quality": 0, "power": 123.4, "volt_q": 0}

for each TopicMapping m:
    val = raw[m.JSONKey]           // extract by key name
    scaled = val * m.Scale
    quality = payload["quality"]   // tier-1 quality
    if m.QualityKey != "":
        quality = raw[m.QualityKey]  // tier-2 per-signal override
    d.Dispatch(m, scaled, quality, ts)
```

**Timestamp**: reads `"date"` field (RFC3339). Falls back to `time.Now()`.

**Quality resolution** (two tiers):
- Tier 1: `"quality"` key in payload — applies to all signals in the message.
- Tier 2: mapping's `quality_key` field — overrides tier-1 for that signal only.
- Integer values are used directly (masked to valid QDS bits).
- String values mapped: `"GOOD"/"OK"/"VALID"` → 0x00, `"BAD"/"INVALID"` → 0x80, `"UNCERTAIN"/"STALE"` → 0x40.

---

## 4b. Sparkplug B Path — `SparkplugHandler` (`worker/sparkplug_dispatch.go`)

The gateway is a **Sparkplug B Primary Application** (passive host, no own NBIRTH).

```
Incoming message type:
  NBIRTH → register session, seq reset to 0, dispatch all metrics
  NDATA  → advance seq (request rebirth if OOO), dispatch changed metrics
  NDEATH → SetDeath(), MarkNodeStale() → dispatch QualityNotTopical for all mappings
  DBIRTH → device session birth, dispatch metrics
  DDATA  → device seq advance, dispatch metrics
  DDEATH → device stale
```

**Metric dispatch**:

```
dispatchMetric(topic, isDevice, metricName, metric, ts):
  1. SSFV intercept (dual-route):
     if metric path matches known SSFV equipment topic:
         ssfvHandler.HandleMetric(mqttTopic, code, val, ts)
  2. IEC-104 route:
     maps = cache.LookupByMetric(nodeBase, metricName)
     for each tm in maps:
         signalPath = business/company/group/node[/device]/metricName
         scaled = metric.Float64() * tm.Scale
         d.Dispatch(tm, scaled, metric.IEC104Quality(), ts)
```

**Signal path for Sparkplug B**: `business/company/group/node[/device]/metricName`.
If `metricName` already starts with `business/company/` (full UNS path embedded), it is used verbatim to avoid duplicate prefix.

---

## 5. Dispatcher (`worker/dispatcher.go`)

`Dispatcher` is an interface with one method:

```go
Dispatch(tm TopicMapping, val float64, quality int, ts time.Time)
```

Two implementations are wired at startup:

### DirectDispatcher (NATS disabled)

```
Dispatch(tm, val, quality, ts):
  iec104.Server.Dispatch(tm.ServerID, Point{IOA, TypeID, Value, Quality, Timestamp})
  hist.Log(HistoryEvent{...})
```

### NatsDispatcher (NATS enabled)

```
Dispatch(tm, val, quality, ts):
  marshal InternalPoint → JSON
  nats.Publish("STREAM.metrics.{serverID}.{IOA}", data)

SCADAWorker (goroutine):
  consume NATS "STREAM.metrics.>"
  iec104.Server.Dispatch(pt.ServerID, Point{...})
  msg.Ack()

TSDBWorker (goroutine):
  consume NATS "STREAM.metrics.>"
  hist.Log(HistoryEvent{...})
  msg.Ack()
```

Both consumers use **durable JetStream consumers** with `AckExplicitPolicy` — no message is lost if a worker restarts mid-batch.

---

## 6. IEC-104 Dispatch (`iec104/manager.go`, `iec104/server.go`)

```
Manager.Dispatch(serverID int64, p Point):
  s = servers[serverID]
  if !s.enabled || !s.running → drop silently
  s.Dispatch(p)

NativeServer.Dispatch(p Point):
  pointStore.put(p)            // update in-memory snapshot (used for GI replies)
  asdu = encodePoint(p, COT_SPONTANEOUS, cfg.ASDUAddr)
  for each activated clientConn:
      cc.send(asdu)            // enqueue I-frame to client write goroutine
```

### ASDU encoding (`iec104/encoding.go`)

TypeID string → binary ASDU:

| TypeID string | Byte | Format | Notes |
|---|---|---|---|
| `M_SP_NA_1` | 1 | IOA(3) + SIQ(1) | Single-point, no time |
| `M_DP_NA_1` | 3 | IOA(3) + DIQ(1) | Double-point, no time |
| `M_ME_NA_1` | 9 | IOA(3) + NVA(2) + QDS(1) | Normalized (float→int16 ÷32767) |
| `M_ME_NB_1` | 11 | IOA(3) + SVA(2) + QDS(1) | Scaled int16 |
| `M_ME_NC_1` | 13 | IOA(3) + R32(4) + QDS(1) | Short float, no time |
| `M_IT_NA_1` | 15 | IOA(3) + BCR(4) + QDS(1) | Integrated totals |
| `M_SP_TB_1` | 30 | IOA(3) + SIQ(1) + CP56(7) | Single-point + timestamp |
| `M_DP_TB_1` | 31 | IOA(3) + DIQ(1) + CP56(7) | Double-point + timestamp |
| `M_ME_TF_1` | 36 | IOA(3) + R32(4) + QDS(1) + CP56(7) | Short float + timestamp |
| `M_IT_TB_1` | 37 | IOA(3) + BCR(4) + QDS(1) + CP56(7) | Integrated totals + timestamp |

CP56Time2a (7 bytes) carries ms-precision timestamp with DST and invalid-time flags.

### APCI wrapping

Each ASDU is wrapped in an I-frame:

```
0x68 | len(4+ASDU) | N(S)<<1 | N(R)<<1 | ASDU bytes
```

Sequence numbers are tracked per connection (15-bit space, `seqMask = 0x7FFF`). The server manages k-window backpressure and t2/t3 keep-alive timers per client connection.

### General Interrogation (GI)

On `C_IC_NA_1` (TypeID=100, QOI=20) from a SCADA master, the server iterates `pointStore.snapshot()` and sends all points as `COT_INTROGEN` (20) ASDUs, then closes with `COT_ACT_TERM`.

---

## 7. Time-Series DB Storage (`worker/history.go`, `tsdb/pipeline.go`)

```
hist.Log(HistoryEvent):
  if tsdbPipe != nil:
      tsdbPipe.Push(DataPoint{
          Measurement: lastSegment(signalPath),
          Tags: {signal_path, business, company},
          Fields: {value, quality},
          Timestamp,
      })
  → channel (non-blocking; drops if buffer full)
```

### WritePipeline stages

```
Push(DataPoint)
  → inputCh (buffered 50k)
      → [N workers] update PointStore (in-mem latest values cache)
      → accumCh
          → accumulator: batch by size (500 pts) or flush interval (500ms)
              → WAL.Append(batch) → BoltDB (crash durability)
              → batchCh
                  → fanOut: parallel dispatch to all backends
                      → backend.WriteBatch(ctx, batch)
                          ✓ WAL.Ack(walID)
                          ✗ → retryQ (per backend, exp backoff: 500ms → 2s → 4.5s → 8s → 12.5s)
                              max 3 retries → DLQ (BoltDB)
```

Circuit breaker per backend: open after failures, probed every 15 s via `HealthCheck`.

WAL replay on startup: unacked batches resubmitted via `WriteBatchSafe` (idempotent, `ON CONFLICT DO NOTHING`).

### Backends

| Backend | Adapter | Write method |
|---|---|---|
| VictoriaMetrics | `adapter_vm.go` | HTTP POST, InfluxDB line protocol |
| TimescaleDB | `adapter_timescale.go` | pgx `COPY FROM` binary |
| SSFV (TimescaleDB) | `adapter_ssfv.go` | pgx, `ON CONFLICT DO NOTHING`, ssfv schema |

---

## 8. Startup Wiring (`cmd/gateway/main.go`)

```
db.Open(gateway.db)
iec104.NewManager() → loadIEC104(db) → iecMgr.Start()
worker.NewHistoryLogger()
tsdb.NewManager() → tsdbMgr.Reload(tsdbCfg)   // starts WritePipeline
worker.NewMappingCache(db) → cache.Reload()
mqtt.NewManager(db, cache, dispatcher)

if NATS enabled:
    nats.Connect()
    dispatcher = NatsDispatcher
    go SCADAWorker.Run()      // NATS → IEC-104
    go TSDBWorker.Run()       // NATS → HistoryLogger
else:
    dispatcher = DirectDispatcher  // direct IEC-104 + HistoryLogger

ssfvHandler wired into mqttMgr
mqttMgr.Start(ctx)
api.NewRouter(Deps{...}) → http.ListenAndServe()
```

On config save via REST API, the relevant `Notify*` callback fires:
- `NotifyMQTT` / `NotifyMappings` → `mqttMgr.Notify()` → reconnect + cache reload
- `NotifyIEC104` → `loadIEC104()` → `iecMgr.Reload()`
- `NotifyTSDB` → `tsdbMgr.Reload()` → restart pipeline

---

## 9. Quality Encoding Summary

| Source value | IEC-104 QDS byte |
|---|---|
| `0x00` / `"GOOD"` / `"OK"` / `"VALID"` | `0x00` (Good) |
| `0x80` / `"BAD"` / `"INVALID"` | `0x80` (Invalid / IV) |
| `0x40` / `"UNCERTAIN"` / `"STALE"` | `0x40` (Not Topical / NT) |
| `0x20` / `"SUBSTITUTED"` | `0x20` (Substituted / SB) |
| `0x10` / `"BLOCKED"` | `0x10` (Blocked / BL) |
| NDEATH / connection loss | `0x40` (QualityNotTopical) dispatched to all node mappings |
| Sparkplug metric `is_transient=true` | skipped (not dispatched) |

---

## 10. Data Lineage by Example

```
MQTT: spBv1.0/EPM_SSFV/NDATA/Nodo1
  payload: Sparkplug B protobuf
    metric: name="outputs/power" alias=42 float_value=250.5 quality=GOOD

SparkplugHandler.handleNDATA():
  session.ResolveName(alias=42) → "outputs/power"
  dispatchMetric(topic, false, "outputs/power", metric, ts):
    ssfvHandler.HandleMetric("EPM_SSFV/Nodo1", "power", 250.5, ts) → TimescaleDB
    maps = cache.LookupByMetric("spBv1.0/EPM_SSFV/Nodo1", "outputs/power")
    → [TopicMapping{ServerID:1, IOA:3001, IEC104Type:"M_ME_NC_1", Scale:1.0, Business:"EPM", Company:"XYZ"}]
    signalPath = "EPM/XYZ/EPM_SSFV/Nodo1/outputs/power"
    scaled = 250.5 * 1.0 = 250.5

d.Dispatch(tm, 250.5, 0x00, ts):

  IEC-104 path:
    Manager.Dispatch(serverID=1, Point{IOA:3001, TypeID:"M_ME_NC_1", Value:250.5, Quality:0})
    NativeServer.Dispatch → pointStore.put → encodeInfoObject:
      [0x0D] [0x01] [0x03] [0x00] [0x01 0x00] [0xB9 0x0B 0x0D 0x3C 0x00 0x00 0x00 0x00]
      ↑TypeID ↑N=1  ↑SPONT ↑ORG  ↑ASDUAddr=1  ↑IOA=3001(3B)   ↑R32(250.5) ↑QDS=GOOD
    → I-frame to every activated SCADA master connection

  TSDB path:
    hist.Log → tsdbPipe.Push(DataPoint{
        Measurement: "power",
        Tags: {signal_path:"EPM/XYZ/EPM_SSFV/Nodo1/outputs/power", business:"EPM", company:"XYZ"},
        Fields: {value:250.5, quality:0},
        Timestamp: ts,
    })
    → batch accumulate → WAL → VictoriaMetrics or TimescaleDB
```
