-- =============================================================================
-- MIGRATION 012 — Retire the separate host-telemetry storage.
--
-- Edge-node System/* metrics are now persisted through the standard catalog
-- path into ssfv.tbl_valores: the edge node is registered as a host "station"
-- (equipo, nombre_topic = group/node) and each System metric as a catalog
-- signal. The parallel host-metrics table, its latest-value view, and the
-- network-interface allowlist are obsolete and removed here.
--
-- Idempotent (DROP … IF EXISTS). Dropping the hypertable removes its
-- compression/retention policies, indexes and chunks automatically.
-- =============================================================================

DROP VIEW  IF EXISTS ssfv.v_host_ultimas;
DROP TABLE IF EXISTS ssfv.tbl_metricas_host CASCADE;
DROP TABLE IF EXISTS ssfv.cfg_host_iface    CASCADE;
