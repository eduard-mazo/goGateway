-- Migration 009: Seed EPM Sede 30 plant catalog from uns_signals.json
-- Covers: Planta, 4 Equipos (INV_1, MED_1, EST_1, FRONTERA_20949722),
-- and all tbl_senales_x_equipo instances.
-- Idempotent: ON CONFLICT DO NOTHING + WHERE NOT EXISTS guards throughout.

DO $$
DECLARE
    -- tipo_equipo
    t_inv  INT; t_med INT; t_est INT; t_fro INT;
    -- tipo_variable
    v_cac  INT; v_vac  INT; v_pot INT; v_pro INT;
    v_ene  INT; v_tem  INT; v_est INT; v_cdc INT;
    v_vdc  INT; v_ala  INT; v_irr INT;
    -- unidades
    u_A    INT; u_V    INT; u_kW   INT; u_kVar INT;
    u_kVA  INT; u_pct  INT; u_Hz   INT; u_kWh  INT;
    u_kVarh INT; u_C   INT; u_MOhm INT; u_adim INT; u_Wm2 INT;
    -- señal IDs (catalog)
    s_IA   INT; s_IB   INT; s_IC   INT;
    s_UAB  INT; s_UBC  INT; s_UCA  INT;
    s_UA   INT; s_UB   INT; s_UC   INT;
    s_AP   INT; s_RP   INT; s_SP   INT; s_FP   INT;
    s_EF   INT; s_FR   INT;
    s_ET   INT; s_IP   INT; s_T    INT; s_IR   INT;
    s_OS   INT; s_OSV  INT;
    s_IDCx INT; s_VDCx INT;
    s_EFx  INT; s_EVx  INT; s_ALx  INT; s_ALCOM INT;
    s_RD   INT; s_TA   INT; s_TP   INT;
    s_API  INT; s_AN   INT; s_QPZ  INT; s_QN   INT;
    -- señales nuevas (not in prior migrations)
    s_AP_A INT; s_AP_B INT; s_AP_C INT;
    s_RP_A INT; s_RP_B INT; s_RP_C INT;
    -- planta y equipo IDs
    p_sede30 INT;
    e_inv1   INT; e_med1 INT; e_est1 INT; e_fro INT;
