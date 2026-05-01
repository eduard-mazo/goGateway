-- 1-minute rollup — UI charts hit this for windows < 12 hours
CREATE MATERIALIZED VIEW IF NOT EXISTS signals_1m
WITH (timescaledb.continuous, timescaledb.materialized_only = FALSE)
AS
SELECT
    time_bucket('1 minute', ts)                              AS bucket,
    measurement,
    ioa,
    AVG(value)                                               AS avg_value,
    MIN(value)                                               AS min_value,
    MAX(value)                                               AS max_value,
    LAST(value, ts)                                          AS last_value,
    FIRST(value, ts)                                         AS first_value,
    COUNT(*)                                                 AS sample_count,
    SUM(CASE WHEN quality = 0 THEN 0 ELSE 1 END)             AS bad_quality_count,
    MIN(tags->>'source')                                     AS source
FROM signals
GROUP BY bucket, measurement, ioa
WITH NO DATA;

SELECT add_continuous_aggregate_policy(
    'signals_1m',
    start_offset      => INTERVAL '10 minutes',
    end_offset        => INTERVAL '30 seconds',
    schedule_interval => INTERVAL '30 seconds',
    if_not_exists     => TRUE
);

CREATE INDEX IF NOT EXISTS idx_signals_1m_ioa_bucket   ON signals_1m (ioa, bucket DESC);
CREATE INDEX IF NOT EXISTS idx_signals_1m_meas_bucket  ON signals_1m (measurement, bucket DESC);

-- 1-hour rollup — cascades from 1m, not raw
CREATE MATERIALIZED VIEW IF NOT EXISTS signals_1h
WITH (timescaledb.continuous, timescaledb.materialized_only = FALSE)
AS
SELECT
    time_bucket('1 hour', bucket)                            AS bucket,
    measurement,
    ioa,
    AVG(avg_value)                                           AS avg_value,
    MIN(min_value)                                           AS min_value,
    MAX(max_value)                                           AS max_value,
    LAST(last_value, bucket)                                 AS last_value,
    FIRST(first_value, bucket)                               AS first_value,
    SUM(sample_count)                                        AS sample_count,
    SUM(bad_quality_count)                                   AS bad_quality_count,
    MIN(source)                                              AS source
FROM signals_1m
GROUP BY time_bucket('1 hour', bucket), measurement, ioa
WITH NO DATA;

SELECT add_continuous_aggregate_policy(
    'signals_1h',
    start_offset      => INTERVAL '3 hours',
    end_offset        => INTERVAL '1 minute',
    schedule_interval => INTERVAL '1 minute',
    if_not_exists     => TRUE
);

CREATE INDEX IF NOT EXISTS idx_signals_1h_ioa_bucket ON signals_1h (ioa, bucket DESC);

-- 1-day rollup — cascades from 1h
CREATE MATERIALIZED VIEW IF NOT EXISTS signals_1d
WITH (timescaledb.continuous, timescaledb.materialized_only = FALSE)
AS
SELECT
    time_bucket('1 day', bucket)                             AS bucket,
    measurement,
    ioa,
    AVG(avg_value)                                           AS avg_value,
    MIN(min_value)                                           AS min_value,
    MAX(max_value)                                           AS max_value,
    LAST(last_value, bucket)                                 AS last_value,
    FIRST(first_value, bucket)                               AS first_value,
    SUM(sample_count)                                        AS sample_count,
    SUM(bad_quality_count)                                   AS bad_quality_count,
    MIN(source)                                              AS source
FROM signals_1h
GROUP BY time_bucket('1 day', bucket), measurement, ioa
WITH NO DATA;

SELECT add_continuous_aggregate_policy(
    'signals_1d',
    start_offset      => INTERVAL '3 days',
    end_offset        => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists     => TRUE
);

CREATE INDEX IF NOT EXISTS idx_signals_1d_ioa_bucket ON signals_1d (ioa, bucket DESC);

-- Compress aggregate chunks to save space
ALTER MATERIALIZED VIEW signals_1m SET (timescaledb.compress = TRUE);
ALTER MATERIALIZED VIEW signals_1h SET (timescaledb.compress = TRUE);
ALTER MATERIALIZED VIEW signals_1d SET (timescaledb.compress = TRUE);

SELECT add_compression_policy('signals_1m', compress_after => INTERVAL '1 day',   if_not_exists => TRUE);
SELECT add_compression_policy('signals_1h', compress_after => INTERVAL '7 days',  if_not_exists => TRUE);
SELECT add_compression_policy('signals_1d', compress_after => INTERVAL '30 days', if_not_exists => TRUE);
