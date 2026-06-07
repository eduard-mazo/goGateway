# Sparkplug B Contract — goMqttDnp3 ⇄ goGateway

> **Interface contract** between the **goMqttDnp3** gateway (Sparkplug B
> *producer* — Modbus, DNP3, and host/System telemetry) and the **goGateway**
> backend (Sparkplug B *consumer* — TSDB dispatch). This document is the single
> source of truth for the payloads on the wire. The JSON examples under
> [`schema/`](schema/) are **decoded** representations for documentation; the
> actual MQTT payload is Sparkplug B **protobuf** (Eclipse Sparkplug 2.2 /
> Appendix-1 §15).

Both apps use independent hand-rolled codecs (no protoc); this contract keeps
them byte-compatible.

---

## 1. Transport & topic namespace

- **Transport:** MQTT 3.1.1, payload = Sparkplug B protobuf (binary).
- **Topic:** `spBv1.0/{group_id}/{message_type}/{edge_node_id}[/{device_id}]`

| Message | Topic | Emitted when |
|---|---|---|
| `NBIRTH` | `spBv1.0/{grp}/NBIRTH/{node}` | On MQTT connect / rebirth. Declares **all** node metrics + `bdSeq`. |
| `NDATA` | `spBv1.0/{grp}/NDATA/{node}` | Node-metric value change (DNP3/Modbus node mappings + System). |
| `DBIRTH` | `spBv1.0/{grp}/DBIRTH/{node}/{device}` | After NBIRTH, once per child device. Declares that device's metrics. |
| `DDATA` | `spBv1.0/{grp}/DDATA/{node}/{device}` | Device-metric value change. |
| `NDEATH` | `spBv1.0/{grp}/NDEATH/{node}` | MQTT LWT and graceful shutdown. Carries `bdSeq`. |
| `DDEATH` | `spBv1.0/{grp}/DDEATH/{node}/{device}` | Device removed/offline. |
| `NCMD` | `spBv1.0/{grp}/NCMD/{node}` | **Inbound** — `Node Control/Rebirth=true` triggers a fresh NBIRTH. |

`group_id` = `sparkplug.groupId`, `edge_node_id` = `sparkplug.nodeId` (config).

### Node vs device scoping

A metric is a **device** metric iff its `SignalMapping.deviceId` is non-empty
(the Sparkplug device ID); otherwise it is a **node** metric. This is
independent of protocol — Modbus and DNP3 mappings can be either. **System/host
telemetry is always node-scoped.**

---

## 2. Session rules (sequence, bdSeq, aliases)

- **`bdSeq`** — `uint64` metric in every NBIRTH and the matching NDEATH. Lets the
  consumer pair a death with its birth. Increments per (re)birth, wraps at 256.
- **`seq`** — payload field, `0` in NBIRTH, then `1..255` wrapping for every
  subsequent N/DDATA & DBIRTH. A gap means a missed message → consumer should
  send `NCMD Node Control/Rebirth=true`.
- **Alias compression** — NBIRTH/DBIRTH carry each metric's **`name` + `alias`**.
  Subsequent NDATA/DDATA carry **`alias` only** (name omitted). **Consumers MUST
  seed an `alias → name` map from the latest BIRTH** and resolve DATA by alias.
  Aliases reset on every NBIRTH; they are unique within a node session (node and
  device metrics share one counter).

> goGateway implements this in `internal/sparkplug/session.go`
> (`NodeSession`/`DeviceSession.ResolveName`).

---

## 3. Datatypes

Sparkplug datatype codes (payload field `datatype`, Appendix-1 §15.2.1):

| Code | Type | Used for |
|---|---|---|
| 7 | `UInt32` | DNP3 double-bit binary; DNP3 counter/frozen-counter **without** scale/offset |
| 8 | `UInt64` | `bdSeq` |
| 10 | `Double` | DNP3 analog/analog-output; counters **with** scale/offset; **all** Modbus registers; **all** System numeric metrics |
| 11 | `Boolean` | DNP3 binary/binary-output-status; Modbus coil/discrete-input |
| 12 | `String` | DNP3 octet-string; System device-identity metrics |

**Rule:** a metric's `datatype` in NBIRTH/DBIRTH is **identical** to the
`datatype` it later sends in NDATA/DDATA. (Enforced by `mapping.BirthMetric`,
which mirrors `mapping.Apply`.)

---

## 4. Metric naming

| Source | Name | Notes |
|---|---|---|
| **DNP3** | `SignalMapping.metricName` (user-defined, free-form) | e.g. `Feeder1/Voltage`. Identity is `(pointType, index)` on the outstation. |
| **Modbus** | `SignalMapping.metricName` (user-defined) | e.g. `Meter1/Energy_kWh`. Identity is `(function, address)`. |
| **System (numeric)** | `{metricPrefix}{group}/{leaf}` | Default prefix `System/`. e.g. `System/CPU/Usage_pct`, `System/Memory/Used_pct`, `System/Disk/<mount>/Free_MB`, `System/Network/<iface>/RxRate_kbps`, `System/Temperature/CPU_C`, `System/Power/Supply_V`, `System/Power/RTC_Battery_OK`, `System/Uptime_h`, `System/Process/Count`. |
| **System (identity, ICR build)** | `{prefix}Device/...` (String) | `System/Device/PartNumber`, `ProductType`, `ProductName`, `Firmware`, `Serial`, `UUID`. |

See the goMqttDnp3 repo's `ARCHITECTURE.md` §5 and its `sysmon` package for the
full System leaf list.