BEGIN

    -- ── Lookup tipo_equipo ────────────────────────────────────────────────────
    SELECT tipo_id INTO t_inv FROM ssfv.tbl_tipo_equipo WHERE nombre = 'Inversor';
    SELECT tipo_id INTO t_med FROM ssfv.tbl_tipo_equipo WHERE nombre = 'Medidor';
    SELECT tipo_id INTO t_est FROM ssfv.tbl_tipo_equipo WHERE nombre = 'Estación Meteorológica';
    SELECT tipo_id INTO t_fro FROM ssfv.tbl_tipo_equipo WHERE nombre = 'Frontera Comercial';

    -- ── Lookup tipo_variable ──────────────────────────────────────────────────
    SELECT tipovar_id INTO v_cac FROM ssfv.tbl_tipo_variable WHERE nombre = 'Corriente AC';
    SELECT tipovar_id INTO v_vac FROM ssfv.tbl_tipo_variable WHERE nombre = 'Voltage AC';
    SELECT tipovar_id INTO v_pot FROM ssfv.tbl_tipo_variable WHERE nombre = 'Potencia';
    SELECT tipovar_id INTO v_pro FROM ssfv.tbl_tipo_variable WHERE nombre = 'Proceso';
    SELECT tipovar_id INTO v_ene FROM ssfv.tbl_tipo_variable WHERE nombre = 'Energía';
    SELECT tipovar_id INTO v_tem FROM ssfv.tbl_tipo_variable WHERE nombre = 'Temperatura';
    SELECT tipovar_id INTO v_est FROM ssfv.tbl_tipo_variable WHERE nombre = 'Estado';
    SELECT tipovar_id INTO v_cdc FROM ssfv.tbl_tipo_variable WHERE nombre = 'Corriente DC';
    SELECT tipovar_id INTO v_vdc FROM ssfv.tbl_tipo_variable WHERE nombre = 'Voltage DC';
    SELECT tipovar_id INTO v_ala FROM ssfv.tbl_tipo_variable WHERE nombre = 'Alarma';
    SELECT tipovar_id INTO v_irr FROM ssfv.tbl_tipo_variable WHERE nombre = 'Irradiancia';

    -- ── Lookup unidades ───────────────────────────────────────────────────────
    SELECT unidad_id INTO u_A     FROM ssfv.tbl_unidades WHERE simbolo = 'A';
    SELECT unidad_id INTO u_V     FROM ssfv.tbl_unidades WHERE simbolo = 'V';
    SELECT unidad_id INTO u_kW    FROM ssfv.tbl_unidades WHERE simbolo = 'kW';
    SELECT unidad_id INTO u_kVar  FROM ssfv.tbl_unidades WHERE simbolo = 'kVar';
    SELECT unidad_id INTO u_kVA   FROM ssfv.tbl_unidades WHERE simbolo = 'kVA';
    SELECT unidad_id INTO u_pct   FROM ssfv.tbl_unidades WHERE simbolo = '%';
    SELECT unidad_id INTO u_Hz    FROM ssfv.tbl_unidades WHERE simbolo = 'Hz';
    SELECT unidad_id INTO u_kWh   FROM ssfv.tbl_unidades WHERE simbolo = 'kWh';
    SELECT unidad_id INTO u_kVarh FROM ssfv.tbl_unidades WHERE simbolo = 'kVarh';
    SELECT unidad_id INTO u_C     FROM ssfv.tbl_unidades WHERE simbolo = '°C';
    SELECT unidad_id INTO u_MOhm  FROM ssfv.tbl_unidades WHERE simbolo = 'MΩ';
    SELECT unidad_id INTO u_adim  FROM ssfv.tbl_unidades WHERE simbolo = 'Adimensional';
    SELECT unidad_id INTO u_Wm2   FROM ssfv.tbl_unidades WHERE simbolo = 'W/m2';

    -- ── Señales nuevas (UB, UC, per-phase powers) ─────────────────────────────
    INSERT INTO ssfv.tbl_senales
        (tipovar_id, unidad_id, nombre, tipo_valor, codigo_senal, es_indexada, activo)
    VALUES
        (v_vac, u_V,    'Voltaje Fase B',           'Instantaneo', 'UB',   false, true),
        (v_vac, u_V,    'Voltaje Fase C',           'Instantaneo', 'UC',   false, true),
        (v_pot, u_kW,   'Potencia Activa Fase A',   'Instantaneo', 'AP_A', false, true),
        (v_pot, u_kW,   'Potencia Activa Fase B',   'Instantaneo', 'AP_B', false, true),
        (v_pot, u_kW,   'Potencia Activa Fase C',   'Instantaneo', 'AP_C', false, true),
        (v_pot, u_kVar, 'Potencia Reactiva Fase A', 'Instantaneo', 'RP_A', false, true),
        (v_pot, u_kVar, 'Potencia Reactiva Fase B', 'Instantaneo', 'RP_B', false, true),
        (v_pot, u_kVar, 'Potencia Reactiva Fase C', 'Instantaneo', 'RP_C', false, true)
    ON CONFLICT (codigo_senal, tipovar_id) DO NOTHING;

    -- ── Lookup señal IDs ──────────────────────────────────────────────────────
    SELECT senal_id INTO s_IA    FROM ssfv.tbl_senales WHERE codigo_senal='IA'    AND tipovar_id=v_cac;
    SELECT senal_id INTO s_IB    FROM ssfv.tbl_senales WHERE codigo_senal='IB'    AND tipovar_id=v_cac;
    SELECT senal_id INTO s_IC    FROM ssfv.tbl_senales WHERE codigo_senal='IC'    AND tipovar_id=v_cac;
    SELECT senal_id INTO s_UAB   FROM ssfv.tbl_senales WHERE codigo_senal='UAB'   AND tipovar_id=v_vac;
    SELECT senal_id INTO s_UBC   FROM ssfv.tbl_senales WHERE codigo_senal='UBC'   AND tipovar_id=v_vac;
    SELECT senal_id INTO s_UCA   FROM ssfv.tbl_senales WHERE codigo_senal='UCA'   AND tipovar_id=v_vac;
    SELECT senal_id INTO s_UA    FROM ssfv.tbl_senales WHERE codigo_senal='UA'    AND tipovar_id=v_vac;
    SELECT senal_id INTO s_UB    FROM ssfv.tbl_senales WHERE codigo_senal='UB'    AND tipovar_id=v_vac;
    SELECT senal_id INTO s_UC    FROM ssfv.tbl_senales WHERE codigo_senal='UC'    AND tipovar_id=v_vac;
    SELECT senal_id INTO s_AP    FROM ssfv.tbl_senales WHERE codigo_senal='AP'    AND tipovar_id=v_pot;
    SELECT senal_id INTO s_RP    FROM ssfv.tbl_senales WHERE codigo_senal='RP'    AND tipovar_id=v_pot;
    SELECT senal_id INTO s_SP    FROM ssfv.tbl_senales WHERE codigo_senal='SP'    AND tipovar_id=v_pot;
    SELECT senal_id INTO s_FP    FROM ssfv.tbl_senales WHERE codigo_senal='FP'    AND tipovar_id=v_pro;
    SELECT senal_id INTO s_EF    FROM ssfv.tbl_senales WHERE codigo_senal='EF'    AND tipovar_id=v_pro;
    SELECT senal_id INTO s_FR    FROM ssfv.tbl_senales WHERE codigo_senal='FR'    AND tipovar_id=v_pro;
    SELECT senal_id INTO s_ET    FROM ssfv.tbl_senales WHERE codigo_senal='ET'    AND tipovar_id=v_ene;
    SELECT senal_id INTO s_IP    FROM ssfv.tbl_senales WHERE codigo_senal='IP'    AND tipovar_id=v_pot;
    SELECT senal_id INTO s_T     FROM ssfv.tbl_senales WHERE codigo_senal='T'     AND tipovar_id=v_tem;
    SELECT senal_id INTO s_IR    FROM ssfv.tbl_senales WHERE codigo_senal='IR'    AND tipovar_id=v_pro;
    SELECT senal_id INTO s_OS    FROM ssfv.tbl_senales WHERE codigo_senal='OS'    AND tipovar_id=v_est;
    SELECT senal_id INTO s_OSV   FROM ssfv.tbl_senales WHERE codigo_senal='OSV'   AND tipovar_id=v_est;
    SELECT senal_id INTO s_IDCx  FROM ssfv.tbl_senales WHERE codigo_senal='IDC_x' AND tipovar_id=v_cdc;
    SELECT senal_id INTO s_VDCx  FROM ssfv.tbl_senales WHERE codigo_senal='VDC_x' AND tipovar_id=v_vdc;
    SELECT senal_id INTO s_EFx   FROM ssfv.tbl_senales WHERE codigo_senal='EF_x'  AND tipovar_id=v_est;
    SELECT senal_id INTO s_EVx   FROM ssfv.tbl_senales WHERE codigo_senal='EV_x'  AND tipovar_id=v_est;
    SELECT senal_id INTO s_ALx   FROM ssfv.tbl_senales WHERE codigo_senal='AL_x'  AND tipovar_id=v_ala;
    SELECT senal_id INTO s_ALCOM FROM ssfv.tbl_senales WHERE codigo_senal='AL_COM' AND tipovar_id=v_ala;
    SELECT senal_id INTO s_RD    FROM ssfv.tbl_senales WHERE codigo_senal='RD'    AND tipovar_id=v_irr;
    SELECT senal_id INTO s_TA    FROM ssfv.tbl_senales WHERE codigo_senal='TA'    AND tipovar_id=v_tem;
    SELECT senal_id INTO s_TP    FROM ssfv.tbl_senales WHERE codigo_senal='TP'    AND tipovar_id=v_tem;
    SELECT senal_id INTO s_API   FROM ssfv.tbl_senales WHERE codigo_senal='API'   AND tipovar_id=v_ene;
    SELECT senal_id INTO s_AN    FROM ssfv.tbl_senales WHERE codigo_senal='AN'    AND tipovar_id=v_ene;
    SELECT senal_id INTO s_QPZ   FROM ssfv.tbl_senales WHERE codigo_senal='QPZ'   AND tipovar_id=v_ene;
    SELECT senal_id INTO s_QN    FROM ssfv.tbl_senales WHERE codigo_senal='QN'    AND tipovar_id=v_ene;
    SELECT senal_id INTO s_AP_A  FROM ssfv.tbl_senales WHERE codigo_senal='AP_A'  AND tipovar_id=v_pot;
    SELECT senal_id INTO s_AP_B  FROM ssfv.tbl_senales WHERE codigo_senal='AP_B'  AND tipovar_id=v_pot;
    SELECT senal_id INTO s_AP_C  FROM ssfv.tbl_senales WHERE codigo_senal='AP_C'  AND tipovar_id=v_pot;
    SELECT senal_id INTO s_RP_A  FROM ssfv.tbl_senales WHERE codigo_senal='RP_A'  AND tipovar_id=v_pot;
    SELECT senal_id INTO s_RP_B  FROM ssfv.tbl_senales WHERE codigo_senal='RP_B'  AND tipovar_id=v_pot;
    SELECT senal_id INTO s_RP_C  FROM ssfv.tbl_senales WHERE codigo_senal='RP_C'  AND tipovar_id=v_pot;

    -- ── Planta: EPM Sede 30 ───────────────────────────────────────────────────
    INSERT INTO ssfv.tbl_planta (nombre, ubicacion, propietario, broker_base, estado)
    VALUES ('EPM Sede 30', 'Medellín, Colombia', 'EPM', 'EPM/SSFV/EPM/Sede30', 1)
    ON CONFLICT (broker_base) DO NOTHING;
    SELECT planta_id INTO p_sede30
    FROM ssfv.tbl_planta WHERE broker_base = 'EPM/SSFV/EPM/Sede30';

    -- ── Equipos ───────────────────────────────────────────────────────────────
    INSERT INTO ssfv.tbl_equipo (planta_id, tipo_id, nombre_equipo, nombre_topic, estado)
    VALUES (p_sede30, t_inv, 'Inversor 1', 'EPM/SSFV/EPM/Sede30/INV_1', 1)
    ON CONFLICT (nombre_topic) DO NOTHING;
    SELECT equipo_id INTO e_inv1
    FROM ssfv.tbl_equipo WHERE nombre_topic = 'EPM/SSFV/EPM/Sede30/INV_1';

    INSERT INTO ssfv.tbl_equipo (planta_id, tipo_id, nombre_equipo, nombre_topic, estado)
    VALUES (p_sede30, t_med, 'Medidor 1', 'EPM/SSFV/EPM/Sede30/MED_1', 1)
    ON CONFLICT (nombre_topic) DO NOTHING;
    SELECT equipo_id INTO e_med1
    FROM ssfv.tbl_equipo WHERE nombre_topic = 'EPM/SSFV/EPM/Sede30/MED_1';

    INSERT INTO ssfv.tbl_equipo (planta_id, tipo_id, nombre_equipo, nombre_topic, estado)
    VALUES (p_sede30, t_est, 'Estación Met 1', 'EPM/SSFV/EPM/Sede30/EST_1', 1)
    ON CONFLICT (nombre_topic) DO NOTHING;
    SELECT equipo_id INTO e_est1
    FROM ssfv.tbl_equipo WHERE nombre_topic = 'EPM/SSFV/EPM/Sede30/EST_1';

    INSERT INTO ssfv.tbl_equipo (planta_id, tipo_id, nombre_equipo, nombre_topic, estado)
    VALUES (p_sede30, t_fro, 'Frontera 20949722', 'EPM/SSFV/EPM/Sede30/FRONTERA_20949722', 1)
    ON CONFLICT (nombre_topic) DO NOTHING;
    SELECT equipo_id INTO e_fro
    FROM ssfv.tbl_equipo WHERE nombre_topic = 'EPM/SSFV/EPM/Sede30/FRONTERA_20949722';

    -- ── Señales x Equipo: INV_1 (non-indexed) ────────────────────────────────
    INSERT INTO ssfv.tbl_senales_x_equipo (senal_id, equipo_id, nombre_instancia, activo)
    SELECT v.sid, e_inv1, v.inst, TRUE
    FROM (VALUES
        (s_IA,'IA'),(s_IB,'IB'),(s_IC,'IC'),
        (s_UAB,'UAB'),(s_UBC,'UBC'),(s_UCA,'UCA'),
        (s_UA,'UA'),(s_UB,'UB'),(s_UC,'UC'),
        (s_EF,'EF'),(s_FR,'FR'),
        (s_AP,'AP'),(s_RP,'RP'),(s_SP,'SP'),(s_FP,'FP'),
        (s_ET,'ET'),(s_IP,'IP'),(s_T,'T'),(s_IR,'IR'),
        (s_OS,'OS'),(s_OSV,'OSV'),(s_ALCOM,'AL_COM')
    ) AS v(sid, inst)
    WHERE NOT EXISTS (
        SELECT 1 FROM ssfv.tbl_senales_x_equipo x
        WHERE x.senal_id=v.sid AND x.equipo_id=e_inv1 AND x.indice_canal IS NULL
    );

    -- INV_1 indexed: IDC, VDC, EF_x, EV_x, AL_x — 3 strings
    INSERT INTO ssfv.tbl_senales_x_equipo (senal_id, equipo_id, indice_canal, nombre_instancia, activo)
    VALUES
        (s_IDCx, e_inv1, 1, 'IDC_1', TRUE),
        (s_IDCx, e_inv1, 2, 'IDC_2', TRUE),
        (s_IDCx, e_inv1, 3, 'IDC_3', TRUE),
        (s_VDCx, e_inv1, 1, 'VDC_1', TRUE),
        (s_VDCx, e_inv1, 2, 'VDC_2', TRUE),
        (s_VDCx, e_inv1, 3, 'VDC_3', TRUE),
        (s_EFx,  e_inv1, 1, 'EF_1',  TRUE),
        (s_EFx,  e_inv1, 2, 'EF_2',  TRUE),
        (s_EFx,  e_inv1, 3, 'EF_3',  TRUE),
        (s_EVx,  e_inv1, 1, 'EV_1',  TRUE),
        (s_EVx,  e_inv1, 2, 'EV_2',  TRUE),
        (s_EVx,  e_inv1, 3, 'EV_3',  TRUE),
        (s_ALx,  e_inv1, 1, 'AL_1',  TRUE),
        (s_ALx,  e_inv1, 2, 'AL_2',  TRUE),
        (s_ALx,  e_inv1, 3, 'AL_3',  TRUE)
    ON CONFLICT ON CONSTRAINT uq_senal_equipo_canal DO NOTHING;

    -- ── Señales x Equipo: MED_1 ───────────────────────────────────────────────
    INSERT INTO ssfv.tbl_senales_x_equipo (senal_id, equipo_id, nombre_instancia, activo)
    SELECT v.sid, e_med1, v.inst, TRUE
    FROM (VALUES
        (s_UA,'UA'),(s_UB,'UB'),(s_UC,'UC'),
        (s_IA,'IA'),(s_IB,'IB'),(s_IC,'IC'),
        (s_AP,'AP'),(s_AP_A,'AP_A'),(s_AP_B,'AP_B'),(s_AP_C,'AP_C'),
        (s_RP,'RP'),(s_RP_A,'RP_A'),(s_RP_B,'RP_B'),(s_RP_C,'RP_C'),
        (s_FP,'FP'),(s_SP,'SP'),
        (s_API,'API'),(s_AN,'AN'),(s_QPZ,'QPZ'),(s_QN,'QN'),
        (s_EF,'EF'),(s_ALCOM,'AL_COM')
    ) AS v(sid, inst)
    WHERE NOT EXISTS (
        SELECT 1 FROM ssfv.tbl_senales_x_equipo x
        WHERE x.senal_id=v.sid AND x.equipo_id=e_med1 AND x.indice_canal IS NULL
    );

    -- ── Señales x Equipo: EST_1 ───────────────────────────────────────────────
    INSERT INTO ssfv.tbl_senales_x_equipo (senal_id, equipo_id, nombre_instancia, activo)
    SELECT v.sid, e_est1, v.inst, TRUE
    FROM (VALUES
        (s_RD,'RD'),(s_TA,'TA'),(s_TP,'TP'),(s_ALCOM,'AL_COM')
    ) AS v(sid, inst)
    WHERE NOT EXISTS (
        SELECT 1 FROM ssfv.tbl_senales_x_equipo x
        WHERE x.senal_id=v.sid AND x.equipo_id=e_est1 AND x.indice_canal IS NULL
    );

    -- ── Señales x Equipo: FRONTERA_20949722 ───────────────────────────────────
    INSERT INTO ssfv.tbl_senales_x_equipo (senal_id, equipo_id, nombre_instancia, activo)
    SELECT v.sid, e_fro, v.inst, TRUE
    FROM (VALUES
        (s_API,'API'),(s_AN,'AN'),(s_QN,'QN'),(s_QPZ,'QPZ'),
        (s_IA,'IA'),(s_IB,'IB'),(s_IC,'IC'),
        (s_UA,'UA'),(s_UB,'UB'),(s_UC,'UC')
    ) AS v(sid, inst)
    WHERE NOT EXISTS (
        SELECT 1 FROM ssfv.tbl_senales_x_equipo x
        WHERE x.senal_id=v.sid AND x.equipo_id=e_fro AND x.indice_canal IS NULL
    );

END $$;
