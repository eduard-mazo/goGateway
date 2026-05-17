SELECT remove_compression_policy('signals', if_not_exists => TRUE);
ALTER TABLE signals SET (timescaledb.compress = FALSE);
