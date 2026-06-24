#!/usr/bin/env bash
# =============================================================================
# soak.sh — one-command reproduction of the full goMqttModbus → goGateway →
# go104 path.
#
# Brings up a throwaway stack (TimescaleDB + edge + Modbus sim + go104), lets the
# gateway autodiscover the edge, approves it (creating its planta inline —
# nothing is pre-seeded), then runs a timed soak that ingests data and deletes
# the planta mid-run to prove: discovery-driven binding, clean delete (bindings
# deactivated, ingest stops), and no re-surfacing.
#
# It ALSO exercises the IEC-104 egress end to end: after approval, the plant is
# linked to a 104 server (POST /ssfv/plantas/{id}/iec104-link), which mirrors
# meter-01's catalog signals as IEC-104 points; go104's signals are seeded from
# the link response, and a go104 master connects, runs GI, and records the values
# — proving origin (edge Modbus→Sparkplug) → middle (goGateway) → destination
# (go104 IEC-104). SSFV is the principal and IEC-104 a DEPENDENT mirror: deleting
# the plant cascade-deletes the mirrors (gw_maps→0) and go104 stops updating.
#
# Usage:
#   scripts/soak.sh run [MINUTES]   # build + bring up + soak (default 10 min)
#   scripts/soak.sh up              # build + bring up + approve, no soak loop
#   scripts/soak.sh down            # tear everything down
#
# The edge also serves a DNP3 outstation that go104 polls as a master — an
# edge-direct path alongside the gateway-mediated IEC-104 one. It survives the
# plant delete (independent of SSFV), proving the DNP3 outstation feature e2e.
#
# Env overrides (defaults in parens):
#   GOGW_DIR (repo root)   EDGE_DIR (../goMqttModbus)   WORK (/tmp/e2e-soak)
#   GO104_DIR (../go104)   GODNP3_DIR (../goDnp3)
#   TSDB_PORT (5434)  GW_HTTP (:8091)  EDGE_HTTP (8090)  MODBUS_PORT (1502)
#   GO104_HTTP (8092)  IEC_PORT (2404)  DNP3_OSTN_PORT (20100)
#   MQTT (tcp://localhost:1883)  GROUP (EPM_SOAK)  DELETE_AT (8)
#
# Requires: docker, go, sqlite3, python3, curl, an MQTT broker on :1883 (reused
# if already up — else one is started), plus g++ and the goDnp3 sibling checkout
# for the DNP3 builds (opendnp3 is vendored on first run via `make opendnp3-vendor`).
# =============================================================================
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GOGW_DIR="${GOGW_DIR:-$(cd "$HERE/.." && pwd)}"
EDGE_DIR="${EDGE_DIR:-$(cd "$GOGW_DIR/../goMqttModbus" 2>/dev/null && pwd || true)}"
GO104_DIR="${GO104_DIR:-$(cd "$GOGW_DIR/../go104" 2>/dev/null && pwd || true)}"
WORK="${WORK:-/tmp/e2e-soak}"
TSDB_PORT="${TSDB_PORT:-5434}"
GW_HTTP="${GW_HTTP:-:8091}"
EDGE_HTTP="${EDGE_HTTP:-8090}"
MODBUS_PORT="${MODBUS_PORT:-1502}"
GO104_HTTP="${GO104_HTTP:-8092}"
IEC_PORT="${IEC_PORT:-2404}"
MQTT="${MQTT:-tcp://localhost:1883}"
GROUP="${GROUP:-EPM_SOAK}"
DELETE_AT="${DELETE_AT:-8}"
# DNP3: the edge serves a DNP3 outstation that go104 polls as a master — the
# edge-direct path alongside the gateway-mediated IEC-104 one.
GODNP3_DIR="${GODNP3_DIR:-$(cd "$GOGW_DIR/../goDnp3" 2>/dev/null && pwd || true)}"
DNP3_TRIPLE="${DNP3_TRIPLE:-x86_64-unknown-linux-gnu}"
DNP3_OSTN_PORT="${DNP3_OSTN_PORT:-20100}"

