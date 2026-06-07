# End-to-End Test & Demo Guide — Edge → Gateway → TimescaleDB

A reproducible runbook for the full pipeline used in the validation tests:
a **goMqttModbus** edge node (Modbus + System telemetry, published as Sparkplug B)
→ MQTT broker → **goGateway** consumer (autodiscovery, catalog gating, TimescaleDB)
→ the goGateway **UI**.

It covers building both apps, standing up the infra, configuring the edge and the
gateway, running the four-iteration test (baseline → approve station → add metric →
second edge), checking each step in the UI, and enabling TLS (including on the
ICR-3232 edge).

> Related: [`sparkplug-contract.md`](sparkplug-contract.md) (wire format +
> §5.1 UNS `uns/*` decomposition), [`deploy-c1-cutover.md`](deploy-c1-cutover.md)
> (C1 migration runbook), [`ARCHITECTURE_goGateway.md`](ARCHITECTURE_goGateway.md),
> and goMqttModbus `ARCHITECTURE.md`.
>
> Current as of the UNS/C1 refactor: producers declare `uns/code` + `uns/instance`;
> the catalog matches on the composite `(entity, codigo_senal, nombre_instancia)`;
> approvals pre-fill from `uns/*` and fail loud (HTTP 207) on bad signals.

---

## 0. What this demonstrates

```
 ┌────────────────────────┐        spBv1.0/<group>/<type>/<node>[/<device>]
 │  EDGE  (goMqttModbus)  │   NBIRTH/NDATA/DBIRTH/DDATA/NDEATH/NCMD (Sparkplug B, protobuf)
 │  Modbus + System  ─────┼──────────────► ┌───────────┐
 │  node "edge-1"         │                │   MQTT    │
 └────────────────────────┘                │  broker   │
 ┌────────────────────────┐                │(mosquitto)│
 │  EDGE  "edge-2"  ───────┼──────────────► └─────┬─────┘
 └────────────────────────┘                      │ subscribe spBv1.0/<group>/#
                                                  ▼
                                   ┌──────────────────────────────┐
                                   │   GATEWAY  (goGateway)        │
                                   │  • per-node Sparkplug session │
                                   │  • autodiscovery (SQLite)     │
                                   │  • catalog gating (SSFV)      │──► TimescaleDB
                                   │  • TSDB write pipeline        │    ssfv.tbl_valores
                                   │  • REST API + embedded UI     │
                                   └──────────────────────────────┘
```

**The core rule under test:** a metric is written to TimescaleDB **only** if it is
registered as a catalog signal on a *station* (an SSFV `equipo`). Unknown
devices/metrics are recorded for operator review (autodiscovery) or dropped to the
**Descartados** ("discarded") list — never silently persisted. This applies to
process signals (Modbus/DNP3) **and** System/host metrics alike.

---

## 1. Components, prerequisites, ports

| Component | What | How |
|---|---|---|
| MQTT broker | mosquitto | Docker |
| TimescaleDB | gateway TSDB sink | Docker |
| goGateway | consumer + UI | `make build` → one binary |
| goMqttModbus | edge producer | `go build` (or static ARMv7 for ICR) |
| Modbus slave sim | fake PLC registers | `scripts/sim/modbusslave` |

Prereqs: **Go 1.24+**, **Docker**, **sqlite3**, **pnpm** (for the UI build),
and `mosquitto_sub`/`mosquitto_pub` (the guide runs them from the mosquitto
Docker image, so a local install is optional).

**Ports used in this guide** (change freely):

| Port | Service |
|---|---|
| 1883 | MQTT broker |
| 8883 | MQTT broker TLS listener (§8) |
| 1502 | Modbus slave sim |
| 8091 | goGateway HTTP/UI |
| 8090 | edge-1 (goMqttModbus) control API |
| 8092 | edge-2 control API |
| 5434 | TimescaleDB (host) |

A scratch working dir is assumed for binaries/configs:

```bash
mkdir -p /tmp/e2e/data && cd /tmp/e2e
```

---

## 2. Infrastructure: broker + TimescaleDB

