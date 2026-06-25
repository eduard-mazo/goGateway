-- =============================================================================
-- MIGRATION 015 — Soft-delete for plantas and equipos.
--
-- "Deleting" a planta/equipo from the UI used to cascade-DELETE every row in the
-- tbl_valores / tbl_alarmas hypertables for its señales. In production that is
-- millions of rows (10.6M for a single planta) and the request blew past its
-- 30s context deadline ("context deadline exceeded").
--
-- Instead we soft-delete: a non-NULL fecha_baja marks the row as deleted while
-- the historical time-series data is kept intact. The ingestion pipeline already
-- only processes estado = 1 rows, so the soft-delete handler also sets estado = 0
-- to stop new data; fecha_baja IS NOT NULL is the durable "deleted at" record.
-- =============================================================================

ALTER TABLE ssfv.tbl_planta ADD COLUMN IF NOT EXISTS fecha_baja TIMESTAMPTZ;
ALTER TABLE ssfv.tbl_equipo ADD COLUMN IF NOT EXISTS fecha_baja TIMESTAMPTZ;

-- Live-row lookups (the default list views) filter on fecha_baja IS NULL.
CREATE INDEX IF NOT EXISTS idx_planta_activa
    ON ssfv.tbl_planta(planta_id) WHERE fecha_baja IS NULL;
CREATE INDEX IF NOT EXISTS idx_equipo_activo
    ON ssfv.tbl_equipo(equipo_id) WHERE fecha_baja IS NULL;
