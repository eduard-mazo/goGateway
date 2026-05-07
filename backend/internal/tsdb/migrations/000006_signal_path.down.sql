DROP VIEW IF EXISTS v_active_alarms;
DROP VIEW IF EXISTS v_current_signals;

ALTER TABLE signal_meta RENAME COLUMN signal_path TO ioa;

DROP INDEX IF EXISTS uq_signals_ts_signal_path;
DROP INDEX IF EXISTS idx_signals_path_ts;
DROP INDEX IF EXISTS idx_signals_signal_ts;
DROP INDEX IF EXISTS idx_signals_good_quality;

ALTER TABLE signals RENAME COLUMN signal_path TO ioa;
ALTER TABLE signals RENAME COLUMN signal      TO measurement;

CREATE UNIQUE INDEX uq_signals_ts_meas_ioa  ON signals (ts, measurement, ioa);
CREATE INDEX        idx_signals_ioa_ts       ON signals (ioa, ts DESC);
CREATE INDEX        idx_signals_meas_ts      ON signals (measurement, ts DESC);
CREATE INDEX        idx_signals_good_quality ON signals (ioa, ts DESC) WHERE quality = 0;

CREATE OR REPLACE VIEW v_current_signals AS
SELECT DISTINCT ON (s.ioa)
    s.ts, s.measurement, s.ioa, s.value, s.quality, s.tags,
    m.display_name, m.unit, m.alarm_low, m.alarm_high, m.warn_low, m.warn_high,
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