```bash
# MQTT broker (anonymous, for the demo)
docker run -d --rm --name e2e-mosq -p 1883:1883 eclipse-mosquitto:2 sh -c \
  "printf 'listener 1883 0.0.0.0\nallow_anonymous true\n' > /mosquitto/config/mosquitto.conf && \
   exec mosquitto -c /mosquitto/config/mosquitto.conf"

# TimescaleDB (throwaway). goGateway applies the ssfv schema migrations on first connect.
docker run -d --rm --name e2e-tsdb -p 5434:5432 \
  -e POSTGRES_PASSWORD=pass -e POSTGRES_DB=gwtest timescale/timescaledb:latest-pg16

# wait until ready
until docker exec e2e-tsdb pg_isready -U postgres >/dev/null 2>&1; do sleep 1; done
```

> If port 1883 is already taken by an existing broker, reuse it and skip the
> mosquitto container.

---

## 3. Build the binaries

### Gateway (goGateway) — UI embedded

```bash
cd /path/to/goGateway
make build          # pnpm frontend build → embed → go build  (binary: ./goGateway at repo root)
cp goGateway /tmp/e2e/gogw
```

For a backend-only iteration loop you can `cd backend && go build -o /tmp/e2e/gogw ./cmd/gateway`,
but the embedded UI is only refreshed by `make build` (or `make frontend embed`).

### Edge (goMqttModbus)

```bash
cd /path/to/goMqttModbus

# Plain host build (Modbus + System; DNP3 master runs in stub mode)
go build -o /tmp/e2e/gateway .

# With the real DNP3 master (cgo + vendored opendnp3):
#   make build-ffi   →   binary honours -tags dnp3_ffi

# ICR-3232 edge target (fully static ARMv7, pure-Go TLS works here):
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -tags netgo -o /tmp/e2e/gateway-icr .

# Modbus slave simulator
go build -o /tmp/e2e/modbusslave ./scripts/sim/modbusslave
```

---

## 4. Start & configure the GATEWAY

Boot once to create the SQLite config DB, then set MQTT + TSDB config. (The
columns below — `qos`, `tls_*` — exist on a fresh DB; `qos` defaults to **1**.)

```bash
cd /tmp/e2e
rm -f gw.db data/*.bolt

# 1) create schema
GW_DB=/tmp/e2e/gw.db GW_HTTP=:8091 ./gogw & GW=$!; sleep 2; kill $GW; sleep 1

# 2) MQTT: enable Sparkplug B, group "plant-floor", subscribe QoS 1
sqlite3 gw.db "UPDATE mqtt_config SET sparkplug_enabled=1, sp_group_id='plant-floor',
                                      host='localhost', port=1883, qos=1 WHERE id=1;"

# 3) TSDB: write to the throwaway TimescaleDB (backend MUST be 'timescaledb')
sqlite3 gw.db "UPDATE tsdb_config SET backend='timescaledb',
   ts_dsn='postgres://postgres:pass@localhost:5434/gwtest', enabled=1,
   wal_path='/tmp/e2e/data/wal.bolt', dlq_path='/tmp/e2e/data/dlq.bolt', flush_ms=200 WHERE id=1;"

# 4) start for real
GW_DB=/tmp/e2e/gw.db GW_HTTP=:8091 ./gogw > /tmp/e2e/gogw.log 2>&1 &
sleep 6
grep -E "applied migration 0000|SSFV adapter connected|mapping cache loaded|sparkplug connected" gogw.log
```

Expected in the log:

```
tsdb: applied migration 000001_initial.up.sql … 000012_drop_host_metrics.up.sql
tsdb: SSFV adapter connected → ssfv.tbl_valores (ON CONFLICT DO NOTHING)
ssfv: mapping cache loaded (… signal entries, … equipos known)
mqtt sparkplug connected, subscribing spBv1.0/plant-floor/#
```

> The seeded catalog already contains demo equipment types (Inversor, Medidor, …)
> and a planta (`EPM Sede 30`, `planta_id=1`). We will register new stations against
> `tipo_id=2` (Medidor) and `planta_id=1`.

You can do steps 2–3 from the **UI** instead (see §7), but SQL is fastest for a
scripted run.

---

## 5. Start & configure the EDGE (edge-1)

Create the edge config. It declares one Modbus device (`meter-01`), one
**node-scoped** process metric (`tank_level`), and System telemetry. Note the
`deviceId` field: present → a Sparkplug **device** (DBIRTH/DDATA); empty → a
**node** metric (NDATA).