---

## 5. Metric properties (PropertySet, payload field 9)

Every NDATA/DDATA value metric carries a `PropertySet`. **Values are typed**
(not all strings) — consumers must read the typed PropertyValue fields:

| Key | PropertyValue type | Meaning |
|---|---|---|
| `quality` | `UInt32` (3) | SCADA quality: **192** = GOOD, **64** = STALE/uncertain, **0** = BAD. |
| `dnp3.flags` | `UInt32` (3) | Raw DNP3/IEEE-1815 flag byte. |
| `dnp3.online` | `Boolean` (7) | Online bit set. |
| `dnp3.restart` | `Boolean` (7) | Restart bit set. |
| `dnp3.comm_lost` | `Boolean` (7) | Comm-lost bit set. |
| `engUnit` | `String` (8) | Engineering unit, when configured (e.g. `V`, `kWh`, `degC`). |
| `uns/code` | `String` (8) | **UNS Attribute** — the canonical signal code (→ consumer catalog `codigo_senal`). Declared in NBIRTH/DBIRTH (and echoed on data). |
| `uns/instance` | `String` (8) | **UNS entity instance / channel** (→ consumer `nombre_instancia`). `default` when the metric has no instance dimension. |

PropertyValue value-field numbers: `int_value`=3, `long_value`=4,
`float_value`=5, `double_value`=6, `boolean_value`=7, `string_value`=8.

### 5.1 UNS / FIWARE decomposition (`uns/*`) — universal

A Sparkplug metric name is a flat string; the **producer** knows its true
structure and declares it explicitly via `uns/code` + `uns/instance` so any
consumer maps it to an **Entity → Attribute** model deterministically, **without
parsing the name**. This is protocol- and domain-agnostic (Modbus, DNP3, System,
any). The **Entity** is the MQTT topic node (`…/NDATA/node`) or device
(`…/DDATA/node/device`); `uns/instance` is the sub-channel *within* that entity;
`uns/code` is the attribute.

| Metric name (example) | `uns/code` | `uns/instance` |
|---|---|---|
| `tank_level` (Modbus node) | `tank_level` | `default` |
| `Energy_kWh` (device) | `Energy_kWh` | `default` |
| `System/CPU/Usage_pct` | `CPU/Usage_pct` | `default` |
| `System/Disk/root/Used_pct` | `Disk/Used_pct` | `root` |
| `System/Network/eth0/Rx_MB` | `Network/Rx_MB` | `eth0` |
| `Feeder1/Voltage` (device sub-component) | `Voltage` | `Feeder1` |

**Producer rules (goMqttDnp3):** Modbus/DNP3 mappings take `uns/code` from the
config `signalCode` (default = `metricName`) and `uns/instance` from `instance`
(default `default`). System metrics derive them natively from the path
(`Category[/Instance]/Attribute`). `bdSeq` and other session metrics carry no
`uns/*`.

**Consumer rules (goGateway):** seed `alias → {uns/code, uns/instance}` from the
BIRTH and resolve data by alias. When `uns/*` is absent (legacy/3rd-party
producer) fall back to parsing the name. The match identity is
`(entity, uns/code, uns/instance)`.

> **Producer note (Modbus):** Modbus reads have no rich quality model — a
> successful read sets `quality=192` / `dnp3.online=true`; the other `dnp3.*`
> flags are present but `false`.
>
> **Consumer note:** reading only `string_value` drops `quality`/`dnp3.*`.
> goGateway's `parsePropertyValueString` reads all typed value fields and
> stringifies them (`"192"`, `"true"`).

For point-quality independent of the properties, the **metric** carries
`is_null` (set when the source value is BAD/unusable) and `is_historical`.

---

## 6. Decoded JSON examples

The files in [`schema/`](schema/) show one of each message type, decoded to JSON
(`datatype` is the numeric code from §3; `value` is the decoded metric value):

| File | Message | Covers |
|---|---|---|
| [`schema/nbirth.json`](schema/nbirth.json) | NBIRTH | `bdSeq` + node DNP3 + node Modbus + System metrics, with aliases. |
| [`schema/ndata.json`](schema/ndata.json) | NDATA | DNP3 analog + DNP3 binary + System, **alias-only**, with typed properties. |
| [`schema/dbirth.json`](schema/dbirth.json) | DBIRTH | A child device's Modbus metrics, with aliases. |
| [`schema/ddata.json`](schema/ddata.json) | DDATA | Device Modbus values, alias-only, with properties. |
| [`schema/ndeath.json`](schema/ndeath.json) | NDEATH | `bdSeq` only. |

---

## 7. Consumer checklist (goGateway)

1. On `NBIRTH`/`DBIRTH`: rebuild the alias→name map and record `bdSeq`/`seq`.
2. On `NDATA`/`DDATA`: resolve each metric by `name` or, if absent, by `alias`.
3. Read the value from the typed oneof; honor `is_null` (→ invalid quality).
4. Read `PropertySet` typed values for `quality`/`engUnit`/`dnp3.*`.
5. On a `seq` gap or unknown alias: publish `NCMD Node Control/Rebirth=true`.
6. On `NDEATH` with matching `bdSeq`: mark the node and its devices offline.

---

## 8. Versioning

This contract tracks Eclipse Sparkplug **2.2**, payload schema **spBv1.0**.
Changes to metric naming, datatypes, or property keys are **breaking** for the
consumer and must bump this section and be coordinated across both repos.
