-- =============================================================================
-- MIGRATION 010 — Move gateway extension tables out of ssfv schema.
--
-- signals_raw and tbl_senales_x_tipo_equipo are gateway-internal tables,
-- not part of the official ssfv schema. This migration ensures they live in
-- public regardless of whether migration 007/008 ran before the schema was
-- corrected (idempotent via IF NOT EXISTS / ON CONFLICT DO NOTHING).
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 1. public.signals_raw  (fallback for SSFV points without a catalog match)
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS public.signals_raw (
    ts          TIMESTAMPTZ  NOT NULL,
    signal_path TEXT         NOT NULL,
    signal      TEXT,
    value       DOUBLE PRECISION,
    quality     SMALLINT     DEFAULT 0,
    tags        JSONB        DEFAULT '{}'::jsonb,
    PRIMARY KEY (ts, signal_path)
);

SELECT create_hypertable(
    'public.signals_raw',
    'ts',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists       => TRUE
);

CREATE INDEX IF NOT EXISTS idx_signals_raw_path_time
    ON public.signals_raw(signal_path, ts DESC);

-- Migrate existing data from ssfv.signals_raw if present, then drop it.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'ssfv' AND table_name = 'signals_raw'
    ) THEN
        INSERT INTO public.signals_raw (ts, signal_path, signal, value, quality, tags)
        SELECT ts, signal_path, signal, value, quality, tags
        FROM ssfv.signals_raw
        ON CONFLICT (ts, signal_path) DO NOTHING;

        DROP TABLE ssfv.signals_raw CASCADE;
    END IF;
END $$;

-- ---------------------------------------------------------------------------
-- 2. public.tbl_senales_x_tipo_equipo  (signal templates per equipment type)
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS public.tbl_senales_x_tipo_equipo (
    senaltipo_id SERIAL   PRIMARY KEY,
    senal_id     INT      NOT NULL REFERENCES ssfv.tbl_senales(senal_id),
    tipo_id      INT      NOT NULL REFERENCES ssfv.tbl_tipo_equipo(tipo_id),
    num_canales  SMALLINT NOT NULL DEFAULT 1,
    CONSTRAINT uq_senal_tipo_equipo UNIQUE (senal_id, tipo_id)
);

-- Migrate from ssfv.tbl_senales_x_tipo_equipo if present, then drop it.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'ssfv' AND table_name = 'tbl_senales_x_tipo_equipo'
    ) THEN
        INSERT INTO public.tbl_senales_x_tipo_equipo (senal_id, tipo_id, num_canales)
        SELECT senal_id, tipo_id, num_canales
        FROM ssfv.tbl_senales_x_tipo_equipo
        ON CONFLICT (senal_id, tipo_id) DO NOTHING;

        DROP TABLE ssfv.tbl_senales_x_tipo_equipo CASCADE;
    END IF;
END $$;
