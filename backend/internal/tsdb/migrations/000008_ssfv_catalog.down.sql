-- Migration 008 rollback
DROP MATERIALIZED VIEW IF EXISTS ssfv.mv_valores_diario CASCADE;
DROP MATERIALIZED VIEW IF EXISTS ssfv.mv_valores_15min  CASCADE;
DROP TABLE IF EXISTS ssfv."Tbl_Senales_x_Tipo_Equipo" CASCADE;
DO $$ BEGIN
    ALTER TABLE ssfv."Tbl_Senales" DROP COLUMN "Es_Alarma";
EXCEPTION WHEN undefined_column THEN NULL;
END $$;
