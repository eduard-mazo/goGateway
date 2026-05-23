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
-- 2. Catálogo completo de señales SSFV + asignaciones por tipo de equipo
-- ---------------------------------------------------------------------------

DO $$
DECLARE
    v_cac INT; v_cdc INT; v_vac INT; v_vdc INT;
    v_pot INT; v_ene INT; v_pro INT; v_tem INT;
    v_irr INT; v_est INT; v_ala INT;
    u_A    INT; u_V  INT; u_kW   INT; u_kVar INT; u_kVA  INT;
    u_kWh  INT; u_kVarh INT; u_pct INT; u_Hz   INT;
    u_C    INT; u_Wm2 INT; u_MOhm INT; u_adim INT;
    t_inv INT; t_med INT; t_est INT; t_fro INT;
    s_IA    INT; s_IB    INT; s_IC   INT;
    s_UAB   INT; s_UBC   INT; s_UCA  INT;
    s_AP    INT; s_RP    INT; s_SP   INT;
    s_FP    INT; s_EF    INT; s_FR   INT;
    s_ET    INT; s_IP    INT; s_T    INT;
    s_IR    INT; s_OS    INT; s_OSV  INT;
    s_IDCx  INT; s_VDCx  INT; s_EFx  INT;
    s_EVx   INT; s_ALx   INT; s_ALCOM INT;
    s_RD    INT; s_TA    INT; s_TP   INT;
    s_UA    INT; s_API   INT; s_AN   INT;
    s_QPZ   INT; s_QN    INT;
