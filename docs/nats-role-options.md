# NATS in goGateway: current role and two ways to make it central

Decision note comparing **#1 fan-out spine** vs **#2 front-door buffer** — what
each solves, failure modes, and effort — so we can choose before implementing.

> Grounded in `cmd/gateway/main.go`, `internal/worker/{dispatcher,nats_workers}.go`,
> `internal/nats/client.go`, `internal/worker/ssfv_handler.go`,
> `internal/tsdb/{pipeline,adapter_ssfv}.go`.
> Related: [`data-gap-recovery.md`](data-gap-recovery.md).

---

## Current role (baseline)

NATS is an **optional, config-gated** (`nats_config.enabled`, default off) fan-out
layer on the **IEC-104 path only**:

```
MQTT → decode → FilteringDispatcher → NatsDispatcher.Publish()
   → JetStream "{stream}.metrics.{serverID}.{IOA}" (FileStorage, 24h, LimitsPolicy)
       ├─ SCADAWorker → IEC-104 slaves
       └─ TSDBWorker  → HistoryLogger → generic public.signals
```

- Disabled → `DirectDispatcher` (straight to IEC-104 + history); NATS untouched.
- It sits on the `Dispatcher` interface, i.e. the **IEC-104 mapping path**
  (a signal needs an IOA mapping to reach it).
- The **SSFV / Sparkplug → TimescaleDB path bypasses it entirely**
  (`ssfvHandler.HandleMetric` → SSFV pipeline → `ssfv.tbl_valores`, own WAL/DLQ).

Net: in a solar/SSFV deployment NATS is nearly dormant — a durable queue in front
of IEC-104 only.

---

## Option #1 — NATS as the decode→sink fan-out spine

Publish each **decoded** sample to JetStream **once**, then fan out to independent
durable consumers.

```
MQTT → decode (+Sparkplug seq validation) → publish ONE canonical sample
   → JetStream  gw.samples.{ssfv|iec104}.…
       ├─ TSDB consumer    → ssfv.tbl_valores
       ├─ IEC-104 consumer → slaves
       └─ Orion consumer   → FIWARE Context Broker   ← the new sink
```

**Solves**
- Decouples sinks: Orion/TSDB/IEC-104 can be slow or down without back-pressuring
  ingestion or each other.
- One durable log replaces the bespoke per-sink WAL/DLQ; new sink = new consumer,
  no rewiring of decode. Replay/reprocessing for free.
- Directly serves the `feature/fiware-orion-integration` work.

**Does NOT solve**
- The inbound gap (gateway down → nothing is published to NATS either). Same
  limitation as today.

**Failure modes / caveats**
- NATS becomes critical: keep the `DirectDispatcher` fallback so a NATS outage
  degrades to direct writes rather than halting ingestion.
- JetStream is **at-least-once** → consumers must be idempotent. Already true:
  TSDB `ON CONFLICT DO NOTHING`, IEC-104 is last-value (dup harmless), Orion is
  an upsert. Sparkplug seq is validated *before* publish (in the handler), so the
  stream carries only accepted samples.
- New ops surface: stream storage sizing, consumer-lag monitoring.

**Effort:** medium, code-local. New publish step + 2–3 consumers + idempotency +
config + fallback. Reworking the SSFV write to go via a consumer is the bulk.
No broker/infra change.

---

## Option #2 — NATS-server (MQTT + JetStream) as the durable front door

`nats-server` speaks MQTT 3.1.1 natively, backed by JetStream. Point producers at
its MQTT endpoint; inbound QoS-1 messages persist in a stream; the gateway becomes
a **durable JetStream consumer that resumes from its last ack** on restart.

```
producer --MQTT QoS1--> nats-server (MQTT endpoint, JetStream-persisted)
                          │  messages survive a gateway restart
                          ▼
                    goGateway durable consumer (explicit ack) → decode → sinks
```

**Solves**
- The **original gap-recovery problem**: the lost minute is retained in the stream
  and redelivered from last ack when the gateway returns. The *only* option here
  that recovers consumer-downtime gaps without changing the producer.

**Failure modes / caveats**
- **Sparkplug vs replay (the hard one):** on a gateway *process* restart the
  in-memory alias/seq state is gone. Replayed NDATA reference aliases seeded by an
  NBIRTH that was acked long ago and won't be redelivered → unresolved. Making
  replay actually work requires **persisting the alias/seq map** across restart
  (then back-dated samples insert idempotently). Without that you fall back to a
  rebirth and lose the gap again — defeating the point.
- MQTT feature parity: nats-server MQTT is 3.1.1, QoS 0/1 only (no QoS2); retained
  + LWT are supported (STATE works) but must be validated against the Sparkplug
  flows.
- Ops/migration: run nats-server as the **production broker**, migrate every
  producer's endpoint, enable JetStream + accounts; clustering/HA to design.

**Effort:** high. Infra + broker migration + gateway consumer rework + alias/seq
persistence.

> Note: if gap recovery is the real goal, **producer-side store-and-forward keyed
> on the STATE topic** (see `data-gap-recovery.md`) is *less* invasive than #2 and
> Sparkplug-idiomatic — #2's complexity may not beat it.

---

## Side by side

| | #1 Fan-out spine | #2 Front-door buffer |
|---|---|---|
| Recovers consumer-downtime gap | ❌ | ✅ (with alias/seq persistence) |
| Decouples sinks / adds Orion cleanly | ✅ | ➖ (orthogonal) |
| Replaces bespoke SSFV WAL | ✅ | ➖ |
| Fights Sparkplug RBE model | low | **high** (replay vs aliases) |
| Code effort | medium | high |
| Infra / broker change | none | **broker migration** |
| Blast radius if NATS down | degrade to DirectDispatcher | **ingestion stops** (NATS is the broker) |
| Serves the Orion branch | **directly** | indirectly |

## Recommendation

- **#1** is the higher-value, lower-risk move and the natural fit for the Orion
  integration: one durable decoded stream feeding TSDB + IEC-104 + Orion, with a
  direct-write fallback. It does **not** address the inbound gap.
- For the inbound gap, prefer **producer store-and-forward** (`data-gap-recovery.md`)
  over **#2**; reserve #2 for when a single durable broker/HA backbone is wanted
  for its own sake — and budget the alias/seq-persistence work.
- #1 and the producer-side gap fix **compose**; #2 largely overlaps both at much
  higher cost.
