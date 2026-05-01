DROP VIEW  IF EXISTS v_dlq_open        CASCADE;
DROP VIEW  IF EXISTS v_active_alarms   CASCADE;
DROP VIEW  IF EXISTS v_current_signals CASCADE;
DROP TABLE IF EXISTS pipeline_health   CASCADE;
DROP TABLE IF EXISTS dlq_entries       CASCADE;
DROP TABLE IF EXISTS signal_meta       CASCADE;
DROP FUNCTION IF EXISTS fn_set_updated_at CASCADE;
