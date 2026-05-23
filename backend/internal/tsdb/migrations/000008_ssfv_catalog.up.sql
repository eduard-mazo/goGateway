-- =============================================================================
-- MIGRATION 008 — SSFV catalog: tipo_equipo↔señales junction, full signal
--                 catalog, continuous aggregates. All names lowercase.
-- Idempotent: every statement uses IF NOT EXISTS / ON CONFLICT / EXCEPTION guards.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 1. Junction table: tipo_equipo ↔ señales (drives auto-instantiation)
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS public.tbl_senales_x_tipo_equipo (
    senaltipo_id SERIAL   PRIMARY KEY,
    senal_id     INT      NOT NULL REFERENCES ssfv.tbl_senales(senal_id),
    tipo_id      INT      NOT NULL REFERENCES ssfv.tbl_tipo_equipo(tipo_id),
    num_canales  SMALLINT NOT NULL DEFAULT 1,
    CONSTRAINT uq_senal_tipo_equipo UNIQUE (senal_id, tipo_id)
);

-- ---------------------------------------------------------------------------
-- 2. Catálogo completo de señales SSFV (plain SQL — auto-commits as its own
--    statement; FK checks in step 3 see the committed rows)
-- ---------------------------------------------------------------------------
INSERT INTO ssfv.tbl_senales
    (tipovar_id, unidad_id, nombre, tipo_valor, codigo_senal, es_indexada, activo)
SELECT tv.tipovar_id, u.unidad_id, v.nombre, v.tipo_valor, v.codigo_senal,
       v.es_indexada, v.activo
