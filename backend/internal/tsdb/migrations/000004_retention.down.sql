SELECT remove_retention_policy('signals_1h', if_not_exists => TRUE);
SELECT remove_retention_policy('signals_1m', if_not_exists => TRUE);
SELECT remove_retention_policy('signals',    if_not_exists => TRUE);
DROP VIEW IF EXISTS v_storage_estimate;
