-- SQLite schema for goGateway.

-- Devices are scoped to one IEC-104 slave: a "device" is the asset as it
-- appears in the SCADA point list of a specific endpoint. The same physical
-- inverter exposed to two SCADA masters is two device rows (one per server).
CREATE TABLE IF NOT EXISTS devices (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id   INTEGER NOT NULL REFERENCES iec104_servers(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT DEFAULT '',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (server_id, name)
);
CREATE INDEX IF NOT EXISTS idx_devices_server ON devices(server_id);

CREATE TABLE IF NOT EXISTS mqtt_config (
    id                INTEGER PRIMARY KEY CHECK (id = 1),
    host              TEXT    NOT NULL DEFAULT 'localhost',
    port              INTEGER NOT NULL DEFAULT 1883,
    username          TEXT    DEFAULT '',
    password          TEXT    DEFAULT '',
    client_id         TEXT    DEFAULT 'goGateway',
    use_tls           INTEGER NOT NULL DEFAULT 0,
    sparkplug_enabled INTEGER NOT NULL DEFAULT 0,
    sp_group_id       TEXT    NOT NULL DEFAULT 'goGateway',
    sp_host_id        TEXT    NOT NULL DEFAULT 'goGateway-host'
);
INSERT OR IGNORE INTO mqtt_config (id) VALUES (1);

CREATE TABLE IF NOT EXISTS topics (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    topic     TEXT NOT NULL,
    qos       INTEGER NOT NULL DEFAULT 0,
    enabled   INTEGER NOT NULL DEFAULT 1,
    UNIQUE (device_id, topic)
);
CREATE INDEX IF NOT EXISTS idx_topics_enabled ON topics(enabled);

-- IEC 60870-5-104 gateway-wide settings. Singleton (id=1). The listen_ip is
-- the IP this host binds on for ALL slave endpoints; SCADA masters connect to
-- listen_ip:<port-of-server-row>. Use 0.0.0.0 to bind every NIC.
CREATE TABLE IF NOT EXISTS iec104_gateway (
    id        INTEGER PRIMARY KEY CHECK (id = 1),
    listen_ip TEXT    NOT NULL DEFAULT '0.0.0.0'
);
INSERT OR IGNORE INTO iec104_gateway (id, listen_ip) VALUES (1, '0.0.0.0');

-- IEC 60870-5-104 slave fleet. One row per passive listener (port). Each row
-- carries its own Common ASDU Address so multiple SCADA masters can read the
-- gateway under different ASDUs. scada_ips is a CSV allowlist: only those
-- remote IPs may complete the TCP handshake. Empty allowlist = block all.
CREATE TABLE IF NOT EXISTS iec104_servers (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL DEFAULT '',
    port        INTEGER NOT NULL DEFAULT 2404,
    asdu_addr   INTEGER NOT NULL DEFAULT 1,
    scada_ips   TEXT NOT NULL DEFAULT '',
    k           INTEGER NOT NULL DEFAULT 12,
    w           INTEGER NOT NULL DEFAULT 8,
    t0          INTEGER NOT NULL DEFAULT 30,
    t1          INTEGER NOT NULL DEFAULT 15,
    t2          INTEGER NOT NULL DEFAULT 10,
    t3          INTEGER NOT NULL DEFAULT 20,
    enabled     INTEGER NOT NULL DEFAULT 0,
    UNIQUE (port)
);
INSERT OR IGNORE INTO iec104_servers
    (id, name, port, asdu_addr, scada_ips, k, w, t0, t1, t2, t3, enabled)
    VALUES (1, 'default', 2404, 1, '', 12, 8, 30, 15, 10, 20, 0);

-- Legacy singleton table, retired when multi-server support landed.
DROP TABLE IF EXISTS iec104_config;

-- signal_mappings: each row is a point on ONE specific IEC-104 slave endpoint.
-- IOA is unique within a server (not globally), since each server has its own
-- ASDU address space. Worker dispatch routes every MQTT sample to the single
-- server pinned by server_id; GI from a SCADA master returns only that
-- server's slice of the point set.
CREATE TABLE IF NOT EXISTS signal_mappings (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id      INTEGER NOT NULL REFERENCES iec104_servers(id) ON DELETE CASCADE,
    topic_id       INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
    device_name    TEXT DEFAULT '',
    variable_type  TEXT DEFAULT '',
    characteristic TEXT DEFAULT '',
    json_key       TEXT NOT NULL,
    quality_key    TEXT NOT NULL DEFAULT '',
    metric_name    TEXT NOT NULL DEFAULT '',
    iec104_type    TEXT NOT NULL,
    ioa            INTEGER NOT NULL,
    unit           TEXT DEFAULT '',
    scale          REAL NOT NULL DEFAULT 1.0,
    enabled        INTEGER NOT NULL DEFAULT 1,
    UNIQUE (server_id, ioa)
);
CREATE INDEX IF NOT EXISTS idx_sigmap_topic ON signal_mappings(topic_id);
CREATE INDEX IF NOT EXISTS idx_sigmap_server ON signal_mappings(server_id);

CREATE TABLE IF NOT EXISTS history (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    mapping_id INTEGER NOT NULL REFERENCES signal_mappings(id) ON DELETE CASCADE,
    signal_key TEXT NOT NULL,
    value      REAL NOT NULL,
    quality    INTEGER NOT NULL DEFAULT 0,
    timestamp  DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_history_mapping_ts ON history(mapping_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_history_ts ON history(timestamp DESC);
