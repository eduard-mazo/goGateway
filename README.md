# goGateway

MQTT → IEC 60870-5-104 bridge.  Receives process data over MQTT (plain JSON or
Sparkplug B protobuf) and exposes it as an IEC 60870-5-104 slave to a SCADA
system.  Optionally forwards every signal sample to VictoriaMetrics and/or
TimescaleDB for long-term storage and analysis.

## Quick start

```
make build              # frontend + embed + Go binary (Linux)
make build-win          # cross-compile Windows/amd64
make run PORT=9090      # build + run on custom port (default :8080)
./goGateway             # run pre-built binary on :8080
```

Open `http://localhost:8080` for the web UI.

## MQTT modes

| Mode | `sparkplug_enabled` | Payload format | Topic format |
|------|---------------------|----------------|--------------|
| JSON | `0` (default) | Arbitrary JSON object | Any string |
| Sparkplug B | `1` | Protobuf (Eclipse Sparkplug B v2.2) | `spBv1.0/{group}/{MSG_TYPE}/{node}[/{device}]` |

Switch modes in **UI → MQTT Config → Sparkplug B**.

See [docs/sparkplug-b.md](docs/sparkplug-b.md) for Sparkplug B topic/payload
examples and step-by-step operator configuration.

## Architecture

```
MQTT broker
    │
    ▼
mqtt.Manager ──[sparkplug_enabled=0]──► worker.ParseAndDispatch (JSON)
    │           [sparkplug_enabled=1]──► worker.SparkplugHandler (protobuf)
    │
    ├──► HistoryLogger (SQLite ring-buffer, max 250 MB)
    │
    ├──► TSDB pipeline (VictoriaMetrics / TimescaleDB)
    │       ├── 128-shard PointStore  (dedup, lock-free at scale)
    │       ├── BoltDB WAL            (crash-safe, replay on restart)
    │       ├── Circuit breaker       (per backend, auto-recovery)
    │       └── BoltDB DLQ            (dead-letter, UI-triggered replay)
    │
    ▼
iec104.Server  (one slave per iec104_servers row)
    │
    ▼
SCADA / RTU  (IEC 60870-5-104 client)
```

## Configuration

All configuration is persisted in a SQLite database (`goGateway.db` by default).
The web UI exposes every setting.

| Table | Purpose |
|-------|---------|
| `mqtt_config` | Broker address, credentials, TLS, Sparkplug B settings |
| `iec104_gateway` | Gateway-wide IEC-104 listen IP |
| `iec104_servers` | One row per IEC-104 slave endpoint |
| `devices` | Logical grouping of topics (scoped to a server) |
| `topics` | MQTT topics to subscribe to |
| `signal_mappings` | Topic/metric → IEC-104 IOA bindings |
| `tsdb_config` | TSDB pipeline settings (backend, URLs, WAL/DLQ paths) |

## TSDB pipeline

Configure in **UI → TSDB Pipeline**.  Supports three modes:

| Backend | Best for |
|---------|----------|
| `victoriametrics` | High-throughput ingest, Grafana, long retention, minimal ops |
| `timescaledb` | SQL queries, Grafana, joins with relational data |
| `both` | Write to both simultaneously; each has its own circuit breaker and retry queue |

The pipeline is crash-safe: every batch is written to a BoltDB WAL before the
network call.  On restart, pending batches are replayed automatically.
Failed batches after 5 retries go to the DLQ; replay them from the UI.

### Data model

Each IEC-104 signal sample is forwarded as one data point:

| Field | Value |
|-------|-------|
| Measurement (metric name) | IEC-104 ASDU type — e.g. `M_ME_NB_1`, `M_SP_NA_1` |
| Tag: `ioa` | Information Object Address — e.g. `1001` |
| Tag: `signal_key` | Original MQTT signal key |
| Field: `value` | Numeric signal value |
| Field: `quality` | IEC-104 quality descriptor bitmask |

---

### Querying VictoriaMetrics

Open `http://<vm-host>:8428/vmui` for the built-in query UI.

The metric name is the IEC-104 ASDU type.  Use MetricsQL (PromQL-compatible).

**Discover what metrics exist:**
```promql
-- list all metric names
{__name__=~".+"}

-- list all IOAs for a given type
M_ME_NB_1
```

