# Code review — 2026-04-24

Professional review of the goGateway codebase pre-commit. This pass covered
bugs, vulnerabilities, dead code, and the IEC-104 passive multi-server
refactor the project owner requested.

## Scope

| Area                                   | Action |
|----------------------------------------|--------|
| IEC-60870-5-104 server                 | Rewritten for multi-endpoint + passive semantics |
| DB lifecycle                           | Explicit shutdown sequence, WAL checkpoint, graceful close |
| REST API (`/api/iec104-config`)        | Replaced by `/api/iec104-servers` CRUD |
| Schema                                 | `iec104_config` retired, `iec104_servers` added |
| Bugs                                   | N(S)/N(R) wrap, cfg race, Stop/Reload races, write-lock downgrade |
| Dead code                              | Purged — `ValidateType`, `EnabledTopics`, `armT2`, unused timer fields |
| Frontend                               | `Iec104Config.vue` rewritten as fleet CRUD; status hooks updated |

## Bugs and vulnerabilities fixed

1. **15-bit sequence wrap missing** — `internal/iec104/server.go`. N(S) and N(R)
   are 15-bit per IEC 60870-5-104; previous code advanced them with plain
   `uint16` arithmetic. After 32 768 frames any SCADA master would have
   rejected the stream as out-of-sequence. Fixed with a `seqMask` (`0x7FFF`)
   applied on every increment and every peer-value parse.
2. **Race on config fields** — `reader()` / `timerLoop()` read `cfg.T2` /
   `cfg.T3` without a lock while `Reload` could overwrite them. The config
   is now snapshotted per-connection at `Accept` time so timers are immutable
   for the life of the client conn.
3. **`Dispatch` took a write lock for a read-only path.** Downgraded to
   `RLock`, reducing contention under heavy ingest.
4. **`Stop` did not wait for client goroutines.** Added a `sync.WaitGroup`
   joined after listener + connections close, eliminating the brief window
   where `Close` could race with an in-flight write.
5. **`replaceAck` and `handleInterrogation` read cfg.ASDUAddr without a
   lock.** Now use `RLock`.
6. **Mapping update endpoint did not default `scale=0 → 1.0`** while create
   did. Aligned behavior in `internal/api/mappings.go`.
7. **Shutdown ordering hazard** — History logger wrote to the DB while
   `database.Close()` ran in `defer`. Replaced with explicit sequence:
   stop HTTP → MQTT → IEC → cancel ctx → wait for history drain → WAL
   checkpoint → close DB.
8. **WAL never truncated on exit** — operators saw multi-MB `-wal` files
   after clean shutdowns. `PRAGMA wal_checkpoint(TRUNCATE)` is now issued
   before `Close()`.

## IEC 60870-5-104 — passive multi-server

Requirements:

- The gateway is **passive only**: never initiates outbound TCP.
- Multiple SCADA masters must be able to consume the same point set, each
  under a different Common ASDU Address.

Design:

- `iec104.Manager` owns N `NativeServer` instances keyed by DB row id.
- A single `pointStore` is shared across instances so a point dispatched
  from MQTT is visible to every master at its own ASDU without duplication.
- `Reload(cfgs []models.IEC104Server)` diffs the fleet: new row → start,
  removed row → stop, endpoint changed → restart, `enabled` toggled →
  start/stop. All other field changes take effect on the next client
  connection.
- The `iec104.Server` interface used by MQTT/API is unchanged aside from
  `Reload` taking a slice. Callers that don't care about per-instance state
  see one aggregated `Status`.

Passive semantics are enforced by the package never calling `net.Dial`. The
package-level doc comment states this explicitly. U-frame CON variants are
accepted only for protocol-hygiene and deliberately never originated by the
gateway — a comment in `handleU` documents that.

## Schema migration

`iec104_config` (singleton, id=1) → `iec104_servers` (N rows, unique
`(listen_addr, port)`). Migration is idempotent:

- `CREATE TABLE IF NOT EXISTS iec104_servers ...`
- Default row inserted with `INSERT OR IGNORE`.
- `DROP TABLE IF EXISTS iec104_config`.

No data loss for greenfield installs. Existing deployments that relied on
non-default timers in `iec104_config` will need to re-enter them in the new
fleet UI — considered acceptable given the project has never shipped.

## Dead code removed

- `iec104.ValidateType` — no caller in the repo.
- `worker.EnabledTopics` — duplicate of `Topics()`.
- `clientConn.armT2` and the unused `t2Fire`/`t3Fire` fields.
- Duplicate U-frame encodings (`uSTARTDT`, `uSTOPDT`) the server never
  originates.
- `COT_PERIODIC`, `COT_BACKGROUND`, `COT_INIT`, `COT_REQ`, `COT_DEACT`,
  `COT_DEACT_CON` — constants defined but never encoded. Kept the ones
  actually emitted (`SPONTANEOUS`, `ACTIVATION`, `ACT_CON`, `ACT_TERM`,
  `INTROGEN`).

## Frontend

- `src/api.ts` — `IEC104Config` → `IEC104Server`.
- `src/composables/useStatus.ts` — `IEC104Status` now exposes
  `servers: IEC104ServerStatus[]` plus fleet-aggregated `running`,
  `points`, `clients`.
- `src/views/Iec104Config.vue` — full rewrite. Lists the fleet, add/edit
  dialog, inline enable toggle, per-row runtime status pill, live client
  counts.
- `src/App.vue` and `src/views/Dashboard.vue` — summary chips now render
  "N/M running" when more than one endpoint is configured.

## Build and test

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./...` — `internal/worker` passes; no other packages carry tests
  in-tree.
- `pnpm --dir frontend build` — clean (vue-tsc + vite).
- Smoke: boots on a fresh DB, `/api/iec104-servers` lists the default row,
  `/api/status` returns the new fleet shape, SIGTERM triggers the documented
  shutdown sequence and leaves `gateway.db` checkpointed (no `-wal`/`-shm`
  remnants).
