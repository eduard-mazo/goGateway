-- Migration 011 rollback — drop host telemetry objects.
DROP VIEW  IF EXISTS ssfv.v_host_ultimas;
DROP TABLE IF EXISTS ssfv.tbl_metricas_host;
DROP TABLE IF EXISTS ssfv.cfg_host_iface;
