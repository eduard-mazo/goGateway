# E2E soak results — 2026-06-11

Three monitored soaks of the full pipeline (goMqttDnp3 edge → mosquitto →
goGateway → TimescaleDB), per the soak procedure in
[`../../e2e-test-guide.md`](../../e2e-test-guide.md) §6. Sample interval 30 s;
columns are cumulative counters.

**Topology:** group `EPM_SSFV`, node `EDGE` (`plantaAlias: GSANRAFA`), device
`DNP` (opendnp3 sim :20000, real `dnp3_ffi` master, Class-1 events every 2 s),
device `MODBUS` (`scripts/sim/modbusslave` :1502, 1 s scan), system telemetry
CPU-only (`SYSTEM/CPU/Usage_pct`, 5 s). Catalog: 5 señales registered via the
contract-v3 approve flow (leaf `codigo_senal` + folder `nombre_instancia`,
planta created inline via `create_planta`).

| Run | Duration | Producer | Consumer | Result |
|---|---|---|---|---|
| 1 `soak1-baseline` | 10 min | pre-fix | pre-fix | **15 rebirth storms** (~1.5/min): seq wrapped 255→1 (skipping 0) → guaranteed out-of-sequence every 255 msgs; each gap fired **2** NCMDs (NDATA+DDATA detectors, no debounce) → 15 NBIRTH + 30 DBIRTH re-publishes. DNP3 data stalled after the startup integrity poll (sim had no event buffer). DB writes healthy: 2.3 rows/s linear, 0 errors. |
| 2 `soak2-seq-fix` | 5 min | seq fixes (`b6b45b4`) | debounce (`6ce5f63`) | **0 rebirths**, including a live 255→0→1 wrap mid-soak. DNP3 still on `integrityScanMs` workaround. 2.6 rows/s linear. |
| 3 `soak3-full-fix` | 10 min | seq fixes | debounce + migration runner fix (`647c63c`) | **0 rebirths / 0 out-of-seq / 0 birth re-publishes.** DNP3 event-driven (sim rebuilt with `EventBufferConfig::AllTypes(50)`, no integrity workaround): ~1 event/2 s per point. 3.2 rows/s linear, 2 028 rows / 456 KB hypertable, errorRate 0, DLQ 0, WAL 0. Postgres logs: 10 materialization lines in 20 min (single cagg set, 5 m/30 m cadence) vs paired 0-row lines every 30 s from duplicated jobs before. |

Postgres-side finding (run 1→3): the unqualified `schema_migrations`
bookkeeping re-ran all migrations on every gateway restart under the SSFV
pool's `search_path`, duplicating the generic pipeline (signals + caggs +
refresh/retention/columnstore jobs) inside the `ssfv` schema. Fixed in
`migrate.go` + migration `000017_dedupe_runner_artifacts` — see the guide's
soak section for the full history.

`dbperfN-*.txt` are the end-of-run TimescaleDB snapshots (hypertable size, row
counts, per-table insert stats).