API="http://localhost${GW_HTTP}/api"
DSN="postgres://postgres:pass@localhost:${TSDB_PORT}/gwtest"
TS(){ docker exec e2e-soak-tsdb psql -U postgres -d gwtest -tAc "$1" 2>/dev/null | tr -d ' '; }
say(){ printf '\033[1;36m== %s\033[0m\n' "$*"; }
die(){ printf '\033[1;31mERROR: %s\033[0m\n' "$*" >&2; exit 1; }

# ── teardown ────────────────────────────────────────────────────────────────
down(){
  say "tearing down"
  for f in "$WORK"/.gw.pid "$WORK"/.edge.pid "$WORK"/.sim.pid "$WORK"/.mosq.pid "$WORK"/.go104.pid; do
    [ -f "$f" ] && { kill "$(cat "$f")" 2>/dev/null || true; rm -f "$f"; }
  done
  docker rm -f e2e-soak-tsdb e2e-soak-mosq >/dev/null 2>&1 || true
  echo "done."
}

# ── build ───────────────────────────────────────────────────────────────────
build(){
  [ -n "$EDGE_DIR" ] && [ -d "$EDGE_DIR" ] || die "goMqttModbus not found (set EDGE_DIR)"
  [ -n "$GO104_DIR" ] && [ -d "$GO104_DIR" ] || die "go104 not found (set GO104_DIR)"
  [ -n "$GODNP3_DIR" ] && [ -d "$GODNP3_DIR" ] || die "goDnp3 not found (set GODNP3_DIR)"
  # Edge serves a DNP3 outstation and go104 polls it, so both link the real
  # opendnp3 (-tags dnp3_ffi) from the shared goDnp3 module. Vendor it if missing.
  local d3="$GODNP3_DIR/third_party/opendnp3/$DNP3_TRIPLE"
  if [ ! -f "$d3/lib/libopendnp3.a" ]; then
    say "vendoring opendnp3 (goDnp3)"
    ( cd "$GODNP3_DIR" && make opendnp3-vendor )
  fi
  local FFI=(CGO_ENABLED=1
    "CGO_CXXFLAGS=-std=c++17 -I$d3/include"
    "CGO_LDFLAGS=-L$d3/lib -lopendnp3 -lssl -lcrypto -lstdc++ -lpthread -lm -ldl")
  mkdir -p "$WORK/data"
  say "building gateway (backend)"
  ( cd "$GOGW_DIR/backend" && go build -o "$WORK/gogw" ./cmd/gateway )
  say "building edge (DNP3 outstation) + modbus sim"
  ( cd "$EDGE_DIR" && env "${FFI[@]}" go build -tags dnp3_ffi -o "$WORK/edge" . \
      && go build -o "$WORK/modbusslave" ./scripts/sim/modbusslave )
  say "building go104 (IEC-104 + DNP3 master)"
  ( cd "$GO104_DIR" && env "${FFI[@]}" go build -tags dnp3_ffi -o "$WORK/go104" ./cmd/server )
}

# ── infra: TSDB (+ broker if none on :1883) ──────────────────────────────────
infra(){
  say "starting throwaway TimescaleDB on :$TSDB_PORT"
  docker rm -f e2e-soak-tsdb >/dev/null 2>&1 || true
  docker run -d --rm --name e2e-soak-tsdb -p "${TSDB_PORT}:5432" \
    -e POSTGRES_PASSWORD=pass -e POSTGRES_DB=gwtest timescale/timescaledb:latest-pg16 >/dev/null
  # Probe the gwtest DB itself, not pg_isready: the image's entrypoint reports
  # ready on a temporary server *before* it creates gwtest and restarts.
  local ok=""
  for _ in $(seq 1 45); do
    [ "$(TS 'SELECT 1')" = "1" ] && { ok=1; break; }
    sleep 1
  done
  [ -n "$ok" ] || die "TimescaleDB did not come up"

  if ! (exec 3<>/dev/tcp/localhost/1883) 2>/dev/null; then
    say "no broker on :1883 — starting throwaway mosquitto"
    docker rm -f e2e-soak-mosq >/dev/null 2>&1 || true
    docker run -d --rm --name e2e-soak-mosq -p 1883:1883 eclipse-mosquitto:2 sh -c \
      "printf 'listener 1883 0.0.0.0\nallow_anonymous true\n' > /mosquitto/config/mosquitto.conf && exec mosquitto -c /mosquitto/config/mosquitto.conf" >/dev/null
    sleep 2
  else
    say "reusing existing broker on :1883"
  fi
}