FROM (VALUES
    ('Corriente AC',  'A',            'Corriente Fase A AC',         'Instantaneo', 'IA',     false, true),
    ('Corriente AC',  'A',            'Corriente Fase B AC',         'Instantaneo', 'IB',     false, true),
    ('Corriente AC',  'A',            'Corriente Fase C AC',         'Instantaneo', 'IC',     false, true),
    ('Voltage AC',    'V',            'Voltaje Línea AB',            'Instantaneo', 'UAB',    false, true),
    ('Voltage AC',    'V',            'Voltaje Línea BC',            'Instantaneo', 'UBC',    false, true),
    ('Voltage AC',    'V',            'Voltaje Línea CA',            'Instantaneo', 'UCA',    false, true),
    ('Potencia',      'kW',           'Potencia Activa',             'Instantaneo', 'AP',     false, true),
    ('Potencia',      'kVar',         'Potencia Reactiva',           'Instantaneo', 'RP',     false, true),
    ('Potencia',      'kVA',          'Potencia Aparente',           'Instantaneo', 'SP',     false, true),
    ('Proceso',       'Adimensional', 'Factor de Potencia',          'Instantaneo', 'FP',     false, true),
    ('Proceso',       '%',            'Eficiencia Inversor',         'Instantaneo', 'EF',     false, true),
    ('Proceso',       'Hz',           'Frecuencia Red',              'Instantaneo', 'FR',     false, true),
    ('Energía',       'kWh',          'Energía Acumulada',           'Acumulado',   'ET',     false, true),
    ('Potencia',      'kW',           'Potencia Entrada DC',         'Instantaneo', 'IP',     false, true),
    ('Temperatura',   '°C',           'Temperatura Inversor',        'Instantaneo', 'T',      false, true),
    ('Proceso',       'MΩ',           'Resistencia Aislamiento',     'Instantaneo', 'IR',     false, true),
    ('Estado',        'Adimensional', 'Estado Operación',            'Instantaneo', 'OS',     false, true),
    ('Estado',        'Adimensional', 'OS Fabricante',               'Instantaneo', 'OSV',    false, true),
    ('Corriente DC',  'A',            'Corriente DC String',         'Instantaneo', 'IDC_x',  true,  true),
    ('Voltage DC',    'V',            'Voltaje DC String',           'Instantaneo', 'VDC_x',  true,  true),
    ('Estado',        'Adimensional', 'Alarma Dispositivo EF',       'Instantaneo', 'EF_x',   true,  true),
    ('Estado',        'Adimensional', 'Alarma Fabricante EV',        'Instantaneo', 'EV_x',   true,  true),
    ('Alarma',        'Adimensional', 'Alarma Dispositivo AL',       'Instantaneo', 'AL_x',   true,  true),
    ('Alarma',        'Adimensional', 'Alarma Comunicación',         'Instantaneo', 'AL_COM', false, true),
    ('Irradiancia',   'W/m2',         'Irradiancia Principal',       'Instantaneo', 'RD',     false, true),
    ('Temperatura',   '°C',           'Temperatura Ambiente',        'Instantaneo', 'TA',     false, true),
    ('Temperatura',   '°C',           'Temperatura Panel',           'Instantaneo', 'TP',     false, true),
    ('Voltage AC',    'V',            'Voltaje Fase A',              'Instantaneo', 'UA',     false, true),
    ('Energía',       'kWh',          'Energía Activa Importada',    'Acumulado',   'API',    false, true),
    ('Energía',       'kWh',          'Energía Activa Exportada',    'Acumulado',   'AN',     false, true),
    ('Energía',       'kVarh',        'Energía Reactiva Importada',  'Acumulado',   'QPZ',    false, true),
    ('Energía',       'kVarh',        'Energía Reactiva Exportada',  'Acumulado',   'QN',     false, true)
) AS v(tipovar_nombre, unidad_simbolo, nombre, tipo_valor, codigo_senal, es_indexada, activo)
JOIN ssfv.tbl_tipo_variable tv ON tv.nombre  = v.tipovar_nombre
JOIN ssfv.tbl_unidades      u  ON u.simbolo  = v.unidad_simbolo
ON CONFLICT (codigo_senal, tipovar_id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3. Asignaciones señales ↔ tipo de equipo (plain SQL — each statement
--    auto-commits; FK on senal_id sees the committed rows from step 2)
-- ---------------------------------------------------------------------------

-- Junction: Inversor (IDC_x and VDC_x use num_canales = 3)
INSERT INTO public.tbl_senales_x_tipo_equipo (senal_id, tipo_id, num_canales)
SELECT s.senal_id, te.tipo_id, n.num_canales::smallint
FROM (VALUES
    ('IA', 1), ('IB', 1), ('IC', 1),
    ('UAB', 1), ('UBC', 1), ('UCA', 1),
    ('AP', 1), ('RP', 1), ('SP', 1),
    ('FP', 1), ('EF', 1), ('FR', 1),
    ('ET', 1), ('IP', 1), ('T', 1),
    ('IR', 1), ('OS', 1), ('OSV', 1),
    ('IDC_x', 3), ('VDC_x', 3),
    ('EF_x', 1), ('EV_x', 1),
    ('AL_x', 1), ('AL_COM', 1)
) AS n(codigo_senal, num_canales)
JOIN ssfv.tbl_senales s ON s.codigo_senal = n.codigo_senal
CROSS JOIN (SELECT tipo_id FROM ssfv.tbl_tipo_equipo WHERE nombre = 'Inversor') te
ON CONFLICT (senal_id, tipo_id) DO NOTHING;

-- Junction: Medidor
INSERT INTO public.tbl_senales_x_tipo_equipo (senal_id, tipo_id, num_canales)
SELECT s.senal_id, te.tipo_id, 1::smallint
FROM ssfv.tbl_senales s
CROSS JOIN (SELECT tipo_id FROM ssfv.tbl_tipo_equipo WHERE nombre = 'Medidor') te
WHERE s.codigo_senal IN ('UA','UAB','UBC','UCA','IA','IB','IC','AP','RP','SP','FP','ET','API','AN','QPZ','QN','AL_COM')
ON CONFLICT (senal_id, tipo_id) DO NOTHING;

-- Junction: Estación Meteorológica
INSERT INTO public.tbl_senales_x_tipo_equipo (senal_id, tipo_id, num_canales)
SELECT s.senal_id, te.tipo_id, 1::smallint
FROM ssfv.tbl_senales s
CROSS JOIN (SELECT tipo_id FROM ssfv.tbl_tipo_equipo WHERE nombre = 'Estación Meteorológica') te
WHERE s.codigo_senal IN ('RD','TA','TP','AL_COM')
ON CONFLICT (senal_id, tipo_id) DO NOTHING;

-- Junction: Frontera Comercial
INSERT INTO public.tbl_senales_x_tipo_equipo (senal_id, tipo_id, num_canales)
SELECT s.senal_id, te.tipo_id, 1::smallint
FROM ssfv.tbl_senales s
CROSS JOIN (SELECT tipo_id FROM ssfv.tbl_tipo_equipo WHERE nombre = 'Frontera Comercial') te
WHERE s.codigo_senal IN ('API','AN','QPZ','QN','IA','UAB','AL_COM')
ON CONFLICT (senal_id, tipo_id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 4. Agregados continuos (15 min y diario)
-- ---------------------------------------------------------------------------

DO $$
BEGIN
    CREATE MATERIALIZED VIEW ssfv.mv_valores_15min
    WITH (timescaledb.continuous) AS
    SELECT
        time_bucket('15 minutes', timestamp_utc) AS bucket,
        equisenal_id,
        AVG(valor)  AS avg_val,
        MIN(valor)  AS min_val,
        MAX(valor)  AS max_val,
        COUNT(*)    AS n_total,
        COUNT(*) FILTER (WHERE calidad = 'Buena') AS n_validas
    FROM ssfv.tbl_valores
    GROUP BY bucket, equisenal_id;
EXCEPTION WHEN OTHERS THEN NULL;
END $$;

DO $$
BEGIN
    PERFORM add_continuous_aggregate_policy('ssfv.mv_valores_15min',
        start_offset      => INTERVAL '2 hours',
        end_offset        => INTERVAL '15 minutes',
        schedule_interval => INTERVAL '15 minutes');
EXCEPTION WHEN OTHERS THEN NULL;
END $$;

DO $$
BEGIN
    CREATE MATERIALIZED VIEW ssfv.mv_valores_diario
    WITH (timescaledb.continuous) AS
    SELECT
        time_bucket('1 day', timestamp_utc) AS bucket,
        equisenal_id,
        AVG(valor)  AS avg_val,
        MIN(valor)  AS min_val,
        MAX(valor)  AS max_val,
        COUNT(*) FILTER (WHERE calidad = 'Buena') AS n_validas
    FROM ssfv.tbl_valores
    GROUP BY bucket, equisenal_id;
EXCEPTION WHEN OTHERS THEN NULL;
END $$;

DO $$
BEGIN
    PERFORM add_continuous_aggregate_policy('ssfv.mv_valores_diario',
        start_offset      => INTERVAL '3 days',
        end_offset        => INTERVAL '1 day',
        schedule_interval => INTERVAL '1 hour');
EXCEPTION WHEN OTHERS THEN NULL;
END $$;
