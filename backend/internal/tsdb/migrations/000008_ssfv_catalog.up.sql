-- =============================================================================
-- MIGRATION 008 — SSFV catalog: Es_Alarma column, tipo_equipo↔señales junction,
--                 full signal catalog, continuous aggregates.
-- Idempotent: every statement uses IF NOT EXISTS / ON CONFLICT / EXCEPTION guards.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 1. Columna Es_Alarma en Tbl_Senales
-- ---------------------------------------------------------------------------

DO $$
BEGIN
    ALTER TABLE ssfv."Tbl_Senales" ADD COLUMN "Es_Alarma" BOOLEAN NOT NULL DEFAULT FALSE;
EXCEPTION WHEN duplicate_column THEN NULL;
END $$;

-- ---------------------------------------------------------------------------
-- 2. Tabla de junctions Tipo_Equipo ↔ Señales
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS ssfv."Tbl_Senales_x_Tipo_Equipo" (
    "SenalTipo_Id" SERIAL   PRIMARY KEY,
    "Senal_Id"     INT      NOT NULL REFERENCES ssfv."Tbl_Senales"("Senal_Id"),
    "Tipo_Id"      INT      NOT NULL REFERENCES ssfv."Tbl_Tipo_Equipo"("Tipo_Id"),
    "Num_Canales"  SMALLINT NOT NULL DEFAULT 1,
    CONSTRAINT uq_senal_tipo_equipo UNIQUE ("Senal_Id", "Tipo_Id")
);

