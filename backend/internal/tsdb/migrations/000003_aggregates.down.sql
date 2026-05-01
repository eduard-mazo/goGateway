SELECT remove_compression_policy('signals_1d', if_not_exists => TRUE);
SELECT remove_compression_policy('signals_1h', if_not_exists => TRUE);
SELECT remove_compression_policy('signals_1m', if_not_exists => TRUE);
SELECT remove_continuous_aggregate_policy('signals_1d', if_not_exists => TRUE);
SELECT remove_continuous_aggregate_policy('signals_1h', if_not_exists => TRUE);
SELECT remove_continuous_aggregate_policy('signals_1m', if_not_exists => TRUE);
DROP MATERIALIZED VIEW IF EXISTS signals_1d CASCADE;
DROP MATERIALIZED VIEW IF EXISTS signals_1h CASCADE;
DROP MATERIALIZED VIEW IF EXISTS signals_1m CASCADE;
