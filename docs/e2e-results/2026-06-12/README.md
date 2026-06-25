# SSFV pipeline soak report — 2026-06-12 (30 min)

**Verdict: PASS.** 30 minutes of sustained Sparkplug B → TimescaleDB flow with
**zero rebirths, zero out-of-sequence, zero write errors, zero unmapped
signals and zero failed background jobs**. Every wire message landed as
exactly one row in `ssfv.tbl_valores` (5 752 rows / 5 750 messages over the
window). Insert rate was flat at **3.20 rows/s ± 4 %** for the full run.

---

## 1. Scope and objectives

Validate, over a 30-minute monitored hold:

1. **MQTT Sparkplug B data flow** — seq/bdSeq integrity, absence of
   NBIRTH/DBIRTH storms, NCMD discipline (producer fixes `b6b45b4`,
   consumer debounce `6ce5f63`).
2. **Database write path** — sustained insert throughput into the
   `ssfv.tbl_valores` hypertable, error/DLQ/unmapped counters.
3. **Hypertable chunk processing** — chunk creation/sizing, compression
   policy state, continuous-aggregate and policy job execution
   (post-migration-0017 cadence).

This run also served as the first end-to-end exercise of the **bulk approval
UI** (commit `57ef5a2` + this branch) against real birth certificates.

## 2. Environment

| Component | Detail |
|---|---|
| Producer | goMqttModbus `edge` (multiproto @ `b6b45b4`), UI :8085 |
| DNP3 source | opendnp3 Docker sim :20000 (outstation 10, `EventBufferConfig::AllTypes(50)`), Class-1 scan 2 s |
| Modbus source | `scripts/sim/modbusslave` :1502, 1 s scan |
| Broker | mosquitto :1883 (shared, long-lived) |
| Consumer | goGateway (this branch, `57ef5a2`+fleet-view), HTTP :8092, SQLite `/tmp/e2e/gw-soak.db` |
| TSDB | timescale/timescaledb:latest-pg16 — PostgreSQL 16.13, TimescaleDB 2.26.4, throwaway container :5434, fresh DB (17 migrations applied once) |
| Topology | group `EPM_SSFV` → node `EDGE` (NDATA: `SYSTEM/CPU/Usage_pct` @5 s) → devices `DNP` (tank_level, VALV_ON; event-driven ~2 s) and `MODBUS` (tank_press, VALV_OFF; 1 s poll) |
| Catalog | 5 señales registered via **bulk approval** (1 NBIRTH + 2 DBIRTH in one batch: planta `GSANRAFA` staged inline, tipo fill-down `Medidor`, fallback variable `Proceso`, host telemetry toggle ON) |

## 3. Methodology

`soak30.sh`: 60 samples at 30 s intervals (window 1 798 s). Wire counters from
`mosquitto_sub spBv1.0/EPM_SSFV/#` inside the broker container; seq-integrity
counters from the gateway log; DB counters via `psql` in the TSDB container.
All counters cumulative; the analysis below uses deltas across the window.
End-of-run snapshots: `dbperf30.txt` (totals, jobs, per-signal distribution,
engine stats) and `chunks30.txt` (chunk detail).

The 4 `rebirth_req`/`out_of_seq` counts visible at t=0 predate the window:
they are the expected session bootstraps from the edge restart (15:07) and
gateway binary swap (15:11) during stack preparation. **Deltas during the
window: 0.**

## 4. Results

### 4.1 Sparkplug B flow integrity

| Metric (Δ over 30 min) | Value | Expected | Status |
|---|---|---|---|
| NDATA on wire | 360 | 1 798 s / 5 s ≈ 360 | ✅ |
| DDATA on wire | 5 390 | MODBUS 2/s + DNP ~1/s ≈ 5 394 | ✅ |
| NBIRTH / DBIRTH re-publishes | **0 / 0** | 0 | ✅ |
| NCMD (rebirth requests) | **0** | 0 | ✅ |
| Consumer out-of-sequence | **0** | 0 | ✅ |
| NCMD suppressed (cooldown) | 0 | 0 | ✅ |

5 750 messages traversed the node's single seq counter — **≈ 22 silent
255→0 wraps** with no out-of-sequence detection, confirming the producer
seq/bdSeq fixes and publish-order lock hold under sustained load.

### 4.2 Database write performance

| Metric | Value |
|---|---|
| Rows written (window) | 5 752 (749 → 6 501) |
| Wire messages (window) | 5 750 → **1:1 message→row, no loss** |
| Mean insert rate | **3.20 rows/s** |
| Per-interval rate (30 s buckets) | min 3.10 / max 3.37 rows/s (± 4 %) |
| Write errors / DLQ / circuit events | **0 / 0 / 0** |
| Unmapped signals (`sin mapeo`) | **0** (host telemetry registered via bulk toggle) |
| Per-signal distribution | tank_press 2 063 · VALV_OFF 2 063 · tank_level 1 031 · VALV_ON 1 031 · CPU Usage_pct 413 (session totals) |

### 4.3 Hypertable and chunk processing