```bash
cat > /tmp/e2e/edge1-base.json <<'JSON'
{
  "mqtt": { "broker": "tcp://localhost:1883", "clientId": "edge-1", "qos": 1, "keepalive": 60,
            "tls": { "enabled": false } },
  "sparkplug": { "groupId": "plant-floor", "nodeId": "edge-1", "birthOnConfigChange": true },
  "outstations": [],
  "modbusDevices": [
    { "id": "plc-sim", "label": "Sim PLC", "host": "127.0.0.1", "port": 1502, "unitId": 1,
      "scanRateMs": 2000, "timeoutMs": 2000, "retries": 1, "retryDelayMs": 500, "enabled": true }
  ],
  "mappings": [
    { "id": "n1", "metricName": "tank_level", "protocol": "modbus", "sourceId": "plc-sim",
      "function": "holding_register", "address": 0, "dataType": "float32", "byteOrder": "ABCD",
      "scale": 1, "engineeringUnit": "m", "deadband": 0.05, "enabled": true },
    { "id": "d1", "metricName": "Energy_kWh", "deviceId": "meter-01", "protocol": "modbus",
      "sourceId": "plc-sim", "function": "holding_register", "address": 0, "dataType": "float32",
      "byteOrder": "ABCD", "scale": 1, "engineeringUnit": "kWh", "deadband": 0.05, "enabled": true },
    { "id": "d2", "metricName": "Relay1", "deviceId": "meter-01", "protocol": "modbus",
      "sourceId": "plc-sim", "function": "coil", "address": 1, "deadband": 0, "enabled": true }
  ],
  "system": { "enabled": true, "intervalMs": 5000, "metricPrefix": "System/", "mounts": ["/"],
    "interfaces": ["docker0"],
    "metrics": { "cpu": true, "load": false, "memory": true, "swap": false, "disk": true,
                 "network": true, "networkRates": true, "temperature": false, "uptime": true,
                 "processes": false } }
}
JSON

# start the Modbus sim, then the edge, then tell it to connect+publish
./modbusslave 1502 > /tmp/e2e/sim.log 2>&1 &
sleep 1
./gateway -port 8090 -config /tmp/e2e/edge1-base.json -log info > /tmp/e2e/edge1.log 2>&1 &
sleep 3
curl -s -X POST http://localhost:8090/api/gateway/start | python3 -m json.tool
```

`{"success":true,"data":{"running":true,"mqttConnected":true,...}}` means the edge
is connected and publishing.

---

## 6. The four-iteration test

A small decode helper (uses the gateway's diagnostic decode API to turn a raw
Sparkplug frame into JSON):

```bash
# decode the LAST frame seen on a topic substring, via POST /api/sparkplug/decode
decode() {  # usage: decode <topic-substring>
  docker run -d --rm --name e2e-cap --network host eclipse-mosquitto:2 \
    mosquitto_sub -h localhost -t 'spBv1.0/#' -F '%t %x' >/dev/null 2>&1
  sleep 6; docker logs e2e-cap > /tmp/e2e/cap.log 2>/dev/null; docker rm -f e2e-cap >/dev/null
  line=$(grep "$1" /tmp/e2e/cap.log | tail -1); topic=${line%% *}; hex=${line#* }
  curl -s -X POST http://localhost:8091/api/sparkplug/decode -H 'Content-Type: application/json' \
    -d "{\"topic\":\"$topic\",\"payload_hex\":\"$hex\"}" | python3 -m json.tool
}
TS(){ docker exec e2e-tsdb psql -U postgres -d gwtest -tc "$1"; }   # query TimescaleDB
```

### Iteration 1 — baseline, no station

The edge is publishing, but nothing is registered yet.

