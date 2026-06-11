-- =============================================================================
-- MIGRATION 016 — Uniqueness for the C1 composite identity per equipo.
--
-- Contract v3 (sparkplug-contract.md §5.1): codigo_senal is the LEAF attribute
-- and nombre_instancia is the folder/channel path, so the same señal (e.g.
-- "Usage_pct") legitimately binds to one equipo several times with different
-- instancias ("CPU", "Memory"). Those rows carry indice_canal NULL, and the
-- existing uq_senal_equipo_canal UNIQUE (senal_id, equipo_id, indice_canal)
-- does not deduplicate them (NULLs compare distinct in Postgres) — until now
-- only the approve handler's NOT EXISTS guard prevented duplicates.
--
-- This partial unique index makes (senal_id, equipo_id, nombre_instancia) the
-- DB-level identity for non-channel-indexed bindings.
-- =============================================================================

CREATE UNIQUE INDEX IF NOT EXISTS uq_sxe_senal_equipo_instancia
    ON ssfv.tbl_senales_x_equipo (senal_id, equipo_id, nombre_instancia)
    WHERE indice_canal IS NULL;
