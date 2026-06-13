-- =============================================================================
-- MIGRATION 018 — Widen nombre_instancia for deep FIWARE/UNS folder paths.
--
-- nombre_instancia holds the folder path between the entity and the leaf
-- attribute (contract v3 §5.1): for a metric a/b/c/signal it stores "a/b/c".
-- VARCHAR(30) only fits one or two shallow segments — a realistic deep path
-- (e.g. "Substation01/Feeder1/PhaseC") already sits at 27 chars and a third
-- level overflows, failing approval. The UI now renders these as a folder tree,
-- so arbitrary depth must be storable. Widen to VARCHAR(120).
--
-- ALTER COLUMN TYPE is blocked while a view references the column (SQLSTATE
-- 0A000), so drop the two dependent ssfv views, alter, then recreate them
-- verbatim from migration 007 (same drop/recreate pattern as migration 006).
-- The composite unique index uq_sxe_senal_equipo_instancia is unaffected.
-- =============================================================================

DROP VIEW IF EXISTS ssfv.v_alarmas_activas;
DROP VIEW IF EXISTS ssfv.v_senales_contexto;

ALTER TABLE ssfv.tbl_senales_x_equipo
    ALTER COLUMN nombre_instancia TYPE VARCHAR(120);

CREATE OR REPLACE VIEW ssfv.v_senales_contexto AS
SELECT
    sxe.equisenal_id,
    sxe.nombre_instancia,
    sxe.indice_canal,
    sxe.activo                  AS senal_activa,
    s.senal_id,
    s.nombre                    AS senal_nombre,
    s.codigo_senal,
    s.tipo_valor,
    s.es_indexada,
    tv.nombre                   AS tipo_variable,
    u.simbolo                   AS unidad,
    u.magnitud,
    e.equipo_id,
    e.nombre_equipo,
    e.nombre_topic,
    e.fabricante,
    e.modelo,
    te.nombre                   AS tipo_equipo,
    p.planta_id,
    p.nombre                    AS planta_nombre,
    p.broker_base
FROM ssfv.tbl_senales_x_equipo  sxe
JOIN ssfv.tbl_senales            s   ON s.senal_id    = sxe.senal_id
JOIN ssfv.tbl_tipo_variable      tv  ON tv.tipovar_id = s.tipovar_id
JOIN ssfv.tbl_unidades           u   ON u.unidad_id   = s.unidad_id
JOIN ssfv.tbl_equipo             e   ON e.equipo_id   = sxe.equipo_id
JOIN ssfv.tbl_tipo_equipo        te  ON te.tipo_id    = e.tipo_id
JOIN ssfv.tbl_planta             p   ON p.planta_id   = e.planta_id;

CREATE OR REPLACE VIEW ssfv.v_alarmas_activas AS
SELECT
    a.alarma_id,
    a.ts_inicio,
    a.tipo_alarma,
    a.severidad,
    a.descripcion,
    sc.planta_nombre,
    sc.nombre_equipo,
    sc.tipo_equipo,
    sc.nombre_instancia,
    sc.tipo_variable
FROM ssfv.tbl_alarmas        a
JOIN ssfv.v_senales_contexto sc ON sc.equisenal_id = a.equisenal_id
WHERE a.activa = TRUE
ORDER BY a.severidad DESC, a.ts_inicio DESC;