```bash
decode "NBIRTH/edge-1"            # names + aliases + datatypes (bdSeq, tank_level, System/*)
decode "DBIRTH/edge-1/meter-01"   # device metrics: Energy_kWh, Relay1
decode "NDATA/edge-1"             # alias-only values (System batched in one message)
decode "DDATA/edge-1/meter-01"    # alias-only + quality properties

# autodiscovery: both the node and the device are recorded as "pending"
sqlite3 -header -column gw.db "SELECT node_id,
  COALESCE(NULLIF(device_id,''),'(node)') dev, status FROM autodiscovered_entities ORDER BY device_id;"

# nothing written to TimescaleDB yet
TS "SELECT count(*) AS tbl_valores FROM ssfv.tbl_valores;"

# protocol health (with the seq fix, no rebirth storm)
echo "rebirth=$(grep -c 'requesting rebirth' gogw.log)  out-of-seq=$(grep -c 'out-of-sequence' gogw.log)"
```

**Expected:** node + `meter-01` = `pending`; `tbl_valores = 0`; `rebirth ≈ 0`.

### Iteration 2 — approve the station (data starts flowing)

"Create a station" = approve an autodiscovered entity and register its signals.
Do it from the UI (§7, **Pendientes** tab) or via the API:

```bash
MID=$(sqlite3 gw.db "SELECT id FROM autodiscovered_entities WHERE device_id='meter-01';")
NID=$(sqlite3 gw.db "SELECT id FROM autodiscovered_entities WHERE device_id='' AND node_id='edge-1';")

# device station meter-01: register Energy_kWh + Relay1
curl -s -X POST "http://localhost:8091/api/ssfv/autodiscovered/$MID/approve" \
  -H 'Content-Type: application/json' -d '{
   "planta_id":1,"tipo_id":2,"nombre_equipo":"meter-01","nombre_topic":"plant-floor/edge-1/meter-01",
   "create_signals":[
     {"codigo_senal":"Energy_kWh","nombre":"Energia","tipavar_id":6,"unidad_id":3,"tipo_valor":"Acumulado"},
     {"codigo_senal":"Relay1","nombre":"Rele","tipavar_id":10,"unidad_id":3,"tipo_valor":"Instantaneo"}]}'

# host station = the edge node itself. UNS/FIWARE split (sparkplug-contract.md
# §5.1): codigo_senal = the Attribute (uns/code), nombre_instancia = the channel
# (uns/instance). A flat metric omits nombre_instancia (→ "default"); a
# channelized one (per-interface, per-mount) sets it. From the UI the approve
# dialog PRE-FILLS both from the producer's uns/* — you just confirm.
curl -s -w ' [HTTP %{http_code}]' -X POST "http://localhost:8091/api/ssfv/autodiscovered/$NID/approve" \
  -H 'Content-Type: application/json' -d '{
   "planta_id":1,"tipo_id":2,"nombre_equipo":"edge-1-host","nombre_topic":"plant-floor/edge-1",
   "create_signals":[
     {"codigo_senal":"CPU/Usage_pct","nombre":"CPU","tipavar_id":5,"unidad_id":3},
     {"codigo_senal":"Network/Rx_MB","nombre":"NetRx","tipavar_id":5,"unidad_id":3,"nombre_instancia":"docker0"}]}'

sleep 6
TS "SELECT e.nombre_topic AS station, s.codigo_senal AS attribute, sxe.nombre_instancia AS channel,
           count(*) rows, round(max(v.valor)::numeric,2) maxv
    FROM ssfv.tbl_valores v
    JOIN ssfv.tbl_senales_x_equipo sxe ON sxe.equisenal_id=v.equisenal_id
    JOIN ssfv.tbl_senales s ON s.senal_id=sxe.senal_id
    JOIN ssfv.tbl_equipo e  ON e.equipo_id=sxe.equipo_id
    GROUP BY 1,2,3 ORDER BY 1,2;"
```

**Expected:** only the registered signals appear in `tbl_valores`. The flat
`CPU/Usage_pct` binds with `nombre_instancia='default'`; the **channelized**
`Network/Rx_MB` binds with `nombre_instancia='docker0'` — and it persists (this
was impossible before C1). The unregistered node metric `tank_level` is **not**
present.

> `tipavar_id` / `unidad_id` are FKs into the seeded `ssfv.tbl_tipo_variable` /
> `ssfv.tbl_unidades`. List them with
> `TS "SELECT tipovar_id,nombre FROM ssfv.tbl_tipo_variable;"` and
> `TS "SELECT unidad_id,nombre FROM ssfv.tbl_unidades;"`.
>
> **Fail-loud approval:** a signal that violates a constraint (e.g. a
> `codigo_senal` over 20 chars) is **not** silently dropped — the response is
> **HTTP 207 Multi-Status** with `rejected:[{codigo_senal, reason}]` so you see
> exactly what didn't register.

