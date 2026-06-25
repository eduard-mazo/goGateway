# SSFV e2e — plant-delete reproduction + UI/signal fixes — 2026-06-13

**Verdict: PASS.** A 10-min soak reproduced the reported "plant won't delete /
weird signals" behavior; three source fixes were implemented and validated live
against the same running stack.

---

## 1. Scope

1. Reproduce the plant-delete symptom over a monitored 10-min soak
   (goMqttModbus → goGateway → TimescaleDB) with a mid-run plant delete.
2. Fix, from source:
   - **plant delete** re-surfacing while the producer is live;
   - **individual signal delete** (not possible from the UI);
   - **signal display** as a folder tree built from the metric path.

## 2. Environment

| Component | Detail |
|---|---|
| Producer | goMqttModbus `edge` (host build, DNP3 stub; Modbus active), control API :8090 |
| Modbus sim | `scripts/sim/modbusslave` :1502 |
| Broker | mosquitto :1883 (shared) |
| Consumer | goGateway (this branch), HTTP :8091, SQLite `/tmp/e2e/gw-soak.db` |
| TSDB | timescale/timescaledb:latest-pg16, throwaway :5434, DB `gwtest` |
| Topology | group `EPM_SSFV` → node `EDGE` → device `MODBUS` (5 metrics incl. deep paths `Feeder1/PhaseA/Voltage`, `Substation01/Feeder1/PhaseC/Voltage`) |

The deep-path metrics confirm the FIWARE/UNS decomposition end-to-end:
`Substation01/Feeder1/PhaseC/Voltage` → `codigo_senal=Voltage`,
`nombre_instancia=Substation01/Feeder1/PhaseC` (27 chars).

## 3. Reproduction (soak10.csv)

20 samples × 30 s; planta deleted at sample 8 (T+214 s).

| Phase | valores Δ/30s | plantas visible | planta estado / baja | autodiscovery |
|---|---|---|---|---|
| Pre-delete (1–7) | ~150 (≈5 rows/s) | 2 | 1 / null | MODBUS approved, EDGE+DNP pending |
| Post-delete (8–20) | **0** (frozen at 2050) | 1 | **0 / set** | **unchanged — all rows linger** |

**Findings:**
- The soft-delete itself works: the planta vanishes from `GET /ssfv/plantas`
  and **ingestion stops immediately** (mapping cache reloaded 95→73 entries,
  5→4 equipos; `tbl_valores` frozen for the remaining 6 min). History kept.
- **Root cause of "won't delete / weird":** `deletePlanta` never touched the
  SQLite `autodiscovered_entities` table. The deleted plant's group rows
  lingered (MODBUS orphaned `approved`, EDGE+DNP `pending`) in the **Pendientes**
  tab, kept alive by the live producer; re-approving any revived the planta via
  `ON CONFLICT (broker_base) … fecha_baja=NULL`.
- Signals could only be deleted at the **catalog** level (whole señal); no
  per-instance (binding) delete existed in the UI, and instances rendered as a
  flat table with the raw `nombre_instancia` path string — the "weird" look.

## 4. Fixes (source)

| # | Area | Change |
|---|---|---|
| 1 | `api/ssfv_catalog.go` `deletePlanta` | Capture `broker_base`; after the soft-delete commits, mark that group's `autodiscovered_entities` rows `rejected`. The autodiscovery upsert only revives rows `WHERE status='pending'`, so the live edge can no longer re-surface them. |
| 2 | `api/ssfv_catalog.go` equipo upsert (approve + AutoProvision) | Add `fecha_baja = NULL` to the `ON CONFLICT (nombre_topic) DO UPDATE` — re-adding a deleted plant now revives its equipos (estado=1 alone left them dead → no ingestion). |
| 3 | `frontend/components/SignalTree.vue` (new) + `SSFVView.vue` | Recursive folder tree built from `nombre_instancia` segments, leaf = signal (`codigo_senal`); per-leaf delete button → `DELETE /ssfv/asignaciones/{equisenal_id}`. Replaces both flat per-equipo signal tables. |
| 4 | migration `000018_widen_instancia` | `nombre_instancia` `VARCHAR(30)→VARCHAR(120)` (drop/recreate dependent ssfv views, à la migration 006) so arbitrarily deep `a/b/c/…` paths are storable. |

## 5. Validation (same running stack)

| Check | Result |
|---|---|
| Delete live planta → autodiscovery rows | all 3 `EPM_SSFV` rows → **rejected** |
| Rows persist under continued publishing (45 s, +DBIRTHs) | **still rejected**, pending count **0** |
| Planta hidden from list | ✅ (only planta 1 visible) |
| Re-add planta (re-approve) → equipo | **estado=1, fecha_baja cleared (live)**; ingestion resumed (+5 rows/s) |
| Per-signal delete, binding **with** history | `200 {deactivated:true}`, `activo=f`, history kept |
| Per-signal delete, binding **without** history | `204`, row hard-deleted |
| migration 018 applied; column width | **120**; ssfv views intact |
| Deep-path insert (39 chars) | accepted (would fail at 30) |
| `go build ./...` + `go vet`; `pnpm build` (embed) | clean |

## 6. Notes / follow-ups

- Visual tree rendering was not browser-verified here; the component builds
  clean and the data path (instance → tree, leaf delete) is validated via API.
- DNP3 ran in stub mode (host build); Modbus alone drove the flow. DNP signals
  are unaffected by these changes.

## 7. Artifacts

| File | Content |
|---|---|
| `soak10.csv` | 20 samples × counters, delete at sample 8 |
