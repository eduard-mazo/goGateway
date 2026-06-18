# FIWARE Orion sink (NATS fan-out spine — increment 1)

First slice of [nats-role-options.md](nats-role-options.md) **#1**: decoded SSFV
samples are published once to the NATS JetStream spine and an Orion consumer
upserts them into a FIWARE context broker (NGSIv2). The direct TimescaleDB write
is unchanged and remains the source of truth — Orion is an *additional*,
decoupled sink.

```
MQTT → decode → SSFVHandler.HandleMetric
        ├─ TimescaleDB (direct, source of truth)        [unchanged]
        └─ NatsSSFVPublisher → JetStream "{stream}.ssfv.sample"
               └─ OrionWorker (durable consumer) → Orion  POST /v2/op/update
```

## Components

- `internal/worker/ssfv_sample.go` — `SSFVSample` + `NatsSSFVPublisher`
  (best-effort publish; a NATS error never blocks ingestion).
- `internal/fiware/orion.go` — NGSIv2 `HTTPClient`; idempotent upsert via
  `POST /v2/op/update` (`actionType: append`). Entity id is a stable URN:
  `urn:ngsi-ld:<type>:<group>:<node>[:<device>]`; the signal `codigo` is the
  attribute, with `instance`/`quality`/`TimeInstant` metadata.
- `internal/worker/orion_worker.go` — durable JetStream consumer
  (`Durable: "orion-worker"`, filter `{stream}.ssfv.>`); at-least-once delivery is
  safe because the upsert is idempotent (Ack on success, Nak to retry, Term on
  poison JSON).

## Enabling it

Requires NATS enabled (`nats_config.enabled=1`; the publisher and worker use the
JetStream stream). Orion itself is configured by environment (MVP):

| Env | Default | Meaning |
|---|---|---|
| `ORION_ENABLED` | `0` | `1`/`true` turns the sink on |
| `ORION_URL` | `http://localhost:1026` | context-broker base URL |
| `ORION_ENTITY_TYPE` | `SSFVEquipo` | NGSI entity type |
| `ORION_SERVICE` | — | `Fiware-Service` tenant header |
| `ORION_SERVICE_PATH` | — | `Fiware-ServicePath` header |

When `ORION_ENABLED` is set but NATS is unavailable, the sink is skipped with a
log warning (it consumes the stream). The SSFV→NATS publisher is wired only when
both NATS and Orion are on, so an idle stream isn't filled with unconsumed data.

## Validated

Smoke test (NATS + Orion stub + the soak stack): `nats: connected`,
`Orion worker started`, and the stub received NGSIv2 upserts with the expected
URN, attribute, and metadata — while TimescaleDB ingestion continued unaffected.
Unit tests cover the NGSIv2 request shaping/URN mapping (`fiware`) and the
sample/subject helpers (`worker`).

## Not in this increment (follow-ups)

- DB-backed config + UI for Orion (today it is env-only).
- Batching upserts (`/v2/op/update` accepts many entities) instead of one POST
  per sample.
- Moving TimescaleDB and IEC-104 *behind* consumers too (full spine), and
  retiring the bespoke SSFV WAL in favor of the JetStream log.
