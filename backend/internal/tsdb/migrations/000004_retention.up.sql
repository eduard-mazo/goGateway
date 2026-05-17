-- Raw data: 90 days  (~30GB compressed at 2000 signals/sec)
SELECT add_retention_policy('signals',    drop_after => INTERVAL '90 days',   if_not_exists => TRUE);
-- 1-min aggregate: 1 year
SELECT add_retention_policy('signals_1m', drop_after => INTERVAL '365 days',  if_not_exists => TRUE);
-- 1-hour aggregate: 5 years
SELECT add_retention_policy('signals_1h', drop_after => INTERVAL '1825 days', if_not_exists => TRUE);
-- 1-day aggregate: no retention (keep forever)

-- Storage estimate helper
CREATE OR REPLACE VIEW v_storage_estimate AS
SELECT
    hypertable_name,
    pg_size_pretty(hypertable_size(format('%I', hypertable_name)::regclass)) AS total_size,
    (SELECT count(*)
     FROM   timescaledb_information.chunks c
     WHERE  c.hypertable_name = i.hypertable_name)                           AS chunks
FROM timescaledb_information.hypertables i;
