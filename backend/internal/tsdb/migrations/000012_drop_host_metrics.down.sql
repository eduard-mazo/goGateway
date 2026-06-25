-- Rollback of migration 012: recreate the host-telemetry storage (mirrors 011).
-- Not used by the runner (it applies *.up.sql only); kept for manual rollback.

CREATE TABLE IF NOT EXISTS ssfv.cfg_host_iface (
    iface_id       SERIAL       PRIMARY KEY,
    nombre         VARCHAR(60)  NOT NULL UNIQUE,
    descripcion    VARCHAR(120),
    activo         BOOLEAN      NOT NULL DEFAULT TRUE,
    fecha_creacion TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

INSERT INTO ssfv.cfg_host_iface (nombre, descripcion) VALUES
    ('eth0',       'Ethernet primaria'),
    ('eth1',       'Ethernet secundaria'),
    ('wlan0',      'WiFi'),
    ('wlp2s0',     'WiFi'),
    ('en0',        'Ethernet (macOS)'),
    ('tailscale0', 'VPN Tailscale'),
    ('docker0',    'Bridge Docker por defecto')
ON CONFLICT (nombre) DO NOTHING;

CREATE TABLE IF NOT EXISTS ssfv.tbl_metricas_host (
    timestamp_utc TIMESTAMPTZ  NOT NULL,
    node_topic    VARCHAR(150) NOT NULL,
    categoria     VARCHAR(40)  NOT NULL,
    subkey        VARCHAR(60)  NOT NULL DEFAULT '',
    metrica       VARCHAR(60)  NOT NULL,
    valor         NUMERIC(18,6),
    calidad       VARCHAR(10)  NOT NULL DEFAULT 'Buena'
                               CHECK (calidad IN ('Buena', 'Dudosa', 'Mala')),
    CONSTRAINT pk_metricas_host
        PRIMARY KEY (timestamp_utc, node_topic, categoria, subkey, metrica)
);

SELECT create_hypertable('ssfv.tbl_metricas_host', 'timestamp_utc',
    chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_metricas_host_lookup
    ON ssfv.tbl_metricas_host(node_topic, categoria, subkey, metrica, timestamp_utc DESC);

DO $$ BEGIN
    ALTER TABLE ssfv.tbl_metricas_host SET (
        timescaledb.compress,
        timescaledb.compress_orderby   = 'timestamp_utc DESC',
        timescaledb.compress_segmentby = 'node_topic, categoria, subkey, metrica');
EXCEPTION WHEN OTHERS THEN NULL; END $$;

DO $$ BEGIN
    PERFORM add_compression_policy('ssfv.tbl_metricas_host', compress_after => INTERVAL '7 days');
EXCEPTION WHEN OTHERS THEN NULL; END $$;

DO $$ BEGIN
    PERFORM add_retention_policy('ssfv.tbl_metricas_host', drop_after => INTERVAL '90 days');
EXCEPTION WHEN OTHERS THEN NULL; END $$;

CREATE OR REPLACE VIEW ssfv.v_host_ultimas AS
SELECT DISTINCT ON (node_topic, categoria, subkey, metrica)
    node_topic, categoria, subkey, metrica, timestamp_utc, valor, calidad
FROM ssfv.tbl_metricas_host
ORDER BY node_topic, categoria, subkey, metrica, timestamp_utc DESC;
