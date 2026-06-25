# Data-gap recovery when the gateway is down

Scenario: the producer (goMqttModbus) publishes every second at **QoS 1** to the
broker; **goGateway goes down and comes back ~1 minute later**. What happens to
that minute of data, and how to make it recoverable.

> Grounded in `internal/mqtt/client.go`, `internal/worker/sparkplug_dispatch.go`,
> `internal/tsdb/{pipeline,adapter_ssfv}.go`, `internal/worker/nats_workers.go`.
> Related: [`sender-identity-and-collisions.md`](sender-identity-and-collisions.md),
> [`ARCHITECTURE_goGateway.md`](ARCHITECTURE_goGateway.md).

---

## What happens today: the minute is LOST

- The gateway connects with **`CleanSession = true`** (paho default — there is no
  `SetCleanSession(false)`). When it disconnects the broker **discards its
  session + subscriptions** and queues nothing for it.
- The producer's QoS 1 is acknowledged by the **broker** (PUBACK), so the
  producer believes delivery succeeded — but with no subscriber session the
  broker has nowhere to queue, so the ~60 messages are **dropped at the broker**.
- On restart the gateway re-subscribes fresh and (Sparkplug) requests a
  **REBIRTH**, recovering *current* values — not the 60 missed samples.

**QoS 1 only guarantees producer→broker.** It says nothing about a subscriber
that is offline under a clean session.

## Why the existing durability layers don't cover this

| Mechanism | What it protects | Helps the gap? |
|---|---|---|
| **NATS / JetStream** | the gateway's *internal* MQTT→IEC-104 handoff (generic JSON path); the SCADA worker can replay if it lags/restarts | **No** — it's external + downstream; the SSFV/Sparkplug→TSDB path doesn't use it, and while the gateway process is down nothing publishes to NATS |
| **TSDB WAL / DLQ** (`pipeline.go`) | gateway→TimescaleDB writes during a *TSDB* outage (idempotent WAL replay) | **No** — downstream of ingestion |
| **MQTT QoS 1** | producer→broker delivery | **No** — broker acks; offline clean-session subscriber gets nothing |

## What already exists that a fix can use

- The gateway publishes **`STATE/{hostID}` = ONLINE / OFFLINE** (retained, QoS 1;
  OFFLINE is the Last-Will). A producer can *know* when the consumer is down.
- SSFV writes are **`ON CONFLICT (timestamp_utc, equisenal_id) DO NOTHING`** using
  each message's own timestamp — so **back-dated/historical samples are accepted
  idempotently**. Backfill is safe and gap-filling at the DB level.

## Options to make it robust (ranked)

1. **Producer store-and-forward keyed on STATE (robust, Sparkplug-idiomatic).**
   goMqttModbus subscribes to `STATE/{hostID}`; while the host is OFFLINE it
   buffers samples locally and, on ONLINE, republishes them as **historical**
   metrics (`is_historical=true`) with their original timestamps. The gateway's
   idempotent back-dated insert fills the gap exactly once. This is the *correct*
   fix: QoS 1 is acked by the broker, so only STATE reveals the consumer outage.
   Cross-repo (producer feature + a small gateway check that historical metrics
   flow through `dispatchMetric` → SSFV with their payload timestamp).

2. **Gateway persistent MQTT session (quick, partial).** `SetCleanSession(false)`
   + stable clientId + `SetResumeSubs(true)` so the broker queues QoS 1 for the
   offline subscriber and redelivers on reconnect. Recovers brief disconnects.
   Caveats: depends on broker session persistence + queue limits
   (`max_queued_messages`, expiration) — a long outage overflows and drops; and
   across a full **process** restart the in-memory Sparkplug seq/alias state is
   lost, so replayed NDATA arrive unresolved/out-of-sequence → rebirth anyway
   (and may trip the duplicate-node detector). Best for network blips where the
   process stays alive.

3. **Durable buffer/bridge in front of the gateway** (persistent broker with deep
   queues, or an MQTT→JetStream bridge that holds the subscriber backlog).
   Infra-level; heaviest.

**Recommendation:** (1) for true per-sample recovery; (2) as a cheap mitigation
for short disconnects. (1) and (2) compose.