### Iteration 3 — the device adds a new metric

Add `Power_kW` to `meter-01` and restart the edge:

```bash
python3 - <<'PY'
import json
c=json.load(open('/tmp/e2e/edge1-base.json'))
c['mappings'].append({"id":"d3","metricName":"Power_kW","deviceId":"meter-01","protocol":"modbus",
  "sourceId":"plc-sim","function":"holding_register","address":2,"dataType":"uint16","scale":1,
  "engineeringUnit":"kW","deadband":0,"enabled":True})
json.dump(c,open('/tmp/e2e/edge1-plus.json','w'),indent=2)
PY
kill %3 2>/dev/null; fuser -k 8090/tcp 2>/dev/null; sleep 1
./gateway -port 8090 -config /tmp/e2e/edge1-plus.json -log info > /tmp/e2e/edge1.log 2>&1 &
sleep 3; curl -s -X POST http://localhost:8090/api/gateway/start >/dev/null
sleep 6

# the new metric appears in Descartados (operator review) but is NOT written
curl -s http://localhost:8091/api/ssfv/missed | python3 -m json.tool   # shows .../meter-01/Power_kW
TS "SELECT count(*) AS power_rows FROM ssfv.tbl_valores v
    JOIN ssfv.tbl_senales_x_equipo se ON se.equisenal_id=v.equisenal_id
    JOIN ssfv.tbl_senales s ON s.senal_id=se.senal_id WHERE s.codigo_senal='Power_kW';"   # → 0
```

**Expected:** `Power_kW` listed under Descartados; `power_rows = 0`; `Energy_kWh`
and `Relay1` keep writing. (Approve `Power_kW` the same way as Iteration 2 to start
persisting it.)

### Iteration 4 — a second edge device

```bash
# edge-2: own node id, client id, and device (meter-02); shares the Modbus sim
sed -e 's/"edge-1"/"edge-2"/g' -e 's/meter-01/meter-02/g' \
    /tmp/e2e/edge1-base.json > /tmp/e2e/edge2-base.json

./gateway -port 8092 -config /tmp/e2e/edge2-base.json -log info > /tmp/e2e/edge2.log 2>&1 &
sleep 3; curl -s -X POST http://localhost:8092/api/gateway/start >/dev/null
sleep 5

# both nodes are tracked independently; edge-2 shows up pending
sqlite3 -header -column gw.db "SELECT node_id,
  COALESCE(NULLIF(device_id,''),'(node)') dev, status FROM autodiscovered_entities ORDER BY node_id,device_id;"

# approve edge-2 just like edge-1 (M2/N2 ids), then confirm both write:
TS "SELECT e.nombre_topic, count(*) rows FROM ssfv.tbl_valores v
    JOIN ssfv.tbl_senales_x_equipo se ON se.equisenal_id=v.equisenal_id
    JOIN ssfv.tbl_equipo e ON e.equipo_id=se.equipo_id GROUP BY 1 ORDER BY 1;"

# steady-state protocol health with two generators
echo "rebirth=$(grep -c 'requesting rebirth' gogw.log)  out-of-seq=$(grep -c 'out-of-sequence' gogw.log)"
```

**Expected:** edge-1 and edge-2 sessions are independent (no cross-talk); after
approval both stations write to `tbl_valores`; steady-state rebirths stay ~0
(brief join resyncs aside).

> **Tip — tipo templates scale to fleets:** a signal registered on a `tipo_equipo`
> propagates to *every* equipo of that type. Approving `edge-2-host` against
> `tipo_id=2` after edge-1 auto-instantiates the System signals you already linked.

---

## 7. Check the UI

Open the gateway UI and log in:

- **URL:** `http://localhost:8091/`  (embedded SPA; served by the same binary)
- **Login:** `admin` / `admin` (printed in `gogw.log` on first boot — change it)

Dev mode (hot reload) instead of the embedded build:

```bash
GW_DB=/tmp/e2e/gw.db GW_HTTP=:8081 ./gogw &      # backend on :8081
pnpm --dir frontend dev                          # Vite proxies /api → :8081
# open the Vite URL it prints (e.g. http://localhost:5173)
```

