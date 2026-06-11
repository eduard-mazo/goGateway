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

### 1.1 Infrastructure hierarchy (group → node → device → signal)

The Sparkplug namespace **is** the infrastructure model; the consumer catalog
mirrors it 1:1. The UI ("Señales SSFV → Infraestructura") shows **aliases** for
the Sparkplug identifiers:

| UI concept | Sparkplug | Catalog | Example |
|---|---|---|---|
| **Planta** (alias) | `group_id` | `tbl_planta.nombre` = alias, `broker_base` = the group_id | `GSANRAFA` ↔ `EPM_SSFV` |
| **Equipo** (alias) | `edge_node_id` | `tbl_equipo` at depth 2: `nombre_topic = {group}/{node}` | `EDGE` ↔ `EPM_SSFV/EDGE` |
| **Device** | `device_id` | `tbl_equipo` at depth 3: `nombre_topic = {group}/{node}/{device}` | `DNP` ↔ `EPM_SSFV/EDGE/DNP` |

```
group_id        ↔  Planta            (ssfv.tbl_planta.broker_base = the group, one segment)
 └─ edge_node   ↔  Equipo (Nodo)     (the edge gateway; the segment after the group)
     ├─ node-level metrics  →  System + node process signals   (NDATA)
     └─ device_id ↔ Device   →  device signals                 (DDATA)
         (signals)           →  ssfv.tbl_senales_x_equipo (codigo_senal + nombre_instancia)
```

**Rules (binding):**
- A **Planta maps to at most one `group_id`**. `tbl_planta.broker_base` holds the
  **group** (a single topic segment, e.g. `EPM_SSFV`); `tbl_planta.nombre` is the
  free-form UI alias (e.g. `GSANRAFA`).
- An **Entity** (catalog `tbl_equipo`, `nombre_topic`) is either an **Equipo/node**
  (`{group}/{node}` — publishes NDATA; carries its System + node process signals)
  or a **Device** (`{group}/{node}/{device}` — publishes DDATA). The Device's
  parent Equipo is implicit: strip the last topic segment.
- **Prefix invariant:** `equipo.nombre_topic` MUST start with its planta's
  `broker_base` (the group). Enforced in the consumer at the API (422) and the DB
  (trigger, migration 0014).
- The **node level is implicit** in the topic: `node = first segment of
  nombre_topic after the group`. (Opción A — no separate node table.)

The producer is already conformant: `sparkplug.groupId` = Planta, `nodeId` =
Equipo, `deviceId` = Device. It SHOULD declare the Planta UI alias in the NBIRTH
payload-level properties as **`uns/planta`** (e.g. `"GSANRAFA"`) so the consumer
can stage the Planta for creation when it does not exist yet (§7).

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

Metric names use `/` as a **folder** separator (standard Sparkplug: clients
render the segments as a tree inside the payload). The folder path groups
metrics within an entity (e.g. `PLC/tank_level`, `VALV/VALV_ON`,
`SYSTEM/CPU/Usage_pct`); the leaf is the attribute. Folders are organizational —
the catalog identity is carried by `uns/*` (§5.1), never parsed from the name.

| Source | Name | Notes |
|---|---|---|
| **DNP3** | `SignalMapping.metricName` (user-defined, free-form) | e.g. `VALV/VALV_ON`, `Feeder1/Voltage`. Identity is `(pointType, index)` on the outstation. |
| **Modbus** | `SignalMapping.metricName` (user-defined) | e.g. `PLC/tank_level`, `Meter1/Energy_kWh`. Identity is `(function, address)`. |
| **System (numeric)** | `{metricPrefix}{group}/{leaf}` | Prefix is configurable: default `System/`, `SYSTEM/` in the EPM deployment — consumers MUST match it case-insensitively. e.g. `SYSTEM/CPU/Usage_pct`, `SYSTEM/Memory/Free_MB`, `System/Disk/<mount>/Free_MB`, `System/Network/<iface>/RxRate_kbps`, `System/Temperature/CPU_C`, `System/Power/Supply_V`, `System/Power/RTC_Battery_OK`, `System/Uptime_h`, `System/Process/Count`. |
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
| `uns/code` | `String` (8) | **UNS Attribute** — the canonical signal code, the metric's **leaf** (→ consumer catalog `codigo_senal`, ≤ 20 chars). Declared in NBIRTH/DBIRTH (and echoed on data). |
| `uns/instance` | `String` (8) | **UNS folder / channel path** between the entity and the leaf (→ consumer `nombre_instancia`, ≤ 30 chars). `default` when the metric is flat. |
| `uns/name` | `String` (8) | **Birth only.** Display name → `tbl_senales.nombre` (e.g. `"Valvula abierta"`). Pre-fills the operator approve dialog (§7). |
| `uns/description` | `String` (8) | **Birth only.** → `tbl_senales.descripcion` (e.g. `"Valvula gas confirmación apertura"`). |
| `uns/planta` | `String` (8) | **NBIRTH payload-level property** (not per-metric): the Planta UI alias (e.g. `"GSANRAFA"`) for staging the Planta on first contact (§1.1, §7). |

