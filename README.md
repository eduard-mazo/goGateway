# goGateway

MQTT → IEC 60870-5-104 bridge.  Receives process data over MQTT (plain JSON or
Sparkplug B protobuf) and exposes it as an IEC 60870-5-104 slave to a SCADA
system.

## Quick start

```
make build          # build frontend + embed + Go binary (Linux)
make build-win      # cross-compile Windows/amd64 binary
./goGateway         # listens on :8080 by default
```

Open `http://localhost:8080` for the web UI.

## MQTT modes

| Mode | `sparkplug_enabled` | Payload format | Topic format |
|------|---------------------|----------------|--------------|
| JSON | `0` (default) | Arbitrary JSON object | Any string |
| Sparkplug B | `1` | Protobuf (Eclipse Sparkplug B v2.2) | `spBv1.0/{group}/{MSG_TYPE}/{node}[/{device}]` |

Switch modes in **UI → MQTT Config → Sparkplug B**.

See [docs/sparkplug-b.md](docs/sparkplug-b.md) for Sparkplug B topic/payload
examples and step-by-step operator configuration.

## Architecture

```
MQTT broker
    │
    ▼
mqtt.Manager ──[sparkplug_enabled=0]──► worker.ParseAndDispatch (JSON)
    │           [sparkplug_enabled=1]──► worker.SparkplugHandler (protobuf)
    │
    ▼
iec104.Server  (one slave per iec104_servers row)
    │
    ▼
SCADA / RTU  (IEC 60870-5-104 client)
```

## Configuration

All configuration is persisted in a SQLite database (`goGateway.db` by default).
The web UI exposes every setting.

| Table | Purpose |
|-------|---------|
| `mqtt_config` | Broker address, credentials, TLS, Sparkplug B settings |
| `iec104_gateway` | Gateway-wide IEC-104 listen IP |
| `iec104_servers` | One row per IEC-104 slave endpoint |
| `devices` | Logical grouping of topics (scoped to a server) |
| `topics` | MQTT topics to subscribe to |
| `signal_mappings` | Topic/metric → IEC-104 IOA bindings |

## Development

```
make test           # Go unit tests
make vet            # go vet
pnpm --dir frontend install
pnpm --dir frontend dev    # Vite dev server with hot-reload (proxy to :8080)
```
