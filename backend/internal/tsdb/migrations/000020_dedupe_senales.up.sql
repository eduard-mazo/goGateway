-- =============================================================================
-- MIGRATION 020 — Deduplicate tbl_senales by codigo_senal (leaf) + enforce it.
--
-- Root cause of the production duplicates:
--   * The catalog señal is identified by the LEAF attribute (contract v3): a
--     metric a/b/c/signal stores codigo_senal = "signal"; the folder path "a/b/c"
--     lives in tbl_senales_x_equipo.nombre_instancia, NOT here.
--   * But the table's uniqueness was UNIQUE(codigo_senal, tipovar_id), so the
--     same code spawned a new row whenever (a) a re-approval picked a different
--     fallback tipo_variable, or (b) the code format changed across versions —
--     early rows stored the full metric path ("CPU/Usage_pct", "PLC/PRESION")
--     where later rows store the leaf ("Usage_pct", "PRESION").
--
-- This migration: normalize every codigo_senal to its leaf, collapse each leaf
-- group onto a single surviving señal (preferring a row already in leaf form,
-- else lowest senal_id), repoint/merge its bindings + tipo-template links and
-- their history, drop the dead rows, then replace the composite uniqueness with
-- UNIQUE(codigo_senal). Idempotent: a clean catalog re-runs as a no-op.
--
-- NOTE: this rewrites catalog rows and re-points time-series ownership — take a
-- database backup before first deploy.
-- =============================================================================

DO $$
DECLARE
    g         RECORD;  -- one leaf-code group
    dup       INT;     -- a non-surviving señal in the group
    b         RECORD;  -- a binding of the duplicate señal
    canonBind INT;     -- the surviving binding it collides with (if any)
BEGIN
    FOR g IN
        SELECT leaf,
               (array_agg(senal_id ORDER BY (codigo_senal = leaf) DESC, senal_id ASC))[1] AS canon
        FROM (
            SELECT senal_id, codigo_senal,
                   reverse(split_part(reverse(codigo_senal), '/', 1)) AS leaf
            FROM ssfv.tbl_senales
        ) s
        GROUP BY leaf
        HAVING count(*) > 1 OR bool_or(codigo_senal <> leaf)
    LOOP
        FOR dup IN
            SELECT senal_id FROM ssfv.tbl_senales
            WHERE reverse(split_part(reverse(codigo_senal), '/', 1)) = g.leaf
              AND senal_id <> g.canon
        LOOP
            -- Move each binding of the duplicate señal onto the survivor.
            FOR b IN SELECT * FROM ssfv.tbl_senales_x_equipo WHERE senal_id = dup
            LOOP
                SELECT equisenal_id INTO canonBind
                FROM ssfv.tbl_senales_x_equipo
                WHERE senal_id = g.canon AND equipo_id = b.equipo_id
                  AND nombre_instancia = b.nombre_instancia
                  AND indice_canal IS NOT DISTINCT FROM b.indice_canal;

                IF canonBind IS NULL THEN
                    -- No equivalent binding on the survivor: just re-point it.
                    UPDATE ssfv.tbl_senales_x_equipo SET senal_id = g.canon
                    WHERE equisenal_id = b.equisenal_id;
                ELSE
                    -- A duplicate binding to the same equipo+instancia exists:
                    -- fold this one's history into it, then drop the duplicate.
                    INSERT INTO ssfv.tbl_valores (timestamp_utc, equisenal_id, valor, calidad)
                    SELECT timestamp_utc, canonBind, valor, calidad
                    FROM ssfv.tbl_valores WHERE equisenal_id = b.equisenal_id
                    ON CONFLICT (timestamp_utc, equisenal_id) DO NOTHING;
                    DELETE FROM ssfv.tbl_valores WHERE equisenal_id = b.equisenal_id;
                    UPDATE ssfv.tbl_alarmas SET equisenal_id = canonBind
                    WHERE equisenal_id = b.equisenal_id;
                    DELETE FROM ssfv.tbl_senales_x_equipo WHERE equisenal_id = b.equisenal_id;
                END IF;
            END LOOP;

            -- Re-point tipo-template links (skip ones the survivor already has).
            UPDATE public.tbl_senales_x_tipo_equipo t SET senal_id = g.canon
            WHERE t.senal_id = dup
              AND NOT EXISTS (SELECT 1 FROM public.tbl_senales_x_tipo_equipo t2
                              WHERE t2.senal_id = g.canon AND t2.tipo_id = t.tipo_id);
            DELETE FROM public.tbl_senales_x_tipo_equipo WHERE senal_id = dup;

            DELETE FROM ssfv.tbl_senales WHERE senal_id = dup;
        END LOOP;

        -- Normalize the survivor's code to the leaf attribute.
        UPDATE ssfv.tbl_senales SET codigo_senal = g.leaf
        WHERE senal_id = g.canon AND codigo_senal <> g.leaf;
    END LOOP;
END $$;

-- Replace UNIQUE(codigo_senal, tipovar_id) with codigo_senal alone.
ALTER TABLE ssfv.tbl_senales DROP CONSTRAINT IF EXISTS uq_senal_codigo_tipvar;
CREATE UNIQUE INDEX IF NOT EXISTS uq_senal_codigo ON ssfv.tbl_senales (codigo_senal);
