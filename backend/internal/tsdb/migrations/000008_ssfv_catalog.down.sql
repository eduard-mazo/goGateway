-- Migration 008 rollback
DROP MATERIALIZED VIEW IF EXISTS ssfv.mv_valores_diario CASCADE;
DROP MATERIALIZED VIEW IF EXISTS ssfv.mv_valores_15min  CASCADE;
DROP TABLE IF EXISTS public.tbl_senales_x_tipo_equipo CASCADE;
