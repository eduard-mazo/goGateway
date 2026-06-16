#!/usr/bin/env bash
# =============================================================================
# soak-edgecases.sh — device-identity / collision edge cases on top of soak.sh.
#
# goGateway identifies a sender purely by the Sparkplug B topic triple
# (group_id / edge_node_id / device_id) — never by the producer's MQTT clientId
# (a subscriber cannot see it). This script exercises the consequences:
#
#   A. ADD DEVICES      — a new node/device (distinct topic) → distinct identity,
#                         own autodiscovery row, clean scaling.
#   B. DUPLICATED DEVICE — a 2nd producer claiming the SAME group/node/device
#                         with a different clientId → ONE shared NodeSession +
#                         ONE seq counter → seq collision → rebirth storm.
#   C. SIGNAL LOST      — producer drops off → stale marking, ingest stops.
#
# Run:  scripts/soak-edgecases.sh        (brings the base stack up itself)
#       scripts/soak-edgecases.sh down   (tear down, incl. extra edges)
#
# Honors the same env overrides as soak.sh (WORK, GROUP, MQTT, TSDB_PORT, …).
# =============================================================================
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOAK="$HERE/soak.sh"
WORK="${WORK:-/tmp/e2e-soak}"
GROUP="${GROUP:-EPM_SOAK}"
MQTT="${MQTT:-tcp://localhost:1883}"
MODBUS_PORT="${MODBUS_PORT:-1502}"
API="http://localhost${GW_HTTP:-:8091}/api"
TS(){ docker exec e2e-soak-tsdb psql -U postgres -d gwtest -tAc "$1" 2>/dev/null | tr -d ' '; }
say(){ printf '\n\033[1;36m=== %s\033[0m\n' "$*"; }
note(){ printf '   %s\n' "$*"; }

# spawn_edge <name> <clientId> <nodeId> <deviceId> <httpPort>
# Publishes one device under group/node with two Modbus-sourced metrics.
spawn_edge(){
  local name="$1" cid="$2" node="$3" dev="$4" port="$5"
  cat >"$WORK/edge-$name.json" <<JSON
{
  "mqtt": { "broker": "$MQTT", "clientId": "$cid", "qos": 1, "keepalive": 60, "tls": { "enabled": false } },
  "sparkplug": { "groupId": "$GROUP", "nodeId": "$node", "birthOnConfigChange": true },
  "outstations": [],
  "modbusDevices": [
    { "id": "plc-sim", "label": "Sim PLC", "host": "127.0.0.1", "port": $MODBUS_PORT, "unitId": 1,
      "scanRateMs": 2000, "timeoutMs": 2000, "retries": 1, "retryDelayMs": 500, "enabled": true }
  ],
  "mappings": [
    { "id": "d1", "metricName": "Medidas/Energy_kWh", "deviceId": "$dev", "protocol": "modbus",
      "sourceId": "plc-sim", "function": "holding_register", "address": 0, "dataType": "float32",
      "byteOrder": "ABCD", "scale": 1, "engineeringUnit": "kWh", "deadband": 0.05, "enabled": true },
    { "id": "d2", "metricName": "Medidas/Power_kW", "deviceId": "$dev", "protocol": "modbus",
      "sourceId": "plc-sim", "function": "holding_register", "address": 2, "dataType": "float32",
      "byteOrder": "ABCD", "scale": 1, "engineeringUnit": "kW", "deadband": 0.05, "enabled": true }
  ],
  "system": { "enabled": false }
}
JSON
  setsid "$WORK/edge" -port "$port" -config "$WORK/edge-$name.json" -log info \
    >"$WORK/edge-$name.log" 2>&1 </dev/null & echo $! >"$WORK/.edge-$name.pid"
  sleep 3
  curl -s -X POST "http://localhost:${port}/api/gateway/start" >/dev/null || true
}

kill_edge(){ local n="$1"; [ -f "$WORK/.edge-$n.pid" ] && { kill "$(cat "$WORK/.edge-$n.pid")" 2>/dev/null || true; rm -f "$WORK/.edge-$n.pid"; }; }

down(){
  for n in add dup; do kill_edge "$n"; done
  bash "$SOAK" down
}

gwcount(){ local n; n=$(grep -ac "$1" "$WORK/gw.log" 2>/dev/null); echo "${n:-0}"; }

