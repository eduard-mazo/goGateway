-- =============================================================================
-- MIGRATION 013 — C1: normalize flat instances to 'default'.
--
-- The catalog match is now the composite (entity, codigo_senal, nombre_instancia)
-- — the FIWARE Entity → Attribute → channel model. Flat signals historically
-- stored nombre_instancia = codigo_senal; under C1 a flat signal has the bare
-- attribute in codigo_senal and the sentinel 'default' as its instance.
--
-- Only non-indexed rows whose instance currently mirrors the code are touched;
-- indexed channels (indice_canal IS NOT NULL, e.g. solar IDC_1/IDC_2) keep the
-- channel name as their instance. Validated collision-free against a copy of the
-- production catalog (63 flat normalized, 15 indexed preserved, 0 collisions).
-- Idempotent: re-running matches nothing once normalized.
-- =============================================================================

UPDATE ssfv.tbl_senales_x_equipo sxe
SET    nombre_instancia = 'default'
FROM   ssfv.tbl_senales s
WHERE  s.senal_id = sxe.senal_id
  AND  sxe.indice_canal IS NULL
  AND  sxe.nombre_instancia = s.codigo_senal;
