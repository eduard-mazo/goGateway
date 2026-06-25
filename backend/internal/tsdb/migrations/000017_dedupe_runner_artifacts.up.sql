-- =============================================================================
-- MIGRATION 017 — Remove duplicate pipeline objects + calm cagg refresh churn.
--
-- 1) The pre-fix migration runner tracked applied versions in an UNQUALIFIED
--    schema_migrations table. The SSFV pool runs with search_path =
--    ssfv, public, so once the ssfv schema existed every restart created a
--    fresh empty ssfv.schema_migrations, saw nothing applied, and re-ran all
--    migrations — duplicating the generic pipeline (signals hypertable,
--    signals_1m/1h/1d caggs, signal_meta, dlq_entries, pipeline_health) and
--    their refresh/retention/columnstore jobs inside ssfv. Symptom: paired
--    "inserted 0 row(s) into materialization table _materialized_hypertable_N"
--    postgres log lines every refresh cycle, twice over.
--
--    Gateway-internal tables belong in public (see migration 010); drop the
--    accidental ssfv copies. Their policies/jobs/chunks go with them.
--    Idempotent: a DB that never double-ran has none of these.
--
-- 2) Relax the generic caggs' refresh cadence (30 s / 1 min are churn —
--    TimescaleDB real-time aggregation already folds the not-yet-materialized
--    tail into query results, so the schedule affects background work, not
--    correctness). On SSFV deployments `signals` is empty and these jobs only
--    produce 0-row materialization log lines.
-- =============================================================================

-- Caggs first (1d reads 1h reads 1m), then their source hypertable.
DROP MATERIALIZED VIEW IF EXISTS ssfv.signals_1d;
DROP MATERIALIZED VIEW IF EXISTS ssfv.signals_1h;
DROP MATERIALIZED VIEW IF EXISTS ssfv.signals_1m;
DROP TABLE IF EXISTS ssfv.signals         CASCADE;
DROP TABLE IF EXISTS ssfv.signal_meta     CASCADE;
DROP TABLE IF EXISTS ssfv.dlq_entries     CASCADE;
DROP TABLE IF EXISTS ssfv.pipeline_health CASCADE;
DROP TABLE IF EXISTS ssfv.schema_migrations;

-- Slow the public caggs' refresh schedule: 1m rollup every 5 min, 1h every
-- 30 min (1d already runs hourly). alter_job is looked up by materialization
-- hypertable so this works regardless of job ids.
DO $$
DECLARE
    j RECORD;
BEGIN
    FOR j IN
        SELECT js.job_id,
               CASE ca.view_name
                   WHEN 'signals_1m' THEN INTERVAL '5 minutes'
                   WHEN 'signals_1h' THEN INTERVAL '30 minutes'
               END AS ival
        FROM timescaledb_information.jobs js
        JOIN timescaledb_information.continuous_aggregates ca
          ON  ca.view_name   = js.hypertable_name
          AND ca.view_schema = js.hypertable_schema
        WHERE js.application_name LIKE 'Refresh Continuous Aggregate%'
          AND ca.view_schema = 'public'
          AND ca.view_name IN ('signals_1m', 'signals_1h')
    LOOP
        PERFORM alter_job(j.job_id, schedule_interval => j.ival);
    END LOOP;
END $$;