if [ "${1:-run}" = "down" ]; then down; exit 0; fi

# ── Base stack: edge-1/meter-01 approved + ingesting ─────────────────────────
# Kill any extra edges left by a previous run first — soak.sh's own teardown
# only knows about the base edge/sim/gw, not this script's add/dup producers.
for n in add dup; do kill_edge "$n"; done
say "bringing up base stack (edge-1/meter-01)"
bash "$SOAK" up || { echo "base stack failed"; exit 1; }

base_val(){ TS "SELECT count(*) FROM ssfv.tbl_valores;"; }

# =============================================================================
say "SCENARIO A — ADD DEVICES (distinct topic → distinct identity)"
before_pending=$(curl -s "$API/ssfv/autodiscovered" | python3 -c "import sys,json;print(len(json.load(sys.stdin)))" 2>/dev/null || echo "?")
note "autodiscovered rows before: $before_pending"
spawn_edge add edge-2-cli edge-2 meter-02 8093
note "spawned edge-2/meter-02 (new node + new device); waiting 12s for births…"
sleep 12
note "autodiscovered (group/node/device → status):"
curl -s "$API/ssfv/autodiscovered" | python3 -c "
import sys,json
for e in json.load(sys.stdin):
    print('     %s/%s/%s  ->  %s' % (e['group_id'], e['node_id'], e.get('device_id') or '(node)', e['status']))" 2>/dev/null || true
note "EXPECT: edge-2 + meter-02 appear as NEW 'pending' rows, independent of edge-1."

# =============================================================================
say "SCENARIO B — DUPLICATED DEVICE (same group/node/device, different clientId)"
reb0=$(gwcount "requesting rebirth"); oos0=$(gwcount "out-of-sequence")
v0=$(base_val)
note "before:  rebirths=$reb0  out-of-seq=$oos0  tbl_valores=$v0"
note "spawning edge-DUP with clientId=edge-dup but SAME topic $GROUP/edge-1/meter-01 …"
spawn_edge dup edge-dup edge-1 meter-01 8094
note "both producers now publish the SAME Sparkplug identity; observing 30s…"
sleep 30
reb1=$(gwcount "requesting rebirth"); oos1=$(gwcount "out-of-sequence")
v1=$(base_val)
note "after:   rebirths=$reb1  out-of-seq=$oos1  tbl_valores=$v1"
note "Δ rebirths=$((reb1-reb0))  Δ out-of-seq=$((oos1-oos0))  Δ rows=$((v1-v0))"
note "distinct equipos for topic $GROUP/edge-1/meter-01 (UNIQUE nombre_topic):"
TS "SELECT count(*) FROM ssfv.tbl_equipo WHERE nombre_topic='$GROUP/edge-1/meter-01';" | sed 's/^/     /'
note "EXPECT: Δ out-of-seq / Δ rebirths spike (two senders share ONE node seq);"
note "        still exactly 1 equipo row — the 2nd sender is silently merged."
kill_edge dup
note "killed edge-DUP."

# =============================================================================
say "SCENARIO C — DEVICE SIGNAL LOST (producer drops off)"
v0=$(base_val); nd0=$(gwcount "NDEATH"); stale0=$(gwcount "marked stale")
note "before:  tbl_valores=$v0  NDEATH=$nd0  stale=$stale0"
note "stopping edge-1 producer (connection loss / LWT)…"
curl -s -X POST "http://localhost:${EDGE_HTTP:-8090}/api/gateway/stop" >/dev/null 2>&1 || true
kill_edge_base(){ [ -f "$WORK/.edge.pid" ] && kill "$(cat "$WORK/.edge.pid")" 2>/dev/null || true; }
kill_edge_base
note "observing 25s for stale marking + ingest stop…"
sleep 25
v1=$(base_val); nd1=$(gwcount "NDEATH"); stale1=$(gwcount "marked stale")
note "after:   tbl_valores=$v1  NDEATH=$nd1  stale=$stale1"
note "Δ rows during outage=$((v1-v0))  Δ NDEATH=$((nd1-nd0))  Δ stale=$((stale1-stale0))"
note "EXPECT: Δ rows → ~0 (ingest stopped); NDEATH/stale fire if LWT delivered."

say "edge cases complete — logs in $WORK/{gw,edge,edge-add,edge-dup}.log"
note "run 'scripts/soak-edgecases.sh down' to tear everything down."