What to look at, mapped to the test:

| UI location | Shows / does | Maps to |
|---|---|---|
| **MQTT config** (Devices/Topics view) | broker host/port, Sparkplug group, **QoS** | §4 step 2. *(The new `tls_*` + `qos` fields are persisted via the API/DB; the form may not expose all of them yet — see note below.)* |
| **TSDB** view | backend = TimescaleDB, DSN, pipeline status: writeRate, DLQ depth, circuit, WAL | §4 step 3 + health |
| **Broker Monitor** view | live stream of decoded Sparkplug frames (NBIRTH/NDATA/DBIRTH/DDATA), per-topic | Iteration 1 (watch frames arrive) |
| **SSFV → Pendientes** tab | autodiscovered nodes/devices in `pending`; **Approve** dialog (pick planta + tipo, create/link signals) | **This is the "create station" UI** — Iterations 1, 2, 4 |
| **SSFV → Catálogo / Plantas / Tipos** tabs | registered señales, equipos, plantas, tipo templates + units | the catalog you build by approving |
| **SSFV → Estado** tab | live status **and the Descartados list** (known equipment, unregistered signal) | Iteration 3 (`Power_kW` shows here) |

Walkthrough:

1. **Iteration 1** — open **Broker Monitor**: frames from `edge-1` stream in.
   Open **SSFV → Pendientes**: `edge-1` (node) and `meter-01` (device) are listed
   `pending`. **TSDB** view shows the pipeline healthy but no plant writes yet.
2. **Iteration 2** — in **Pendientes**, click **Approve** on `meter-01`: choose
   *Planta* = EPM Sede 30, *Tipo* = Medidor. Each signal's **`codigo_senal` and
   `nombre_instancia` are pre-filled** from the producer's `uns/*` properties
   (sparkplug-contract.md §5.1) — you confirm rather than type. Repeat for the
   node (`edge-1`): channelized System metrics show their channel, e.g.
   `Network/Rx_MB` *· instancia `docker0` (se vincula a este equipo)*. The entity
   moves to `approved`; **Catálogo** lists the señales and values begin to persist.
3. **Iteration 3** — after adding `Power_kW`, open **SSFV → Estado**: the
   Descartados list shows `plant-floor/edge-1/meter-01/Power_kW` (received but not
   registered → not stored). Approve it to start persisting.
4. **Iteration 4** — **Pendientes** now also lists `edge-2`; approve it the same
   way. Both nodes appear in **Catálogo/Estado** independently.

> **Note on the new TLS/QoS settings:** the backend persists `mqtt_config.qos`
> and `mqtt_config.tls_ca_file/tls_cert_file/tls_key_file/tls_insecure` and the
> REST API round-trips them, but the MQTT-config **form may not yet render input
> fields** for every one. Until it does, set them via the API or SQL (see §8). This
> is a known follow-up.

---

## 8. Enable TLS (gateway + edge)

`crypto/tls` is pure Go in both apps, so TLS needs no native dependency and works
in the static ICR-3232 edge build.

### 8.1 Broker TLS + a demo CA

```bash
cd /tmp/e2e && mkdir -p tls && cd tls
# a throwaway CA + a broker server cert (CN/SAN = the host clients dial)
openssl req -x509 -newkey rsa:2048 -nodes -days 825 -keyout ca.key -out ca.crt -subj "/CN=e2e-ca"
openssl req -newkey rsa:2048 -nodes -keyout broker.key -out broker.csr \
  -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
openssl x509 -req -in broker.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -days 825 -out broker.crt -copy_extensions copyext
cd /tmp/e2e

docker rm -f e2e-mosq 2>/dev/null
docker run -d --rm --name e2e-mosq -p 8883:8883 -v /tmp/e2e/tls:/tls eclipse-mosquitto:2 sh -c \
  "printf 'listener 8883 0.0.0.0\nallow_anonymous true\ncafile /tls/ca.crt\ncertfile /tls/broker.crt\nkeyfile /tls/broker.key\n' \
     > /mosquitto/config/mosquitto.conf && exec mosquitto -c /mosquitto/config/mosquitto.conf"
```

