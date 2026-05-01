-- Per-IOA configuration: display names, units, alarm thresholds.
CREATE TABLE IF NOT EXISTS signal_meta (
    ioa          TEXT        PRIMARY KEY,
    display_name TEXT        NOT NULL DEFAULT '',
    unit         TEXT        NOT NULL DEFAULT '',
    description  TEXT,
    measurement  TEXT,
    source       TEXT,
    alarm_low    DOUBLE PRECISION,
    alarm_high   DOUBLE PRECISION,
    warn_low     DOUBLE PRECISION,
    warn_high    DOUBLE PRECISION,
    enabled      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_signal_meta_source      ON signal_meta (source);
CREATE INDEX IF NOT EXISTS idx_signal_meta_measurement ON signal_meta (measurement);

CREATE OR REPLACE FUNCTION fn_set_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER trg_signal_meta_updated_at
    BEFORE UPDATE ON signal_meta
    FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();

-- DLQ mirror: SQL-queryable copy of Go-side BoltDB DLQ for reporting.
-- Written async by the gateway; best-effort consistency.
CREATE TABLE IF NOT EXISTS dlq_entries (
    id          BIGSERIAL    PRIMARY KEY,
    backend     TEXT         NOT NULL,
    reason      TEXT         NOT NULL,
    retries     INT          NOT NULL DEFAULT 0,
    point_count INT          NOT NULL DEFAULT 0,
    payload     JSONB        NOT NULL DEFAULT '[]',
    replayed    BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    replayed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_dlq_created ON dlq_entries (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dlq_open    ON dlq_entries (created_at DESC) WHERE replayed = FALSE;

-- Pipeline health snapshots for trend graphs in the UI.
CREATE TABLE IF NOT EXISTS pipeline_health (
    ts          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    write_rate  DOUBLE PRECISION,
    input_queue INT,
    wal_pending INT,
    dlq_depth   INT,
    backends    JSONB NOT NULL DEFAULT '{}'
);

SELECT create_hypertable('pipeline_health', 'ts', if_not_exists => TRUE,
    chunk_time_interval => INTERVAL '1 day');

SELECT add_retention_policy('pipeline_health', drop_after => INTERVAL '30 days', if_not_exists => TRUE);

-- Latest value per IOA with alarm state (fast: uses idx_signals_ioa_ts)
CREATE OR REPLACE VIEW v_current_signals AS
SELECT DISTINCT ON (s.ioa)
    s.ts,
    s.measurement,
    s.ioa,
    s.value,
    s.quality,
    s.tags,
    m.display_name,
    m.unit,
    m.alarm_low,
    m.alarm_high,
    m.warn_low,
    m.warn_high,
    CASE
        WHEN s.value < COALESCE(m.alarm_low,  '-Inf'::float) THEN 'ALARM_LOW'
        WHEN s.value > COALESCE(m.alarm_high, '+Inf'::float) THEN 'ALARM_HIGH'
        WHEN s.value < COALESCE(m.warn_low,   '-Inf'::float) THEN 'WARN_LOW'
        WHEN s.value > COALESCE(m.warn_high,  '+Inf'::float) THEN 'WARN_HIGH'
        ELSE 'OK'
    END AS alarm_state,
    (s.quality & 4)  != 0 AS substituted,
    (s.quality & 8)  != 0 AS not_topical,
    (s.quality & 16) != 0 AS invalid
FROM   signals s
LEFT   JOIN signal_meta m USING (ioa)
ORDER  BY s.ioa, s.ts DESC;

CREATE OR REPLACE VIEW v_active_alarms AS
SELECT * FROM v_current_signals
WHERE  alarm_state NOT IN ('OK') AND invalid = FALSE
ORDER  BY ts DESC;

CREATE OR REPLACE VIEW v_dlq_open AS
SELECT id, backend, reason, retries, point_count, created_at,
       NOW() - created_at AS age
FROM   dlq_entries
WHERE  replayed = FALSE
ORDER  BY created_at DESC;

COMMENT ON TABLE signal_meta     IS 'Per-IOA configuration: names, units, alarm thresholds';
COMMENT ON TABLE dlq_entries     IS 'SQL mirror of Go BoltDB DLQ for reporting';
COMMENT ON TABLE pipeline_health IS 'Periodic write pipeline health snapshots';
COMMENT ON VIEW  v_current_signals IS 'Latest value per IOA with alarm state; UI live panel';
COMMENT ON VIEW  v_active_alarms   IS 'IOAs currently in alarm or warning state';
COMMENT ON VIEW  v_dlq_open        IS 'Unresolved DLQ entries for operator review';
