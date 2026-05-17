-- Extensions
CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

-- Raw signals table
CREATE TABLE IF NOT EXISTS signals (
    ts          TIMESTAMPTZ      NOT NULL,
    measurement TEXT             NOT NULL,
    ioa         TEXT             NOT NULL DEFAULT '',
    value       DOUBLE PRECISION,
    quality     SMALLINT         NOT NULL DEFAULT 0,
    tags        JSONB            NOT NULL DEFAULT '{}'
);

-- Hypertable: 1-day chunks — at 2000 signals/sec ~2GB/day uncompressed
SELECT create_hypertable(
    'signals', 'ts',
    if_not_exists          => TRUE,
    chunk_time_interval    => INTERVAL '1 day',
    create_default_indexes => FALSE
);

-- Unique constraint for idempotent WAL replay via ON CONFLICT DO NOTHING
CREATE UNIQUE INDEX IF NOT EXISTS uq_signals_ts_meas_ioa
    ON signals (ts, measurement, ioa);

CREATE INDEX IF NOT EXISTS idx_signals_ioa_ts
    ON signals (ioa, ts DESC);

CREATE INDEX IF NOT EXISTS idx_signals_meas_ts
    ON signals (measurement, ts DESC);

-- Partial index for good-quality-only queries (quality = 0 means all bits clear)
CREATE INDEX IF NOT EXISTS idx_signals_good_quality
    ON signals (ioa, ts DESC)
    WHERE quality = 0;

-- JSONB GIN index for tag-based filtering  e.g. WHERE tags->>'source' = 'RTU_01'
CREATE INDEX IF NOT EXISTS idx_signals_tags
    ON signals USING GIN (tags);

COMMENT ON TABLE  signals             IS 'Raw time-series signal data from IEC-104/MQTT gateway';
COMMENT ON COLUMN signals.measurement IS 'IEC 60870-5-104 ASDU type, e.g. M_ME_NC_1';
COMMENT ON COLUMN signals.ioa         IS 'Information Object Address';
COMMENT ON COLUMN signals.quality     IS 'IEC 60870-5-104 quality descriptor bitmask: bit0=OV bit1=BL bit2=SB bit3=NT bit4=IV';
COMMENT ON COLUMN signals.tags        IS 'Freeform metadata: source, rtu_id, unit, signal_key';
