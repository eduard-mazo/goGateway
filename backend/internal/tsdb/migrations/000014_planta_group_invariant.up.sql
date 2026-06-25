-- =============================================================================
-- MIGRATION 014 — Planta = group; enforce the topic-prefix invariant.
--
-- Contract (sparkplug-contract.md §1.1): a Planta maps to at most one group_id
-- (tbl_planta.broker_base = the group, one segment); every equipo's nombre_topic
-- (group/node[/device]) must start with its planta's broker_base.
--
-- This trigger enforces the prefix at the DB level so a Sparkplug entity can
-- never be filed under a planta of a different group. Existing rows are not
-- rewritten (the production planta/equipo tables are empty; the seeded Sede30
-- rows already satisfy the prefix and are inserted before this trigger exists).
-- =============================================================================

CREATE OR REPLACE FUNCTION ssfv.enforce_equipo_topic_prefix()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE base TEXT;
BEGIN
    SELECT broker_base INTO base FROM ssfv.tbl_planta WHERE planta_id = NEW.planta_id;
    IF base IS NOT NULL
       AND NEW.nombre_topic <> base
       AND NEW.nombre_topic NOT LIKE base || '/%' THEN
        RAISE EXCEPTION
            'equipo nombre_topic "%" must start with planta broker_base (group) "%"',
            NEW.nombre_topic, base
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_equipo_topic_prefix ON ssfv.tbl_equipo;
CREATE TRIGGER trg_equipo_topic_prefix
    BEFORE INSERT OR UPDATE ON ssfv.tbl_equipo
    FOR EACH ROW EXECUTE FUNCTION ssfv.enforce_equipo_topic_prefix();