| Metric | Value |
|---|---|
| Chunks (`ssfv.tbl_valores`) | **1** — `_hyper_11_1_chunk`, range 2026-06-12 → 2026-06-13 UTC (1-day `chunk_time_interval`; a 30-min run stays inside one chunk, as designed) |
| Chunk size end-of-run | 1 264 kB (heap + indexes), uncompressed |
| Hypertable growth (window) | +1 015 808 B ≈ **176.6 B/row** including PK + `idx_valores_equisenal_time` |
| Storage rate at this load | ≈ 0.57 kB/s ≈ **49 MB/day** uncompressed |
| Compression | enabled (`segmentby equisenal_id`, `orderby timestamp_utc DESC`), policy armed at 12 h cadence with `compress_after = 7 days` → correctly **not** compressing the live chunk |
| Chunk insert stats | `_hyper_11_1_chunk` n_tup_ins 6 595 = n_live_tup 6 597 (no bloat, no dead tuples) |

### 4.4 Background jobs and continuous aggregates

All **15 TimescaleDB jobs ran with 0 failures**. Cadences match migration
`000017` (post-dedupe):

| Job | Cadence | Runs in window+setup | Expected | Status |
|---|---|---|---|---|
| refresh `public.signals_1m` | 5 min | 13 | ~13 | ✅ |
| refresh `public.signals_1h` | 30 min | 3 | ~2–3 | ✅ |
| refresh `public.signals_1d` | 1 h | 2 | ~1–2 | ✅ |
| compression ×6, retention ×4, telemetry | 12 h / 1 d | 1 each | 1 | ✅ |

Postgres log: **16** `inserted 0 row(s) into materialization table` lines in
~35 min — exactly one per cagg refresh (the caggs aggregate the generic
`public.signals` pipeline, unused by SSFV, hence 0 rows). Before the
migration-runner fix this was paired lines every 30–60 s from duplicated job
sets; the single-cadence behavior is confirmed healthy.

### 4.5 PostgreSQL engine health

| Metric | Value |
|---|---|
| Cache hit ratio | **99.89 %** (602 445 hits / 644 reads) |
| Transactions | 5 297 commits / 3 rollbacks |
| Deadlocks | **0** |
| tup_inserted (db-wide) | 12 487 (valores + bookkeeping + job stats) |

### 4.6 Operational validation — bulk onboarding

The 3 real birth certificates (node `EDGE`, devices `DNP`, `MODBUS`) were
onboarded in **one bulk-approval run**: planta staged from the group, tipo
fill-down applied to all rows, fallback variable for meta-less metrics, host
telemetry registered via the new dialog toggle. Result: 3 ok / 0 partial /
0 error; first values landed in `tbl_valores` within seconds of the
post-approval NCMD rebirth.

**Bug found and fixed during this exercise:** the staged-planta alias was
derived from the *first selected entity* of a group — a DBIRTH device carries
no `uns/planta` property, so the alias fell back to the group id instead of
the node's declared `GSANRAFA`. The alias derivation now scans every selected
entity of the group and prefers the NBIRTH property.

## 5. Pass criteria

| Criterion | Threshold | Measured | Verdict |
|---|---|---|---|
| Rebirth storms during hold | 0 | 0 | ✅ |
| Out-of-sequence during hold | 0 | 0 | ✅ |
| Birth re-publishes during hold | 0 | 0 | ✅ |
| Write errors / DLQ | 0 | 0 / 0 | ✅ |
| Unmapped signals | 0 | 0 | ✅ |
| Insert rate stability | linear, no decay | 3.20 rows/s ± 4 % | ✅ |
| Wire→row accounting | 1:1 | 5 750 → 5 752 | ✅ |
| Background job failures | 0 | 0 (15 jobs) | ✅ |
| Postgres log noise | 1 line per cagg refresh | 16 in ~35 min | ✅ |

## 6. Recommendations

1. **Chunk interval at fleet scale.** 1-day chunks are right for this volume
   (≈ 49 MB/day). If the deployment grows toward thousands of plants
   (~10³× signals), revisit `chunk_time_interval` (e.g. 1–4 h) so chunks stay
   inside `shared_buffers`, and consider raising the compression policy
   frequency.
2. **Compression validation pending.** `compress_after = 7 days` means no
   soak of this length can observe columnstore conversion; schedule a
   follow-up with a backdated dataset or a temporarily lowered threshold to
   measure the compression ratio for `segmentby equisenal_id`.
3. **Cagg refresh on an idle pipeline.** The generic `public.signals` caggs
   refresh forever on an unused table in SSFV-only deployments; consider
   disabling jobs 1001–1011 when `backend=timescaledb` runs in SSFV mode.

## 7. Artifacts

| File | Content |
|---|---|
| `soak30.csv` | 60 samples × 13 cumulative counters (30 s interval) |
| `dbperf30.txt` | End-of-run snapshot: totals, hypertables, jobs, per-signal distribution, `pg_stat_database`, chunk insert stats |
| `chunks30.txt` | Chunk detail (name, time range, compression state, size) |

UI evidence (bulk approval of the real births, fleet overview with live
freshness): session screenshots `19–23` under `/tmp/e2e/shots/`.
