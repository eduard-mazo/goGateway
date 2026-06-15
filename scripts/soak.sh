#!/usr/bin/env bash
# =============================================================================
# soak.sh — one-command reproduction of the goMqttModbus → goGateway soak.
#
# Brings up a throwaway stack (TimescaleDB + edge + Modbus sim), lets the
# gateway autodiscover the edge, approves it (creating its planta inline —
# nothing is pre-seeded), then runs a timed soak that ingests data and deletes
# the planta mid-run to prove: discovery-driven binding, clean delete (bindings
# deactivated, ingest stops), and no re-surfacing.
#
# Usage:
#   scripts/soak.sh run [MINUTES]   # build + bring up + soak (default 10 min)
#   scripts/soak.sh up              # build + bring up + approve, no soak loop
#   scripts/soak.sh down            # tear everything down
#
# Env overrides (defaults in parens):
#   GOGW_DIR (repo root)   EDGE_DIR (../goMqttModbus)   WORK (/tmp/e2e-soak)
#   TSDB_PORT (5434)  GW_HTTP (:8091)  EDGE_HTTP (8090)  MODBUS_PORT (1502)
#   MQTT (tcp://localhost:1883)  GROUP (EPM_SOAK)  DELETE_AT (8)
#
# Requires: docker, go, sqlite3, python3, curl, and an MQTT broker on :1883
# (reused if already up — e.g. the shared mosquitto — else one is started).
# =============================================================================
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GOGW_DIR="${GOGW_DIR:-$(cd "$HERE/.." && pwd)}"
EDGE_DIR="${EDGE_DIR:-$(cd "$GOGW_DIR/../goMqttModbus" 2>/dev/null && pwd || true)}"
WORK="${WORK:-/tmp/e2e-soak}"
TSDB_PORT="${TSDB_PORT:-5434}"
GW_HTTP="${GW_HTTP:-:8091}"
EDGE_HTTP="${EDGE_HTTP:-8090}"
MODBUS_PORT="${MODBUS_PORT:-1502}"
MQTT="${MQTT:-tcp://localhost:1883}"
GROUP="${GROUP:-EPM_SOAK}"
DELETE_AT="${DELETE_AT:-8}"

API="http://localhost${GW_HTTP}/api"
DSN="postgres://postgres:pass@localhost:${TSDB_PORT}/gwtest"
TS(){ docker exec e2e-soak-tsdb psql -U postgres -d gwtest -tAc "$1" 2>/dev/null | tr -d ' '; }
say(){ printf '\033[1;36m== %s\033[0m\n' "$*"; }
die(){ printf '\033[1;31mERROR: %s\033[0m\n' "$*" >&2; exit 1; }

# ── teardown ────────────────────────────────────────────────────────────────
down(){
  say "tearing down"
  for f in "$WORK"/.gw.pid "$WORK"/.edge.pid "$WORK"/.sim.pid "$WORK"/.mosq.pid; do
    [ -f "$f" ] && { kill "$(cat "$f")" 2>/dev/null || true; rm -f "$f"; }
  done
  docker rm -f e2e-soak-tsdb e2e-soak-mosq >/dev/null 2>&1 || true
  echo "done."
}

# ── build ───────────────────────────────────────────────────────────────────
build(){
  [ -n "$EDGE_DIR" ] && [ -d "$EDGE_DIR" ] || die "goMqttModbus not found (set EDGE_DIR)"
  mkdir -p "$WORK/data"
  say "building gateway (backend)"
  ( cd "$GOGW_DIR/backend" && go build -o "$WORK/gogw" ./cmd/gateway )
  say "building edge + modbus sim"
  ( cd "$EDGE_DIR" && go build -o "$WORK/edge" . && go build -o "$WORK/modbusslave" ./scripts/sim/modbusslave )
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
  sqlite3 "$WORK/gw.db" "UPDATE mqtt_config SET sparkplug_enabled=1, sp_group_id='$GROUP', host='localhost', port=1883, qos=1 WHERE id=1;"
  sqlite3 "$WORK/gw.db" "UPDATE tsdb_config SET backend='timescaledb', ts_dsn='$DSN', enabled=1, wal_path='$WORK/data/wal.bolt', dlq_path='$WORK/data/dlq.bolt', flush_ms=200 WHERE id=1;"
  setsid env GW_DB="$WORK/gw.db" GW_HTTP="$GW_HTTP" "$WORK/gogw" >"$WORK/gw.log" 2>&1 </dev/null & echo $! >"$WORK/.gw.pid"
  sleep 6
  grep -aq "SSFV adapter connected" "$WORK/gw.log" || die "gateway did not connect (see $WORK/gw.log)"
  echo "   migrations applied: $(grep -ac 'applied migration' "$WORK/gw.log")"
  grep -a "mapping cache loaded" "$WORK/gw.log" | tail -1 | sed 's/^/   /'
}

