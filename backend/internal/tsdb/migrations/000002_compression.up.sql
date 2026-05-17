-- Segment by measurement+ioa so compressed chunks have homogeneous data.
-- TimescaleDB uses dictionary encoding per segment → best compression ratio.
ALTER TABLE signals SET (
    timescaledb.compress           = TRUE,
    timescaledb.compress_segmentby = 'measurement, ioa',
    timescaledb.compress_orderby   = 'ts DESC'
);

-- Auto-compress chunks older than 7 days.
-- 7 days at 2000/s ≈ 1.2B rows → ~12GB compressed (from ~120GB uncompressed).
SELECT add_compression_policy(
    'signals',
    compress_after => INTERVAL '7 days',
    if_not_exists  => TRUE
);
