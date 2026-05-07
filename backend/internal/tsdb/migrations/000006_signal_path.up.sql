-- Rename signals: measurement → signal, ioa → signal_path.
-- Wrapped in DO blocks so the migration is idempotent (safe to re-run
-- after a partial failure without the "column does not exist" error).

DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'signals' AND column_name = 'measurement'
    ) THEN
        ALTER TABLE signals RENAME COLUMN measurement TO signal;
    END IF;
END $$;

DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'signals' AND column_name = 'ioa'
    ) THEN
        ALTER TABLE signals RENAME COLUMN ioa TO signal_path;
    END IF;
END $$;

DROP INDEX IF EXISTS uq_signals_ts_meas_ioa;
DROP INDEX IF EXISTS idx_signals_ioa_ts;
DROP INDEX IF EXISTS idx_signals_meas_ts;
DROP INDEX IF EXISTS idx_signals_good_quality;

CREATE UNIQUE INDEX IF NOT EXISTS uq_signals_ts_signal_path ON signals (ts, signal_path);
CREATE INDEX        IF NOT EXISTS idx_signals_path_ts        ON signals (signal_path, ts DESC);
CREATE INDEX        IF NOT EXISTS idx_signals_signal_ts      ON signals (signal, ts DESC);
CREATE INDEX        IF NOT EXISTS idx_signals_good_quality   ON signals (signal_path, ts DESC) WHERE quality = 0;

DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'signal_meta' AND column_name = 'ioa'
    ) THEN
        ALTER TABLE signal_meta RENAME COLUMN ioa TO signal_path;
    END IF;
END $$;

DROP VIEW IF EXISTS v_active_alarms;
DROP VIEW IF EXISTS v_current_signals;

CREATE OR REPLACE VIEW v_current_signals AS
SELECT DISTINCT ON (s.signal_path)
    s.ts,
    s.signal,
    s.signal_path,
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
LEFT   JOIN signal_meta m USING (signal_path)
ORDER  BY s.signal_path, s.ts DESC;

CREATE OR REPLACE VIEW v_active_alarms AS
SELECT * FROM v_current_signals
WHERE  alarm_state NOT IN ('OK') AND invalid = FALSE
ORDER  BY ts DESC;

COMMENT ON COLUMN signals.signal      IS 'Last segment of the signal path, e.g. Temperature';
COMMENT ON COLUMN signals.signal_path IS 'Full hierarchical path: business/company/B1/.../signal';
