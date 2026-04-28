# Sparkplug B mode — operator guide and message examples

goGateway acts as a **Sparkplug B Primary Application (SCADA Host)**.  It does
not publish sensor data of its own; it *consumes* data from Edge-of-Network
(EoN) nodes and forwards decoded metric values to one or more IEC 60870-5-104
slave endpoints.

---

## Topic namespace anatomy

```
spBv1.0 / {group_id} / {msg_type} / {edge_node_id} [/ {device_id}]
   │           │            │              │                │
   │           │            │              │                └─ present only for D* messages
   │           │            │              └─ identifier for the EoN node (e.g. "INV-001")
   │           │            └─ NBIRTH | NDEATH | NDATA | DBIRTH | DDEATH | DDATA | NCMD
   │           └─ logical grouping configured in MQTT Config → Group ID (e.g. "plant-floor")
   └─ always literal "spBv1.0"
```

The gateway subscribes to the single wildcard `spBv1.0/{group_id}/#` and routes
every message by its parsed type.

**STATE** uses a different pattern — no namespace prefix:
```
STATE/{host_id}    payload = "ONLINE" or "OFFLINE"  (plain UTF-8, QoS 1, retained)
```
The gateway publishes `STATE/{host_id} = "ONLINE"` when it connects and sets a
Last-Will to `STATE/{host_id} = "OFFLINE"` so the broker delivers it on any
unclean disconnect.

---

## Protobuf payload — JSON representation

The wire format is Google Protocol Buffers (binary).  The JSON below shows what
each message *means*; it is **not** what arrives over the wire.

### Datatypes reference

| Sparkplug datatype | Code | Go type extracted |
|--------------------|------|-------------------|
| Int8 / Int16 / Int32 | 1–3 | `int_value` (uint32 wire) |
| Int64 | 4 | `long_value` (uint64 wire) |
| UInt8 / UInt16 / UInt32 | 5–7 | `int_value` (uint32 wire) |
| UInt64 | 8 | `long_value` (uint64 wire) |
| Float | 9 | `float_value` |
| Double | 10 | `double_value` |
| Boolean | 11 | `bool_value` |
| String / Text | 12 / 14 | `string_value` (not forwarded to IEC-104) |

---

### NBIRTH — EoN node comes online

**Topic:** `spBv1.0/plant-floor/NBIRTH/INV-001`

```json
{
  "timestamp": 1706000000000,
  "seq": 0,
  "metrics": [
    { "name": "bdSeq",                  "datatype": 8,  "long_value": 3          },
    { "name": "Node Control/Rebirth",   "datatype": 11, "bool_value": false       },
    { "name": "outputs/power",          "alias": 1, "datatype": 9, "float_value": 450.7  },
    { "name": "outputs/voltage",        "alias": 2, "datatype": 9, "float_value": 233.1  },
    { "name": "outputs/current",        "alias": 3, "datatype": 9, "float_value": 1.94   },
    { "name": "status/fault",           "alias": 4, "datatype": 11,"bool_value":  false  },
    { "name": "status/grid_connected",  "alias": 5, "datatype": 11,"bool_value":  true   }
  ]
}
```

**What the gateway does:**
1. Validates `seq == 0` (spec requirement for every NBIRTH).
2. Builds the **alias map**: `1 → "outputs/power"`, `2 → "outputs/voltage"`, etc.
   Subsequent NDATA packets may omit the name and send only the alias number.
3. For each metric that has a `metric_name` match in `signal_mappings`, pushes
   the decoded value to the corresponding IEC-104 IOA with `QualityGood`.
4. Marks the node **online** in the session registry.

`bdSeq` is a birth/death sequence counter.  The gateway records it and checks
that the NDEATH LWT contains the same value (spec §16.8).

---

### NDATA — EoN node reports changed values

**Topic:** `spBv1.0/plant-floor/NDATA/INV-001`

```json
{
  "timestamp": 1706000060000,
  "seq": 1,
  "metrics": [
    { "alias": 1, "datatype": 9, "float_value": 462.3 },
    { "alias": 2, "datatype": 9, "float_value": 234.0 }
  ]
}
```

**What the gateway does:**
1. Validates `seq == (prev_seq + 1) % 256`.  If wrong, publishes
   `spBv1.0/plant-floor/NCMD/INV-001` with a **Rebirth** command and waits
   for a new NBIRTH.
2. Resolves alias → metric name via the map seeded at NBIRTH.
3. Dispatches only the metrics present in this packet (report-by-exception).
4. `outputs/current` and `status/*` are absent — those IOAs keep their last
   values and quality unchanged.

---

### NDEATH — EoN node goes offline