# ── edge + sim ────────────────────────────────────────────────────────────────
start_edge(){
  say "starting Modbus sim + edge producer (group $GROUP)"
  cat >"$WORK/edge.json" <<JSON
{
  "mqtt": { "broker": "$MQTT", "clientId": "edge-soak", "qos": 1, "keepalive": 60, "tls": { "enabled": false } },
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
      "byteOrder": "ABCD", "scale": 1, "engineeringUnit": "kWh", "deadband": 0.05, "enabled": true },
    { "id": "d2", "metricName": "Medidas/Power_kW", "deviceId": "meter-01", "protocol": "modbus",
      "sourceId": "plc-sim", "function": "holding_register", "address": 2, "dataType": "float32",
      "byteOrder": "ABCD", "scale": 1, "engineeringUnit": "kW", "deadband": 0.05, "enabled": true }
  ],
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
  curl -s -X POST "$API/ssfv/autodiscovered/$mid/approve" -H 'Content-Type: application/json' -d "{
     \"planta_id\":0,
     \"create_planta\":{\"nombre\":\"Soak Plant\",\"broker_base\":\"$GROUP\"},
     \"tipo_id\":2, \"nombre_equipo\":\"meter-01\", \"nombre_topic\":\"$GROUP/edge-1/meter-01\",
     \"create_signals\":[
       {\"codigo_senal\":\"Energy_kWh\",\"nombre\":\"Energia\",\"tipavar_id\":6,\"unidad_id\":6,\"tipo_valor\":\"Acumulado\",\"nombre_instancia\":\"Medidas\"},
       {\"codigo_senal\":\"Power_kW\",\"nombre\":\"Potencia\",\"tipavar_id\":5,\"unidad_id\":3,\"tipo_valor\":\"Instantaneo\",\"nombre_instancia\":\"Medidas\"}
     ]}" | python3 -m json.tool
  PID=$(TS "SELECT planta_id FROM ssfv.tbl_planta WHERE broker_base='$GROUP';")
  [ -n "$PID" ] || die "planta was not created"
  echo "   planta_id=$PID"
}

# ── soak loop ─────────────────────────────────────────────────────────────────
soak(){
  local mins="${1:-10}"
  local samples=$(( mins * 2 ))  # 30s cadence
  local csv="$WORK/soak.csv"
  say "soak: ${mins} min, sample/30s, delete planta $PID at sample $DELETE_AT"
  echo "sample,elapsed_s,valores,delta,plantas_visible,p_visible,p_estado,active_bindings,pending" >"$csv"
  local prev=0 start; start=$(date +%s)
  for i in $(seq 1 "$samples"); do
    local now el; now=$(date +%s); el=$((now-start))
    if [ "$i" -eq "$DELETE_AT" ]; then
      local code; code=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$API/ssfv/plantas/$PID")
      echo "   >>> T+${el}s sample $i: DELETE /ssfv/plantas/$PID -> HTTP $code"
    fi
    local val delta pv pvis pest ab pend
    val=$(TS "SELECT count(*) FROM ssfv.tbl_valores;"); val=${val:-0}; delta=$((val-prev)); prev=$val
    pv=$(curl -s "$API/ssfv/plantas" | python3 -c "import sys,json;print(len(json.load(sys.stdin)))" 2>/dev/null || echo "?")
    pvis=$(curl -s "$API/ssfv/plantas" | python3 -c "import sys,json;print(int(any(p['planta_id']==$PID for p in json.load(sys.stdin))))" 2>/dev/null || echo "?")
    pest=$(TS "SELECT estado FROM ssfv.tbl_planta WHERE planta_id=$PID;")
    ab=$(TS "SELECT count(*) FROM ssfv.tbl_senales_x_equipo sxe JOIN ssfv.tbl_equipo e ON e.equipo_id=sxe.equipo_id WHERE e.planta_id=$PID AND sxe.activo=TRUE;")
    pend=$(curl -s "$API/ssfv/autodiscovered" | python3 -c "import sys,json;print(sum(1 for e in json.load(sys.stdin) if e['status']=='pending'))" 2>/dev/null || echo "?")
    echo "$i,$el,$val,$delta,$pv,$pvis,$pest,$ab,$pend" | tee -a "$csv"
    [ "$i" -lt "$samples" ] && sleep 30
  done
  say "soak done — CSV at $csv"
  echo "expected: ingest>0 then delta->0 after delete; p_visible 1->0; active_bindings ->0"
}

# ── entrypoint ────────────────────────────────────────────────────────────────
case "${1:-run}" in
  down) down ;;
  up)   down; build; infra; start_gw; start_edge; approve;
        say "stack up. API $API · CSV target $WORK/soak.csv · 'scripts/soak.sh down' to clean" ;;
  run)  down; build; infra; start_gw; start_edge; approve; soak "${2:-10}" ;;
  *)    die "usage: $0 {run [MINUTES]|up|down}" ;;
esac