# ── gateway ───────────────────────────────────────────────────────────────────
start_gw(){
  say "configuring + starting gateway"
  rm -f "$WORK/gw.db" "$WORK"/data/*.bolt 2>/dev/null || true
  GW_DB="$WORK/gw.db" GW_HTTP="$GW_HTTP" "$WORK/gogw" >"$WORK/gw-init.log" 2>&1 & local p=$!
  sleep 3; kill "$p" 2>/dev/null || true; sleep 1
  # Distinct client_id so the soak gateway never collides on the shared broker
  # with a stray/previous gateway (a duplicate clientID causes a reconnect storm
  # that drops Sparkplug session state and stalls dispatch).
  sqlite3 "$WORK/gw.db" "UPDATE mqtt_config SET sparkplug_enabled=1, sp_group_id='$GROUP', host='localhost', port=1883, qos=1, client_id='gw-soak' WHERE id=1;"
  sqlite3 "$WORK/gw.db" "UPDATE tsdb_config SET backend='timescaledb', ts_dsn='$DSN', enabled=1, wal_path='$WORK/data/wal.bolt', dlq_path='$WORK/data/dlq.bolt', flush_ms=200 WHERE id=1;"
  # IEC-104 egress: enable the passive slave on :$IEC_PORT (localhost allowlist).
  # Only the SERVER (config) is seeded here — the per-signal mappings are created
  # later from the SSFV catalog via the expose-iec104 bridge, not raw SQL.
  sqlite3 "$WORK/gw.db" "UPDATE iec104_servers SET enabled=1, port=$IEC_PORT, asdu_addr=1, scada_ips='127.0.0.1' WHERE id=1;"
  setsid env GW_DB="$WORK/gw.db" GW_HTTP="$GW_HTTP" "$WORK/gogw" >"$WORK/gw.log" 2>&1 </dev/null & echo $! >"$WORK/.gw.pid"
  sleep 6
  grep -aq "SSFV adapter connected" "$WORK/gw.log" || die "gateway did not connect (see $WORK/gw.log)"
  echo "   migrations applied: $(grep -ac 'applied migration' "$WORK/gw.log")"
  grep -a "mapping cache loaded" "$WORK/gw.log" | tail -1 | sed 's/^/   /'
  grep -aq "iec104.*listening" "$WORK/gw.log" || die "IEC-104 slave did not bind (see $WORK/gw.log)"
  grep -a "iec104.*listening" "$WORK/gw.log" | tail -1 | sed 's/^/   /'
}

# ── edge + sim ────────────────────────────────────────────────────────────────
start_edge(){
  say "starting Modbus sim + edge producer (group $GROUP)"
  cat >"$WORK/edge.json" <<JSON
{
  "mqtt": { "broker": "$MQTT", "clientId": "edge-soak", "qos": 1, "keepalive": 60, "publishBatchMs": 200, "tls": { "enabled": false } },
  "sparkplug": { "groupId": "$GROUP", "nodeId": "edge-1", "plantaAlias": "Soak Plant", "birthOnConfigChange": true },
  "outstations": [],
  "modbusDevices": [
    { "id": "plc-sim", "label": "Sim PLC", "host": "127.0.0.1", "port": $MODBUS_PORT, "unitId": 1,
      "scanRateMs": 2000, "timeoutMs": 2000, "retries": 1, "retryDelayMs": 500, "enabled": true }
  ],
  "mappings": [
    { "id": "n1", "metricName": "PLC/tank_level", "protocol": "modbus", "sourceId": "plc-sim",
      "function": "holding_register", "address": 0, "dataType": "float32", "byteOrder": "ABCD",
      "scale": 1, "engineeringUnit": "m", "deadband": 0.05, "enabled": true },
    { "id": "d1", "metricName": "Medidas/Energy_kWh", "deviceId": "meter-01", "protocol": "modbus",
      "sourceId": "plc-sim", "function": "holding_register", "address": 0, "dataType": "float32",
      "byteOrder": "ABCD", "scale": 1, "engineeringUnit": "kWh", "deadband": 0.05,
      "serveDnp3": true, "outType": "analog", "outIndex": 0, "enabled": true },
    { "id": "d2", "metricName": "Medidas/Power_kW", "deviceId": "meter-01", "protocol": "modbus",
      "sourceId": "plc-sim", "function": "holding_register", "address": 2, "dataType": "uint16",
      "scale": 1, "engineeringUnit": "kW", "deadband": 0.05,
      "serveDnp3": true, "outType": "analog", "outIndex": 1, "enabled": true }
  ],
  "dnp3Server": { "enabled": true, "bindHost": "127.0.0.1", "port": $DNP3_OSTN_PORT,
    "localAddress": 1024, "masterAddress": 1, "eventBufferSize": 100 },
  "system": { "enabled": true, "intervalMs": 5000, "metricPrefix": "System/", "mounts": ["/"],
    "interfaces": ["docker0"],
    "metrics": { "cpu": true, "load": false, "memory": true, "swap": false, "disk": true,
                 "network": true, "networkRates": true, "temperature": false, "uptime": true, "processes": false } }
}
JSON
  setsid "$WORK/modbusslave" "$MODBUS_PORT" >"$WORK/sim.log" 2>&1 </dev/null & echo $! >"$WORK/.sim.pid"; sleep 1
  setsid "$WORK/edge" -port "$EDGE_HTTP" -config "$WORK/edge.json" -log info >"$WORK/edge.log" 2>&1 </dev/null & echo $! >"$WORK/.edge.pid"; sleep 3
  curl -s -X POST "http://localhost:${EDGE_HTTP}/api/gateway/start" >/dev/null
}

# ── go104 (IEC-104 master = destination) ──────────────────────────────────────
# Seeds a line to the gateway slave, plus one signal per IEC-104 point the bridge
# created (IOA + name + type read back from expose.json), then runs go104.
# StartAll() auto-connects enabled lines on boot (STARTDT → GI → spontaneous), so
# the line dials the gateway and records each bridged signal into its datapoints
# table — the soak's proof that SSFV-catalogued data reached the far end.
start_go104(){
  say "starting go104 (IEC-104 master) → gateway slave 127.0.0.1:$IEC_PORT"
  rm -f "$WORK/go104.db"
  # First boot creates the schema; stop, seed config, restart so StartAll picks
  # it up (mirrors the gateway's configure-then-restart pattern).
  setsid env DB_PATH="$WORK/go104.db" HTTP_PORT="$GO104_HTTP" "$WORK/go104" >"$WORK/go104.log" 2>&1 </dev/null & local p=$!
  sleep 3; kill "$p" 2>/dev/null || true; sleep 1
  sqlite3 "$WORK/go104.db" "INSERT INTO lines(name,host,port,common_address,enabled,gi_interval_s) VALUES('gw-soak','127.0.0.1',$IEC_PORT,1,1,60);"
  # One go104 signal per bridge-created mapping (ioa | name | type_id | kind).
  python3 - "$WORK/expose.json" <<'PY' | while read -r ioa name typeid kind; do
import json, sys
TYPEMAP = {"M_ME_NC_1": (13, "analog"), "M_SP_NA_1": (1, "digital"), "M_ME_TF_1": (36, "analog")}
d = json.load(open(sys.argv[1]))
for m in d.get("created", []):
    t, kind = TYPEMAP.get(m.get("iec104_type", ""), (13, "analog"))
    name = (m.get("metric_name") or "").split("/")[-1] or ("ioa%d" % m["ioa"])
    print(m["ioa"], name, t, kind)
PY
    sqlite3 "$WORK/go104.db" "INSERT INTO signals(line_id,name,ioa,type_id,signal_type,scale) SELECT id,'$name',$ioa,$typeid,'$kind',1.0 FROM lines WHERE name='gw-soak';"
  done
  echo "   go104 signals seeded: $(sqlite3 "$WORK/go104.db" 'SELECT count(*) FROM signals;')"
  # DNP3 line: go104 polls the edge's DNP3 outstation directly (edge-direct path,
  # independent of the SSFV plant). Two analog points mirror Energy#0 / Power#1.
  sqlite3 "$WORK/go104.db" "INSERT INTO lines(name,host,port,protocol,dnp3_outstation_addr,dnp3_master_addr,gi_interval_s,enabled) VALUES('edge-dnp3','127.0.0.1',$DNP3_OSTN_PORT,'dnp3',1024,1,30,1);"
  sqlite3 "$WORK/go104.db" "INSERT INTO signals(line_id,name,ioa,type_id,signal_type,point_type,scale) SELECT id,'Energy_kWh',0,0,'analog','analog',1.0 FROM lines WHERE name='edge-dnp3';"
  sqlite3 "$WORK/go104.db" "INSERT INTO signals(line_id,name,ioa,type_id,signal_type,point_type,scale) SELECT id,'Power_kW',1,0,'analog','analog',1.0 FROM lines WHERE name='edge-dnp3';"
  echo "   go104 DNP3 line seeded → 127.0.0.1:$DNP3_OSTN_PORT (outstation 1024)"
  setsid env DB_PATH="$WORK/go104.db" HTTP_PORT="$GO104_HTTP" "$WORK/go104" >"$WORK/go104.log" 2>&1 </dev/null & echo $! >"$WORK/.go104.pid"
  sleep 5
  grep -aq "GI complete" "$WORK/go104.log" && echo "   go104 IEC-104 line ACTIVE (GI complete)" \
    || echo "   WARNING: go104 GI not completed yet (see $WORK/go104.log)"
  grep -aq "TCP accept" "$WORK/gw.log" && echo "   gateway accepted go104 connection" || true
  grep -aq '\[DNP3\].*ACTIVE' "$WORK/go104.log" && echo "   go104 DNP3 line ACTIVE (polling edge outstation)" \
    || echo "   WARNING: go104 DNP3 line not active yet (see $WORK/go104.log)"
}

# ── approve (create planta inline; bind the two discovered device signals) ────
approve(){
  say "waiting for autodiscovery of meter-01"
  local mid=""
  for _ in $(seq 1 20); do
    mid=$(sqlite3 "$WORK/gw.db" "SELECT id FROM autodiscovered_entities WHERE device_id='meter-01' LIMIT 1;" 2>/dev/null || true)
    [ -n "$mid" ] && break; sleep 2
  done
  [ -n "$mid" ] || die "meter-01 was never autodiscovered (see $WORK/gw.log / $WORK/edge.log)"
  say "approving meter-01 (id=$mid) — planta created inline, 2 signals bound directly"
  local resp
  resp=$(curl -s -X POST "$API/ssfv/autodiscovered/$mid/approve" -H 'Content-Type: application/json' -d "{
     \"planta_id\":0,
     \"create_planta\":{\"nombre\":\"Soak Plant\",\"broker_base\":\"$GROUP\"},
     \"tipo_id\":2, \"nombre_equipo\":\"meter-01\", \"nombre_topic\":\"$GROUP/edge-1/meter-01\",
     \"create_signals\":[
       {\"codigo_senal\":\"Energy_kWh\",\"nombre\":\"Energia\",\"tipavar_id\":6,\"unidad_id\":6,\"tipo_valor\":\"Acumulado\",\"nombre_instancia\":\"Medidas\"},
       {\"codigo_senal\":\"Power_kW\",\"nombre\":\"Potencia\",\"tipavar_id\":5,\"unidad_id\":3,\"tipo_valor\":\"Instantaneo\",\"nombre_instancia\":\"Medidas\"}
     ]}")
  echo "$resp" | python3 -m json.tool
  EQUIPO_ID=$(echo "$resp" | python3 -c "import sys,json;print(json.load(sys.stdin).get('equipo_id',''))" 2>/dev/null)
  PID=$(TS "SELECT planta_id FROM ssfv.tbl_planta WHERE broker_base='$GROUP';")
  [ -n "$PID" ] || die "planta was not created"
  [ -n "$EQUIPO_ID" ] || die "approval did not return an equipo_id"
  echo "   planta_id=$PID equipo_id=$EQUIPO_ID"
}

# ── link the plant to an IEC-104 line (mirrors its SSFV signals) ──────────────
# SSFV is the principal; IEC-104 is a dependent mirror. Linking the plant to a
# 104 server (operator-chosen) backfills mirrors for its catalog signals
# (entity/codigo/instancia/type/unit), deriving the type and assigning a
# contiguous IOA block from IOA_BASE. Plants may share a server; IOAs stay unique
# per server. New signals then auto-mirror, and deleting the plant/equipo/signal
# cascade-deletes the mirrors. The created mappings drive go104's signal seeding.
IOA_BASE="${IOA_BASE:-1001}"
link_iec104(){
  say "linking plant $PID → IEC-104 server 1 (IOA base $IOA_BASE) — mirrors its SSFV signals"
  curl -s -X POST "$API/ssfv/plantas/$PID/iec104-link" \
    -H 'Content-Type: application/json' -d "{\"server_id\":1,\"ioa_start\":$IOA_BASE}" >"$WORK/expose.json"
  python3 -m json.tool <"$WORK/expose.json" || { cat "$WORK/expose.json"; die "iec104-link failed"; }
  local n
  n=$(python3 -c "import json;print(len(json.load(open('$WORK/expose.json')).get('created',[])))" 2>/dev/null || echo 0)
  [ "$n" -ge 1 ] || die "link created no IEC-104 mirrors (see $WORK/expose.json / $WORK/gw.log)"
  echo "   linked: $n IEC-104 mirror(s) created"
}

# ── soak loop ─────────────────────────────────────────────────────────────────
soak(){
  local mins="${1:-10}"
  local samples=$(( mins * 2 ))  # 30s cadence
  local csv="$WORK/soak.csv"
  say "soak: ${mins} min, sample/30s, delete planta $PID at sample $DELETE_AT"
  echo "sample,elapsed_s,valores,delta,plantas_visible,p_visible,p_estado,active_bindings,pending,gw_maps,g104_pts,g104_val,dnp3_pts,dnp3_val" >"$csv"
  local prev=0 start; start=$(date +%s)
  for i in $(seq 1 "$samples"); do
    local now el; now=$(date +%s); el=$((now-start))
    if [ "$i" -eq "$DELETE_AT" ]; then
      local code; code=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$API/ssfv/plantas/$PID")
      echo "   >>> T+${el}s sample $i: DELETE /ssfv/plantas/$PID -> HTTP $code"
    fi
    local val delta pv pvis pest ab pend gwm g104n g104v dnp3n dnp3v
    val=$(TS "SELECT count(*) FROM ssfv.tbl_valores;"); val=${val:-0}; delta=$((val-prev)); prev=$val
    pv=$(curl -s "$API/ssfv/plantas" | python3 -c "import sys,json;print(len(json.load(sys.stdin)))" 2>/dev/null || echo "?")
    pvis=$(curl -s "$API/ssfv/plantas" | python3 -c "import sys,json;print(int(any(p['planta_id']==$PID for p in json.load(sys.stdin))))" 2>/dev/null || echo "?")
    pest=$(TS "SELECT estado FROM ssfv.tbl_planta WHERE planta_id=$PID;")
    ab=$(TS "SELECT count(*) FROM ssfv.tbl_senales_x_equipo sxe JOIN ssfv.tbl_equipo e ON e.equipo_id=sxe.equipo_id WHERE e.planta_id=$PID AND sxe.activo=TRUE;")
    pend=$(curl -s "$API/ssfv/autodiscovered" | python3 -c "import sys,json;print(sum(1 for e in json.load(sys.stdin) if e['status']=='pending'))" 2>/dev/null || echo "?")
    # Cascade proof: IEC-104 mirrors in the gateway for this plant. After the plant
    # delete this MUST drop to 0 (SSFV is principal; 104 is a dependent mirror).
    gwm=$(sqlite3 "$WORK/gw.db" "SELECT count(*) FROM signal_mappings WHERE ssfv_planta_id=$PID;" 2>/dev/null); gwm=${gwm:-0}
    # IEC-104 path (gateway-mediated): scope to the gw-soak line so the DNP3 line's
    # IOA 0/1 don't collide. These STOP updating after the plant delete (cascade).
    g104n=$(sqlite3 "$WORK/go104.db" "SELECT count(*) FROM datapoints WHERE line_id=(SELECT id FROM lines WHERE name='gw-soak');" 2>/dev/null); g104n=${g104n:-0}
    g104v=$(sqlite3 "$WORK/go104.db" "SELECT printf('%.2f',value) FROM datapoints WHERE line_id=(SELECT id FROM lines WHERE name='gw-soak') ORDER BY ioa LIMIT 1;" 2>/dev/null); g104v=${g104v:-NA}
    # DNP3 path (edge-direct): go104 polls the edge outstation; independent of the
    # SSFV plant, so it KEEPS updating after the plant delete.
    dnp3n=$(sqlite3 "$WORK/go104.db" "SELECT count(*) FROM datapoints WHERE line_id=(SELECT id FROM lines WHERE name='edge-dnp3');" 2>/dev/null); dnp3n=${dnp3n:-0}
    dnp3v=$(sqlite3 "$WORK/go104.db" "SELECT printf('%.2f',value) FROM datapoints WHERE line_id=(SELECT id FROM lines WHERE name='edge-dnp3') ORDER BY ioa LIMIT 1;" 2>/dev/null); dnp3v=${dnp3v:-NA}
    echo "$i,$el,$val,$delta,$pv,$pvis,$pest,$ab,$pend,$gwm,$g104n,$g104v,$dnp3n,$dnp3v" | tee -a "$csv"
    [ "$i" -lt "$samples" ] && sleep 30
  done
  say "soak done — CSV at $csv"
  echo "expected: ingest>0 then delta->0 after delete; p_visible 1->0; active_bindings ->0;"
  echo "          gw_maps drops 2->0 at the delete (mirror cascade), and g104_val"
  echo "          stops changing afterward — SSFV is principal, IEC-104 a dependent"
  echo "          mirror that is removed with its plant."
  echo "          DNP3 path is edge-direct: dnp3_pts>=2 and dnp3_val keeps changing"
  echo "          even AFTER the plant delete (the edge outstation is independent"
  echo "          of SSFV) — proving the new DNP3 outstation served go104 e2e."
}

# ── entrypoint ────────────────────────────────────────────────────────────────
case "${1:-run}" in
  down) down ;;
  up)   down; build; infra; start_gw; start_edge; approve; link_iec104; start_go104;
        say "stack up. API $API · go104 :$GO104_HTTP · CSV target $WORK/soak.csv · 'scripts/soak.sh down' to clean" ;;
  run)  down; build; infra; start_gw; start_edge; approve; link_iec104; start_go104; soak "${2:-10}" ;;
  *)    die "usage: $0 {run [MINUTES]|up|down}" ;;
esac