**Topic:** `spBv1.0/plant-floor/NDEATH/INV-001`  
*(usually delivered as the MQTT broker's Last-Will on unclean disconnect)*

```json
{
  "timestamp": 1706000120000,
  "metrics": [
    { "name": "bdSeq", "datatype": 8, "long_value": 3 }
  ]
}
```

**What the gateway does:**
1. Marks the node **offline**.
2. Dispatches `QualityNotTopical` (`0x04`) with `value = 0` to **every** IOA
   that was mapped to this node — SCADA immediately sees the stale flag.

---

### DBIRTH — device under a node comes online

**Topic:** `spBv1.0/plant-floor/DBIRTH/INV-001/MPPT-A`

```json
{
  "timestamp": 1706000001000,
  "seq": 1,
  "metrics": [
    { "name": "temperature",     "alias": 10, "datatype": 9,  "float_value": 25.3    },
    { "name": "pressure",        "alias": 11, "datatype": 9,  "float_value": 1013.2  },
    { "name": "alarm/high_temp", "alias": 12, "datatype": 11, "bool_value":  false   }
  ]
}
```

Device-level messages share the node's MQTT session but have their own
sequence counter and alias map.  The gateway builds a separate per-device
alias map; metric names are resolved independently from the node's map.

---

### DDATA — device data update

**Topic:** `spBv1.0/plant-floor/DDATA/INV-001/MPPT-A`

```json
{
  "timestamp": 1706000065000,
  "seq": 2,
  "metrics": [
    { "alias": 10, "datatype": 9,  "float_value": 26.1  },
    { "alias": 12, "datatype": 11, "bool_value":  true  }
  ]
}
```

Same report-by-exception semantics as NDATA.  Out-of-sequence DDATA triggers
the same NCMD Rebirth as for node-level data.

---

### DDEATH — device goes offline

**Topic:** `spBv1.0/plant-floor/DDEATH/INV-001/MPPT-A`

All IOAs mapped to this device receive `QualityNotTopical`.

---

## Quality mapping: Sparkplug B → IEC-104 QDS

| Sparkplug B condition | IEC-104 QDS byte | Constant |
|-----------------------|-----------------|---------|
| Normal metric, node online | `0x00` | `QualityGood` |
| Metric has `is_null = true` | `0x80` | `QualityInvalid` |
| Metric has `is_historical = true` | `0x04` | `QualityNotTopical` |
| NDEATH / DDEATH received | `0x04` | `QualityNotTopical` |
| Broker disconnection | `0x04` | `QualityNotTopical` |

---

## Operator configuration — step by step

### 1. Enable Sparkplug B

**UI → MQTT Config → Sparkplug B**

| Field | Example | Meaning |
|-------|---------|---------|
| Sparkplug B | ✓ enabled | Switch all MQTT traffic to protobuf mode |
| Group ID | `plant-floor` | Subscribes to `spBv1.0/plant-floor/#` |
| Host ID | `goGateway-scada` | STATE topic becomes `STATE/goGateway-scada` |

### 2. Create a Device and Topic for the EoN node

**UI → Devices & Topics**

Create a **Device** scoped to the target IEC-104 server, then add a **Topic**
whose topic string is the node's *nodeBase* (not a full MQTT topic — no
`/NBIRTH/` etc.):

| Field | Value |
|-------|-------|
| Topic | `spBv1.0/plant-floor/INV-001` |

For a **device-level** metric (DBIRTH/DDATA), use the deviceBase instead:

| Field | Value |
|-------|-------|
| Topic | `spBv1.0/plant-floor/INV-001/MPPT-A` |

### 3. Create Signal Mappings

**UI → Signal Mapper → New mapping**

For each metric you want forwarded to IEC-104:

| Field | Value | Note |
|-------|-------|------|
| Topic | `spBv1.0/plant-floor/INV-001` | nodeBase from step 2 |
| Sparkplug metric name | `outputs/power` | exact name from NBIRTH |
| JSON key | *(leave blank)* | only used in JSON mode |
| IEC 104 type | `M_ME_TF_1` | measured float w/ timestamp |
| IOA | `16385` | address the SCADA will poll |
| Scale | `0.001` | e.g. W → kW conversion |

**Example mapping table for INV-001:**

| Metric name | IOA | IEC type | Scale | Description |
|-------------|-----|----------|-------|-------------|
| `outputs/power` | 16385 | M_ME_TF_1 | 0.001 | AC power (W → kW) |
| `outputs/voltage` | 16386 | M_ME_TF_1 | 1.0 | AC voltage (V) |
| `outputs/current` | 16387 | M_ME_TF_1 | 1.0 | AC current (A) |
| `status/fault` | 16388 | M_SP_TB_1 | 1.0 | Fault alarm (bool) |
| `status/grid_connected` | 16389 | M_SP_TB_1 | 1.0 | Grid relay (bool) |
| `temperature` | 16390 | M_ME_TF_1 | 1.0 | MPPT-A board temp |
| `alarm/high_temp` | 16391 | M_SP_TB_1 | 1.0 | MPPT-A over-temp |

The last two rows use the device topic `spBv1.0/plant-floor/INV-001/MPPT-A`.

### 4. Save and reconnect

Saving MQTT Config triggers an immediate reconnect.  The gateway will:
1. Set LWT `STATE/goGateway-scada = "OFFLINE"` before connecting.
2. On connect: publish `STATE/goGateway-scada = "ONLINE"` (retained, QoS 1).
3. Subscribe `spBv1.0/plant-floor/#` at QoS 0.
4. Wait for NBIRTH from each node; dispatch metrics as they arrive.

---

## Backward compatibility

Setting `sparkplug_enabled = 0` restores the original JSON mode completely.
All `signal_mappings` rows with a `metric_name` set are simply ignored in JSON
mode; rows with only `json_key` set are ignored in Sparkplug B mode.  Both
fields can coexist in the database without conflict.
