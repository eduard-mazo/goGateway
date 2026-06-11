DROP INDEX IF EXISTS ssfv.idx_equipo_activo;
DROP INDEX IF EXISTS ssfv.idx_planta_activa;
ALTER TABLE ssfv.tbl_equipo DROP COLUMN IF EXISTS fecha_baja;
ALTER TABLE ssfv.tbl_planta DROP COLUMN IF EXISTS fecha_baja;