### 8.2 Gateway over TLS

```bash
# point the consumer at the TLS listener and give it the CA
sqlite3 gw.db "UPDATE mqtt_config SET use_tls=1, port=8883, qos=1,
                                      tls_ca_file='/tmp/e2e/tls/ca.crt' WHERE id=1;"
# (mutual TLS: also set tls_cert_file/tls_key_file; dev shortcut: tls_insecure=1)
# restart gogw — it builds a *tls.Config and dials ssl://localhost:8883
```

Or via the REST API:

```bash
curl -s -X PUT http://localhost:8091/api/mqtt-config -H 'Content-Type: application/json' -d '{
  "host":"localhost","port":8883,"client_id":"goGateway","use_tls":true,"qos":1,
  "sparkplug_enabled":true,"sp_group_id":"plant-floor","sp_host_id":"goGateway-host","sp_topics":"",
  "tls_ca_file":"/tmp/e2e/tls/ca.crt","tls_cert_file":"","tls_key_file":"","tls_insecure":false }'
```

### 8.3 Edge over TLS

Set the `tls` block and point the broker at the TLS listener. The scheme is
coerced to `ssl://` automatically when `tls.enabled` is true, but the **port must
be the TLS port**:

```jsonc
"mqtt": {
  "broker": "ssl://localhost:8883",          // or tcp://… — scheme is upgraded when enabled
  "clientId": "edge-1", "qos": 1, "keepalive": 60,
  "tls": { "enabled": true, "caFile": "/tmp/e2e/tls/ca.crt",
           "certFile": "", "keyFile": "", "insecure": false }
}
```

On the **ICR-3232**, ship `ca.crt` (and client cert/key for mutual TLS) to the
device and reference their on-device paths. The static `netgo` build from §3 has
full TLS support; no OpenSSL is required.

---

## 9. Teardown

```bash
fuser -k 8090/tcp 8091/tcp 8092/tcp 1502/tcp 2>/dev/null
pkill -f /tmp/e2e/gateway; pkill -f /tmp/e2e/gogw; pkill -f /tmp/e2e/modbusslave
docker rm -f e2e-mosq e2e-tsdb e2e-cap 2>/dev/null
```

---

## Appendix — cheat sheets

**REST API (gateway, port 8091):**

| Method · path | Purpose |
|---|---|
| `POST /api/sparkplug/decode` | decode a raw frame (`{topic, payload_hex}`) → JSON |
| `GET  /api/ssfv/autodiscovered` | pending/approved nodes & devices |
| `POST /api/ssfv/autodiscovered/{id}/approve` | create a station + register signals |
| `GET  /api/ssfv/missed` | Descartados (known equipment, unregistered signal) |
| `GET  /api/tsdb/status` | pipeline health (writeRate, dlqDepth, circuit, WAL) |
| `GET  /api/status` | broker/IEC-104/counts |
| `GET/PUT /api/mqtt-config` | broker, Sparkplug, **qos**, **tls_*** |

**Edge control API (per producer, ports 8090/8092):**

| Method · path | Purpose |
|---|---|
| `POST /api/gateway/start` | connect MQTT + start publishing |
| `POST /api/gateway/stop` | stop (publishes NDEATH) |
| `GET  /api/system/telemetry` | host telemetry even when the broker is down |

**TimescaleDB (psql) quick checks:**

```sql
-- rows per registered signal
SELECT s.codigo_senal, count(*) FROM ssfv.tbl_valores v
  JOIN ssfv.tbl_senales_x_equipo se ON se.equisenal_id=v.equisenal_id
  JOIN ssfv.tbl_senales s ON s.senal_id=se.senal_id GROUP BY 1 ORDER BY 2 DESC;
-- stations (equipos) and their registered signals
SELECT e.nombre_topic, s.codigo_senal FROM ssfv.tbl_senales_x_equipo se
  JOIN ssfv.tbl_equipo e USING(equipo_id) JOIN ssfv.tbl_senales s USING(senal_id)
  ORDER BY 1,2;
```

**Sequence health (the rebirth-storm indicator):**

```bash
grep -c 'requesting rebirth' gogw.log   # steady-state should stay ~0
grep -c 'out-of-sequence'   gogw.log    # spikes only on producer restart / mid-stream join
```