PropertyValue value-field numbers: `int_value`=3, `long_value`=4,
`float_value`=5, `double_value`=6, `boolean_value`=7, `string_value`=8.

### 5.1 UNS / FIWARE decomposition (`uns/*`) — universal

A Sparkplug metric name is a flat string; the **producer** knows its true
structure and declares it explicitly via `uns/code` + `uns/instance` so any
consumer maps it to an **Entity → Attribute** model deterministically, **without
parsing the name**. This is protocol- and domain-agnostic (Modbus, DNP3, System,
any). The **Entity** is the MQTT topic node (`…/NDATA/node`) or device
(`…/DDATA/node/device`); `uns/code` is the **leaf** attribute; `uns/instance`
is the **folder/channel path** between the entity and the leaf.

**The rule (v3):** for a metric name `[prefix/]folder₁/…/folderₙ/leaf`
(the cosmetic host-telemetry prefix stripped first, §4):

- `uns/code` = `leaf`
- `uns/instance` = `folder₁/…/folderₙ`, or `default` when there are no folders.

| Metric name (example) | `uns/code` | `uns/instance` |
|---|---|---|
| `tank_press` (flat, device) | `tank_press` | `default` |
| `PLC/tank_level` (device) | `tank_level` | `PLC` |
| `VALV/VALV_ON` (device) | `VALV_ON` | `VALV` |
| `SYSTEM/CPU/Usage_pct` (node) | `Usage_pct` | `CPU` |
| `SYSTEM/Memory/Usage_pct` (node) | `Usage_pct` | `Memory` |
| `System/Disk/root/Used_pct` (node) | `Used_pct` | `Disk/root` |
| `System/Network/eth0/Rx_MB` (node) | `Rx_MB` | `Network/eth0` |
| `Feeder1/Voltage` (device sub-component) | `Voltage` | `Feeder1` |

Note the same leaf under two folders (`Usage_pct` @ `CPU` / `Memory`) is **one**
catalog señal (`codigo_senal = Usage_pct`, one `nombre`/`descripcion`) bound to
the entity once per instancia — the folder is the instance dimension.

**Producer rules (goMqttDnp3):** Modbus/DNP3 mappings take `uns/code` from the
config `signalCode` (default = the **leaf** of `metricName`) and `uns/instance`
from `instance` (default = the folder path of `metricName`, else `default`).
System metrics derive them natively from the path. `bdSeq` and other session
metrics carry no `uns/*`.

**Consumer rules (goGateway):** seed `alias → {uns/code, uns/instance}` from the
BIRTH and resolve data by alias. When `uns/*` is absent (legacy/3rd-party
producer) fall back to parsing the name with the same leaf/folder rule. The
match identity is `(entity, uns/code, uns/instance)`.

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
7. **Auto-discovery staging:** every NBIRTH/DBIRTH from an entity not in the
   catalog is recorded as `pending` (with its metric list, `uns/*` metadata and
   node properties) for operator review — **nothing is provisioned without
   confirmation in the UI**. On approval the consumer creates, as needed: the
   **Planta** (alias from `uns/planta`, group from the topic), the
   **Equipo/Device** (`tbl_equipo`), and the **señales**
   (`codigo_senal`/`nombre`/`descripcion` pre-filled from
   `uns/code`/`uns/name`/`uns/description`) — then publishes
   `NCMD Node Control/Rebirth=true` so data flows against the fresh catalog.
   Unregistered signals on known entities are quarantined (Descartados), never
   silently dropped.

---

## 8. Versioning

This contract tracks Eclipse Sparkplug **2.2**, payload schema **spBv1.0**.
Changes to metric naming, datatypes, or property keys are **breaking** for the
consumer and must bump this section and be coordinated across both repos.

**Contract v3** (2026-06): `uns/code` is now the **leaf** attribute and
`uns/instance` the **folder path** (§5.1) — previously the code kept the
category (`CPU/Usage_pct`) and the instance was only the middle channel
segment. Catalog rows created under the v2 scheme (codes containing `/`) must
be migrated by splitting at the last `/`. Added birth-only metadata properties
`uns/name`/`uns/description` and the NBIRTH payload property `uns/planta`
(§5), and the staging lifecycle (§7).