BEGIN
    SELECT tipovar_id INTO v_cac FROM ssfv.tbl_tipo_variable WHERE nombre = 'Corriente AC';
    SELECT tipovar_id INTO v_cdc FROM ssfv.tbl_tipo_variable WHERE nombre = 'Corriente DC';
    SELECT tipovar_id INTO v_vac FROM ssfv.tbl_tipo_variable WHERE nombre = 'Voltage AC';
    SELECT tipovar_id INTO v_vdc FROM ssfv.tbl_tipo_variable WHERE nombre = 'Voltage DC';
    SELECT tipovar_id INTO v_pot FROM ssfv.tbl_tipo_variable WHERE nombre = 'Potencia';
    SELECT tipovar_id INTO v_ene FROM ssfv.tbl_tipo_variable WHERE nombre = 'Energía';
    SELECT tipovar_id INTO v_pro FROM ssfv.tbl_tipo_variable WHERE nombre = 'Proceso';
    SELECT tipovar_id INTO v_tem FROM ssfv.tbl_tipo_variable WHERE nombre = 'Temperatura';
    SELECT tipovar_id INTO v_irr FROM ssfv.tbl_tipo_variable WHERE nombre = 'Irradiancia';
    SELECT tipovar_id INTO v_est FROM ssfv.tbl_tipo_variable WHERE nombre = 'Estado';
    SELECT tipovar_id INTO v_ala FROM ssfv.tbl_tipo_variable WHERE nombre = 'Alarma';

    SELECT unidad_id INTO u_A     FROM ssfv.tbl_unidades WHERE simbolo = 'A';
    SELECT unidad_id INTO u_V     FROM ssfv.tbl_unidades WHERE simbolo = 'V';
    SELECT unidad_id INTO u_kW    FROM ssfv.tbl_unidades WHERE simbolo = 'kW';
    SELECT unidad_id INTO u_kVar  FROM ssfv.tbl_unidades WHERE simbolo = 'kVar';
    SELECT unidad_id INTO u_kVA   FROM ssfv.tbl_unidades WHERE simbolo = 'kVA';
    SELECT unidad_id INTO u_kWh   FROM ssfv.tbl_unidades WHERE simbolo = 'kWh';
    SELECT unidad_id INTO u_kVarh FROM ssfv.tbl_unidades WHERE simbolo = 'kVarh';
    SELECT unidad_id INTO u_pct   FROM ssfv.tbl_unidades WHERE simbolo = '%';
    SELECT unidad_id INTO u_Hz    FROM ssfv.tbl_unidades WHERE simbolo = 'Hz';
    SELECT unidad_id INTO u_C     FROM ssfv.tbl_unidades WHERE simbolo = '°C';
    SELECT unidad_id INTO u_Wm2   FROM ssfv.tbl_unidades WHERE simbolo = 'W/m2';
    SELECT unidad_id INTO u_MOhm  FROM ssfv.tbl_unidades WHERE simbolo = 'MΩ';
    SELECT unidad_id INTO u_adim  FROM ssfv.tbl_unidades WHERE simbolo = 'Adimensional';

    SELECT tipo_id INTO t_inv FROM ssfv.tbl_tipo_equipo WHERE nombre = 'Inversor';
    SELECT tipo_id INTO t_med FROM ssfv.tbl_tipo_equipo WHERE nombre = 'Medidor';
    SELECT tipo_id INTO t_est FROM ssfv.tbl_tipo_equipo WHERE nombre = 'Estación Meteorológica';
    SELECT tipo_id INTO t_fro FROM ssfv.tbl_tipo_equipo WHERE nombre = 'Frontera Comercial';

    INSERT INTO ssfv.tbl_senales
        (tipovar_id, unidad_id, nombre, tipo_valor, codigo_senal, es_indexada, activo)
    VALUES
        (v_cac, u_A,    'Corriente Fase A AC',        'Instantaneo', 'IA',     false, true),
        (v_cac, u_A,    'Corriente Fase B AC',        'Instantaneo', 'IB',     false, true),
        (v_cac, u_A,    'Corriente Fase C AC',        'Instantaneo', 'IC',     false, true),
        (v_vac, u_V,    'Voltaje Línea AB',           'Instantaneo', 'UAB',    false, true),
        (v_vac, u_V,    'Voltaje Línea BC',           'Instantaneo', 'UBC',    false, true),
        (v_vac, u_V,    'Voltaje Línea CA',           'Instantaneo', 'UCA',    false, true),
        (v_pot, u_kW,   'Potencia Activa',            'Instantaneo', 'AP',     false, true),
        (v_pot, u_kVar, 'Potencia Reactiva',          'Instantaneo', 'RP',     false, true),
        (v_pot, u_kVA,  'Potencia Aparente',          'Instantaneo', 'SP',     false, true),
        (v_pro, u_adim, 'Factor de Potencia',         'Instantaneo', 'FP',     false, true),
        (v_pro, u_pct,  'Eficiencia Inversor',        'Instantaneo', 'EF',     false, true),
        (v_pro, u_Hz,   'Frecuencia Red',             'Instantaneo', 'FR',     false, true),
        (v_ene, u_kWh,  'Energía Acumulada',          'Acumulado',   'ET',     false, true),
        (v_pot, u_kW,   'Potencia Entrada DC',        'Instantaneo', 'IP',     false, true),
        (v_tem, u_C,    'Temperatura Inversor',       'Instantaneo', 'T',      false, true),
        (v_pro, u_MOhm, 'Resistencia Aislamiento',   'Instantaneo', 'IR',     false, true),
        (v_est, u_adim, 'Estado Operación',           'Instantaneo', 'OS',     false, true),
        (v_est, u_adim, 'OS Fabricante',              'Instantaneo', 'OSV',    false, true),
        (v_cdc, u_A,    'Corriente DC String',        'Instantaneo', 'IDC_x',  true,  true),
        (v_vdc, u_V,    'Voltaje DC String',          'Instantaneo', 'VDC_x',  true,  true),
        (v_est, u_adim, 'Alarma Dispositivo EF',      'Instantaneo', 'EF_x',   true,  true),
        (v_est, u_adim, 'Alarma Fabricante EV',       'Instantaneo', 'EV_x',   true,  true),
        (v_ala, u_adim, 'Alarma Dispositivo AL',      'Instantaneo', 'AL_x',   true,  true),
        (v_ala, u_adim, 'Alarma Comunicación',        'Instantaneo', 'AL_COM', false, true),
        (v_irr, u_Wm2,  'Irradiancia Principal',     'Instantaneo', 'RD',     false, true),
        (v_tem, u_C,    'Temperatura Ambiente',       'Instantaneo', 'TA',     false, true),
        (v_tem, u_C,    'Temperatura Panel',          'Instantaneo', 'TP',     false, true),
        (v_vac, u_V,    'Voltaje Fase A',             'Instantaneo', 'UA',     false, true),
        (v_ene, u_kWh,  'Energía Activa Importada',  'Acumulado',   'API',    false, true),
        (v_ene, u_kWh,  'Energía Activa Exportada',  'Acumulado',   'AN',     false, true),
        (v_ene, u_kVarh,'Energía Reactiva Importada','Acumulado',   'QPZ',    false, true),
        (v_ene, u_kVarh,'Energía Reactiva Exportada','Acumulado',   'QN',     false, true)
    ON CONFLICT (codigo_senal, tipovar_id) DO NOTHING;

    SELECT senal_id INTO s_IA    FROM ssfv.tbl_senales WHERE codigo_senal = 'IA'     AND tipovar_id = v_cac;
    SELECT senal_id INTO s_IB    FROM ssfv.tbl_senales WHERE codigo_senal = 'IB'     AND tipovar_id = v_cac;
    SELECT senal_id INTO s_IC    FROM ssfv.tbl_senales WHERE codigo_senal = 'IC'     AND tipovar_id = v_cac;
    SELECT senal_id INTO s_UAB   FROM ssfv.tbl_senales WHERE codigo_senal = 'UAB'    AND tipovar_id = v_vac;
    SELECT senal_id INTO s_UBC   FROM ssfv.tbl_senales WHERE codigo_senal = 'UBC'    AND tipovar_id = v_vac;
    SELECT senal_id INTO s_UCA   FROM ssfv.tbl_senales WHERE codigo_senal = 'UCA'    AND tipovar_id = v_vac;
    SELECT senal_id INTO s_AP    FROM ssfv.tbl_senales WHERE codigo_senal = 'AP'     AND tipovar_id = v_pot;
    SELECT senal_id INTO s_RP    FROM ssfv.tbl_senales WHERE codigo_senal = 'RP'     AND tipovar_id = v_pot;
    SELECT senal_id INTO s_SP    FROM ssfv.tbl_senales WHERE codigo_senal = 'SP'     AND tipovar_id = v_pot;
    SELECT senal_id INTO s_FP    FROM ssfv.tbl_senales WHERE codigo_senal = 'FP'     AND tipovar_id = v_pro;
    SELECT senal_id INTO s_EF    FROM ssfv.tbl_senales WHERE codigo_senal = 'EF'     AND tipovar_id = v_pro;
    SELECT senal_id INTO s_FR    FROM ssfv.tbl_senales WHERE codigo_senal = 'FR'     AND tipovar_id = v_pro;
    SELECT senal_id INTO s_ET    FROM ssfv.tbl_senales WHERE codigo_senal = 'ET'     AND tipovar_id = v_ene;
    SELECT senal_id INTO s_IP    FROM ssfv.tbl_senales WHERE codigo_senal = 'IP'     AND tipovar_id = v_pot;
    SELECT senal_id INTO s_T     FROM ssfv.tbl_senales WHERE codigo_senal = 'T'      AND tipovar_id = v_tem;
    SELECT senal_id INTO s_IR    FROM ssfv.tbl_senales WHERE codigo_senal = 'IR'     AND tipovar_id = v_pro;
    SELECT senal_id INTO s_OS    FROM ssfv.tbl_senales WHERE codigo_senal = 'OS'     AND tipovar_id = v_est;
    SELECT senal_id INTO s_OSV   FROM ssfv.tbl_senales WHERE codigo_senal = 'OSV'    AND tipovar_id = v_est;
    SELECT senal_id INTO s_IDCx  FROM ssfv.tbl_senales WHERE codigo_senal = 'IDC_x'  AND tipovar_id = v_cdc;
    SELECT senal_id INTO s_VDCx  FROM ssfv.tbl_senales WHERE codigo_senal = 'VDC_x'  AND tipovar_id = v_vdc;
    SELECT senal_id INTO s_EFx   FROM ssfv.tbl_senales WHERE codigo_senal = 'EF_x'   AND tipovar_id = v_est;
    SELECT senal_id INTO s_EVx   FROM ssfv.tbl_senales WHERE codigo_senal = 'EV_x'   AND tipovar_id = v_est;
    SELECT senal_id INTO s_ALx   FROM ssfv.tbl_senales WHERE codigo_senal = 'AL_x'   AND tipovar_id = v_ala;
    SELECT senal_id INTO s_ALCOM FROM ssfv.tbl_senales WHERE codigo_senal = 'AL_COM' AND tipovar_id = v_ala;
    SELECT senal_id INTO s_RD    FROM ssfv.tbl_senales WHERE codigo_senal = 'RD'     AND tipovar_id = v_irr;
    SELECT senal_id INTO s_TA    FROM ssfv.tbl_senales WHERE codigo_senal = 'TA'     AND tipovar_id = v_tem;
    SELECT senal_id INTO s_TP    FROM ssfv.tbl_senales WHERE codigo_senal = 'TP'     AND tipovar_id = v_tem;
    SELECT senal_id INTO s_UA    FROM ssfv.tbl_senales WHERE codigo_senal = 'UA'     AND tipovar_id = v_vac;
    SELECT senal_id INTO s_API   FROM ssfv.tbl_senales WHERE codigo_senal = 'API'    AND tipovar_id = v_ene;
    SELECT senal_id INTO s_AN    FROM ssfv.tbl_senales WHERE codigo_senal = 'AN'     AND tipovar_id = v_ene;
    SELECT senal_id INTO s_QPZ   FROM ssfv.tbl_senales WHERE codigo_senal = 'QPZ'    AND tipovar_id = v_ene;
    SELECT senal_id INTO s_QN    FROM ssfv.tbl_senales WHERE codigo_senal = 'QN'     AND tipovar_id = v_ene;

    -- Junction Inversor
    INSERT INTO public.tbl_senales_x_tipo_equipo (senal_id, tipo_id, num_canales) VALUES
        (s_IA, t_inv, 1), (s_IB, t_inv, 1), (s_IC, t_inv, 1),
        (s_UAB, t_inv, 1), (s_UBC, t_inv, 1), (s_UCA, t_inv, 1),
        (s_AP, t_inv, 1), (s_RP, t_inv, 1), (s_SP, t_inv, 1),
        (s_FP, t_inv, 1), (s_EF, t_inv, 1), (s_FR, t_inv, 1),
        (s_ET, t_inv, 1), (s_IP, t_inv, 1), (s_T, t_inv, 1),
        (s_IR, t_inv, 1), (s_OS, t_inv, 1), (s_OSV, t_inv, 1),
        (s_IDCx, t_inv, 3), (s_VDCx, t_inv, 3),
        (s_EFx, t_inv, 1), (s_EVx, t_inv, 1),
        (s_ALx, t_inv, 1), (s_ALCOM, t_inv, 1)
    ON CONFLICT DO NOTHING;

    -- Junction Medidor
    INSERT INTO public.tbl_senales_x_tipo_equipo (senal_id, tipo_id, num_canales) VALUES
        (s_UA, t_med, 1), (s_UAB, t_med, 1), (s_UBC, t_med, 1), (s_UCA, t_med, 1),
        (s_IA, t_med, 1), (s_IB, t_med, 1), (s_IC, t_med, 1),
        (s_AP, t_med, 1), (s_RP, t_med, 1), (s_SP, t_med, 1), (s_FP, t_med, 1),
        (s_ET, t_med, 1),
        (s_API, t_med, 1), (s_AN, t_med, 1), (s_QPZ, t_med, 1), (s_QN, t_med, 1),
        (s_ALCOM, t_med, 1)
    ON CONFLICT DO NOTHING;

    -- Junction Estación Meteorológica
    INSERT INTO public.tbl_senales_x_tipo_equipo (senal_id, tipo_id, num_canales) VALUES
        (s_RD, t_est, 1), (s_TA, t_est, 1), (s_TP, t_est, 1), (s_ALCOM, t_est, 1)
    ON CONFLICT DO NOTHING;

    -- Junction Frontera Comercial
    INSERT INTO public.tbl_senales_x_tipo_equipo (senal_id, tipo_id, num_canales) VALUES
        (s_API, t_fro, 1), (s_AN, t_fro, 1), (s_QPZ, t_fro, 1), (s_QN, t_fro, 1),
        (s_IA,  t_fro, 1), (s_UAB, t_fro, 1), (s_ALCOM, t_fro, 1)
    ON CONFLICT DO NOTHING;
END $$;

-- ---------------------------------------------------------------------------
-- 3. Agregados continuos (15 min y diario)
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