-- ---------------------------------------------------------------------------
-- 3. Catálogo completo de señales SSFV + asignaciones por tipo de equipo
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
    SELECT "TipoVar_Id" INTO v_cac FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Corriente AC';
    SELECT "TipoVar_Id" INTO v_cdc FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Corriente DC';
    SELECT "TipoVar_Id" INTO v_vac FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Voltage AC';
    SELECT "TipoVar_Id" INTO v_vdc FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Voltage DC';
    SELECT "TipoVar_Id" INTO v_pot FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Potencia';
    SELECT "TipoVar_Id" INTO v_ene FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Energía';
    SELECT "TipoVar_Id" INTO v_pro FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Proceso';
    SELECT "TipoVar_Id" INTO v_tem FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Temperatura';
    SELECT "TipoVar_Id" INTO v_irr FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Irradiancia';
    SELECT "TipoVar_Id" INTO v_est FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Estado';
    SELECT "TipoVar_Id" INTO v_ala FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Alarma';

    SELECT "Unidad_Id" INTO u_A     FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'A';
    SELECT "Unidad_Id" INTO u_V     FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'V';
    SELECT "Unidad_Id" INTO u_kW    FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kW';
    SELECT "Unidad_Id" INTO u_kVar  FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kVar';
    SELECT "Unidad_Id" INTO u_kVA   FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kVA';
    SELECT "Unidad_Id" INTO u_kWh   FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kWh';
    SELECT "Unidad_Id" INTO u_kVarh FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kVarh';
    SELECT "Unidad_Id" INTO u_pct   FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = '%';
    SELECT "Unidad_Id" INTO u_Hz    FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'Hz';
    SELECT "Unidad_Id" INTO u_C     FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = '°C';
    SELECT "Unidad_Id" INTO u_Wm2   FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'W/m2';
    SELECT "Unidad_Id" INTO u_MOhm  FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'MΩ';
    SELECT "Unidad_Id" INTO u_adim  FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'Adimensional';

    SELECT "Tipo_Id" INTO t_inv FROM ssfv."Tbl_Tipo_Equipo" WHERE "Nombre" = 'Inversor';
    SELECT "Tipo_Id" INTO t_med FROM ssfv."Tbl_Tipo_Equipo" WHERE "Nombre" = 'Medidor';
    SELECT "Tipo_Id" INTO t_est FROM ssfv."Tbl_Tipo_Equipo" WHERE "Nombre" = 'Estación Meteorológica';
    SELECT "Tipo_Id" INTO t_fro FROM ssfv."Tbl_Tipo_Equipo" WHERE "Nombre" = 'Frontera Comercial';

    INSERT INTO ssfv."Tbl_Senales"
        ("TipoVar_Id","Unidad_Id","Nombre","Tipo_Valor","Codigo_Senal","Es_Indexada","Es_Alarma","Activo")
    VALUES
        (v_cac, u_A,    'Corriente Fase A AC',        'Instantaneo', 'IA',     false, false, true),
        (v_cac, u_A,    'Corriente Fase B AC',        'Instantaneo', 'IB',     false, false, true),
        (v_cac, u_A,    'Corriente Fase C AC',        'Instantaneo', 'IC',     false, false, true),
        (v_vac, u_V,    'Voltaje Línea AB',           'Instantaneo', 'UAB',    false, false, true),
        (v_vac, u_V,    'Voltaje Línea BC',           'Instantaneo', 'UBC',    false, false, true),
        (v_vac, u_V,    'Voltaje Línea CA',           'Instantaneo', 'UCA',    false, false, true),
        (v_pot, u_kW,   'Potencia Activa',            'Instantaneo', 'AP',     false, false, true),
        (v_pot, u_kVar, 'Potencia Reactiva',          'Instantaneo', 'RP',     false, false, true),
        (v_pot, u_kVA,  'Potencia Aparente',          'Instantaneo', 'SP',     false, false, true),
        (v_pro, u_adim, 'Factor de Potencia',         'Instantaneo', 'FP',     false, false, true),
        (v_pro, u_pct,  'Eficiencia Inversor',        'Instantaneo', 'EF',     false, false, true),
        (v_pro, u_Hz,   'Frecuencia Red',             'Instantaneo', 'FR',     false, false, true),
        (v_ene, u_kWh,  'Energía Acumulada',          'Acumulado',   'ET',     false, false, true),
        (v_pot, u_kW,   'Potencia Entrada DC',        'Instantaneo', 'IP',     false, false, true),
        (v_tem, u_C,    'Temperatura Inversor',       'Instantaneo', 'T',      false, false, true),
        (v_pro, u_MOhm, 'Resistencia Aislamiento',   'Instantaneo', 'IR',     false, false, true),
        (v_est, u_adim, 'Estado Operación',           'Instantaneo', 'OS',     false, false, true),
        (v_est, u_adim, 'OS Fabricante',              'Instantaneo', 'OSV',    false, false, true),
        (v_cdc, u_A,    'Corriente DC String',        'Instantaneo', 'IDC_x',  true,  false, true),
        (v_vdc, u_V,    'Voltaje DC String',          'Instantaneo', 'VDC_x',  true,  false, true),
        (v_est, u_adim, 'Alarma Dispositivo EF',      'Instantaneo', 'EF_x',   true,  true,  true),
        (v_est, u_adim, 'Alarma Fabricante EV',       'Instantaneo', 'EV_x',   true,  true,  true),
        (v_ala, u_adim, 'Alarma Dispositivo AL',      'Instantaneo', 'AL_x',   true,  true,  true),
        (v_ala, u_adim, 'Alarma Comunicación',        'Instantaneo', 'AL_COM', false, true,  true),
        (v_irr, u_Wm2,  'Irradiancia Principal',     'Instantaneo', 'RD',     false, false, true),
        (v_tem, u_C,    'Temperatura Ambiente',       'Instantaneo', 'TA',     false, false, true),
        (v_tem, u_C,    'Temperatura Panel',          'Instantaneo', 'TP',     false, false, true),
        (v_vac, u_V,    'Voltaje Fase A',             'Instantaneo', 'UA',     false, false, true),
        (v_ene, u_kWh,  'Energía Activa Importada',  'Acumulado',   'API',    false, false, true),
        (v_ene, u_kWh,  'Energía Activa Exportada',  'Acumulado',   'AN',     false, false, true),
        (v_ene, u_kVarh,'Energía Reactiva Importada','Acumulado',   'QPZ',    false, false, true),
        (v_ene, u_kVarh,'Energía Reactiva Exportada','Acumulado',   'QN',     false, false, true)
    ON CONFLICT ("Codigo_Senal", "TipoVar_Id") DO NOTHING;

    SELECT "Senal_Id" INTO s_IA    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'IA'     AND "TipoVar_Id" = v_cac;
    SELECT "Senal_Id" INTO s_IB    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'IB'     AND "TipoVar_Id" = v_cac;
    SELECT "Senal_Id" INTO s_IC    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'IC'     AND "TipoVar_Id" = v_cac;
    SELECT "Senal_Id" INTO s_UAB   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'UAB'    AND "TipoVar_Id" = v_vac;
    SELECT "Senal_Id" INTO s_UBC   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'UBC'    AND "TipoVar_Id" = v_vac;
    SELECT "Senal_Id" INTO s_UCA   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'UCA'    AND "TipoVar_Id" = v_vac;
    SELECT "Senal_Id" INTO s_AP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'AP'     AND "TipoVar_Id" = v_pot;
    SELECT "Senal_Id" INTO s_RP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'RP'     AND "TipoVar_Id" = v_pot;
    SELECT "Senal_Id" INTO s_SP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'SP'     AND "TipoVar_Id" = v_pot;
    SELECT "Senal_Id" INTO s_FP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'FP'     AND "TipoVar_Id" = v_pro;
    SELECT "Senal_Id" INTO s_EF    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'EF'     AND "TipoVar_Id" = v_pro;
    SELECT "Senal_Id" INTO s_FR    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'FR'     AND "TipoVar_Id" = v_pro;
    SELECT "Senal_Id" INTO s_ET    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'ET'     AND "TipoVar_Id" = v_ene;
    SELECT "Senal_Id" INTO s_IP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'IP'     AND "TipoVar_Id" = v_pot;
    SELECT "Senal_Id" INTO s_T     FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'T'      AND "TipoVar_Id" = v_tem;
    SELECT "Senal_Id" INTO s_IR    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'IR'     AND "TipoVar_Id" = v_pro;
    SELECT "Senal_Id" INTO s_OS    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'OS'     AND "TipoVar_Id" = v_est;
    SELECT "Senal_Id" INTO s_OSV   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'OSV'    AND "TipoVar_Id" = v_est;
    SELECT "Senal_Id" INTO s_IDCx  FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'IDC_x'  AND "TipoVar_Id" = v_cdc;
    SELECT "Senal_Id" INTO s_VDCx  FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'VDC_x'  AND "TipoVar_Id" = v_vdc;
    SELECT "Senal_Id" INTO s_EFx   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'EF_x'   AND "TipoVar_Id" = v_est;
    SELECT "Senal_Id" INTO s_EVx   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'EV_x'   AND "TipoVar_Id" = v_est;
    SELECT "Senal_Id" INTO s_ALx   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'AL_x'   AND "TipoVar_Id" = v_ala;
    SELECT "Senal_Id" INTO s_ALCOM FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'AL_COM' AND "TipoVar_Id" = v_ala;
    SELECT "Senal_Id" INTO s_RD    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'RD'     AND "TipoVar_Id" = v_irr;
    SELECT "Senal_Id" INTO s_TA    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'TA'     AND "TipoVar_Id" = v_tem;
    SELECT "Senal_Id" INTO s_TP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'TP'     AND "TipoVar_Id" = v_tem;
    SELECT "Senal_Id" INTO s_UA    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'UA'     AND "TipoVar_Id" = v_vac;
    SELECT "Senal_Id" INTO s_API   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'API'    AND "TipoVar_Id" = v_ene;
    SELECT "Senal_Id" INTO s_AN    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'AN'     AND "TipoVar_Id" = v_ene;
    SELECT "Senal_Id" INTO s_QPZ   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'QPZ'    AND "TipoVar_Id" = v_ene;
    SELECT "Senal_Id" INTO s_QN    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'QN'     AND "TipoVar_Id" = v_ene;

    -- Junction Inversor
    INSERT INTO ssfv."Tbl_Senales_x_Tipo_Equipo" ("Senal_Id","Tipo_Id","Num_Canales") VALUES
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
    INSERT INTO ssfv."Tbl_Senales_x_Tipo_Equipo" ("Senal_Id","Tipo_Id","Num_Canales") VALUES
        (s_UA, t_med, 1), (s_UAB, t_med, 1), (s_UBC, t_med, 1), (s_UCA, t_med, 1),
        (s_IA, t_med, 1), (s_IB, t_med, 1), (s_IC, t_med, 1),
        (s_AP, t_med, 1), (s_RP, t_med, 1), (s_SP, t_med, 1), (s_FP, t_med, 1),
        (s_ET, t_med, 1),
        (s_API, t_med, 1), (s_AN, t_med, 1), (s_QPZ, t_med, 1), (s_QN, t_med, 1),
        (s_ALCOM, t_med, 1)
    ON CONFLICT DO NOTHING;

    -- Junction Estación Meteorológica
    INSERT INTO ssfv."Tbl_Senales_x_Tipo_Equipo" ("Senal_Id","Tipo_Id","Num_Canales") VALUES
        (s_RD, t_est, 1), (s_TA, t_est, 1), (s_TP, t_est, 1), (s_ALCOM, t_est, 1)
    ON CONFLICT DO NOTHING;

    -- Junction Frontera Comercial
    INSERT INTO ssfv."Tbl_Senales_x_Tipo_Equipo" ("Senal_Id","Tipo_Id","Num_Canales") VALUES
        (s_API, t_fro, 1), (s_AN, t_fro, 1), (s_QPZ, t_fro, 1), (s_QN, t_fro, 1),
        (s_IA, t_fro, 1), (s_UAB, t_fro, 1), (s_ALCOM, t_fro, 1)
    ON CONFLICT DO NOTHING;
END $$;

-- ---------------------------------------------------------------------------
-- 4. Continuous aggregates (15 min y diario)
-- ---------------------------------------------------------------------------

DO $$
BEGIN
    CREATE MATERIALIZED VIEW ssfv.mv_valores_15min
    WITH (timescaledb.continuous) AS
    SELECT
        time_bucket('15 minutes', "Timestamp_UTC") AS bucket,
        "EquiSenal_Id",
        AVG("Valor")  AS avg_val,
        MIN("Valor")  AS min_val,
        MAX("Valor")  AS max_val,
        COUNT(*)      AS n_total,
        COUNT(*) FILTER (WHERE "Calidad" = 'Buena') AS n_validas
    FROM ssfv."Tbl_Valores"
    GROUP BY bucket, "EquiSenal_Id";
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
        time_bucket('1 day', "Timestamp_UTC") AS bucket,
        "EquiSenal_Id",
        AVG("Valor")  AS avg_val,
        MIN("Valor")  AS min_val,
        MAX("Valor")  AS max_val,
        COUNT(*) FILTER (WHERE "Calidad" = 'Buena') AS n_validas
    FROM ssfv."Tbl_Valores"
    GROUP BY bucket, "EquiSenal_Id";
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