**Single signal by IOA:**
```promql
M_ME_NB_1{ioa="1001"}
```

**All signals of a type:**
```promql
M_ME_NB_1
```

**Filter by signal_key (MQTT path):**
```promql
{signal_key="plant/unit1/temperature"}
```

**Any signal on a given IOA regardless of type:**
```promql
{ioa="1001"}
```

**Rate of change:**
```promql
rate(M_ME_NB_1{ioa="1001"}[1m])
```

**Only good-quality samples (quality bitmask = 0):**
```promql
M_ME_NB_1{ioa="1001"} == 0
```
> Quality field is stored separately; filter `value` where the quality metric is 0:
```promql
M_ME_NB_1{ioa="1001"} unless M_ME_NB_1_quality{ioa="1001"} > 0
```

**Grafana datasource:** add VictoriaMetrics as a Prometheus datasource pointing
to `http://<vm-host>:8428`.

---

### Querying TimescaleDB

Connect with any PostgreSQL client (`psql`, DBeaver, Grafana).

#### Raw signals table

```sql
-- schema
-- signals(ts TIMESTAMPTZ, measurement TEXT, ioa TEXT, value DOUBLE PRECISION,
--         quality SMALLINT, tags JSONB)

-- last 100 samples for a single IOA
SELECT ts, value, quality
FROM signals
WHERE ioa = '1001'
ORDER BY ts DESC
LIMIT 100;

-- last hour for a specific type + IOA
SELECT ts, value, quality
FROM signals
WHERE measurement = 'M_ME_NB_1'
  AND ioa = '1001'
  AND ts > NOW() - INTERVAL '1 hour'
ORDER BY ts;

-- all active signals right now (latest value per IOA)
SELECT DISTINCT ON (ioa)
    ioa, measurement, value, quality, ts
FROM signals
ORDER BY ioa, ts DESC;

-- bad-quality events in the last 24 hours
SELECT ts, measurement, ioa, value, quality
FROM signals
WHERE quality > 0
  AND ts > NOW() - INTERVAL '24 hours'
ORDER BY ts DESC;

-- filter by MQTT signal_key stored in JSONB tags
SELECT ts, value
FROM signals
WHERE tags->>'signal_key' = 'plant/unit1/temperature'
  AND ts > NOW() - INTERVAL '6 hours'
ORDER BY ts;
```

#### Continuous aggregates (pre-computed rollups)

| View | Resolution | Retention |
|------|-----------|-----------|
| `signals_1m` | 1 minute | 1 year |
| `signals_1h` | 1 hour | 5 years |
| `signals_1d` | 1 day | forever |

Each aggregate exposes: `bucket`, `measurement`, `ioa`, `avg_value`,
`min_value`, `max_value`, `last_value`, `first_value`, `sample_count`,
`bad_quality_count`.

```sql
-- 1-minute averages for the last 3 hours
SELECT bucket, avg_value, min_value, max_value
FROM signals_1m
WHERE ioa = '1001'
  AND bucket > NOW() - INTERVAL '3 hours'
ORDER BY bucket;

-- hourly trend over the last week
SELECT bucket, avg_value, bad_quality_count
FROM signals_1h
WHERE measurement = 'M_ME_NB_1'
  AND ioa = '1001'
  AND bucket > NOW() - INTERVAL '7 days'
ORDER BY bucket;

-- daily min/max for the last 30 days
SELECT bucket, min_value, max_value, last_value
FROM signals_1d
WHERE ioa = '1001'
  AND bucket > NOW() - INTERVAL '30 days'
ORDER BY bucket;

-- top 10 most active signals in the last hour (by sample count)
SELECT ioa, measurement, SUM(sample_count) AS samples
FROM signals_1m
WHERE bucket > NOW() - INTERVAL '1 hour'
GROUP BY ioa, measurement
ORDER BY samples DESC
LIMIT 10;
```

#### Storage estimate

```sql
SELECT * FROM v_storage_estimate;
```

#### Grafana datasource

Add TimescaleDB as a PostgreSQL datasource.  Use the continuous aggregate views
for dashboard panels — they are far faster than querying raw `signals` for
windows longer than a few minutes.

---

## Development

```
make test           # Go unit tests
make vet            # go vet
pnpm --dir frontend install
pnpm --dir frontend dev    # Vite dev server with hot-reload (proxy to :8080)
```
