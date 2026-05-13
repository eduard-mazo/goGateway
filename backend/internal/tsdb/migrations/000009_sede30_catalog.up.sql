-- Migration 009: Seed EPM Sede 30 plant catalog from uns_signals.json
-- Covers: Planta, 4 Equipos (INV_1, MED_1, EST_1, FRONTERA_20949722),
-- and all Tbl_Senales_x_Equipo instances.
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
    SELECT "Tipo_Id" INTO t_inv FROM ssfv."Tbl_Tipo_Equipo" WHERE "Nombre" = 'Inversor';
    SELECT "Tipo_Id" INTO t_med FROM ssfv."Tbl_Tipo_Equipo" WHERE "Nombre" = 'Medidor';
    SELECT "Tipo_Id" INTO t_est FROM ssfv."Tbl_Tipo_Equipo" WHERE "Nombre" = 'Estación Meteorológica';
    SELECT "Tipo_Id" INTO t_fro FROM ssfv."Tbl_Tipo_Equipo" WHERE "Nombre" = 'Frontera Comercial';

    -- ── Lookup tipo_variable ──────────────────────────────────────────────────
    SELECT "TipoVar_Id" INTO v_cac FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Corriente AC';
    SELECT "TipoVar_Id" INTO v_vac FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Voltage AC';
    SELECT "TipoVar_Id" INTO v_pot FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Potencia';
    SELECT "TipoVar_Id" INTO v_pro FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Proceso';
    SELECT "TipoVar_Id" INTO v_ene FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Energía';
    SELECT "TipoVar_Id" INTO v_tem FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Temperatura';
    SELECT "TipoVar_Id" INTO v_est FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Estado';
    SELECT "TipoVar_Id" INTO v_cdc FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Corriente DC';
    SELECT "TipoVar_Id" INTO v_vdc FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Voltage DC';
    SELECT "TipoVar_Id" INTO v_ala FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Alarma';
    SELECT "TipoVar_Id" INTO v_irr FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Irradiancia';

    -- ── Lookup unidades ───────────────────────────────────────────────────────
    SELECT "Unidad_Id" INTO u_A     FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'A';
    SELECT "Unidad_Id" INTO u_V     FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'V';
    SELECT "Unidad_Id" INTO u_kW    FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kW';
    SELECT "Unidad_Id" INTO u_kVar  FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kVar';
    SELECT "Unidad_Id" INTO u_kVA   FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kVA';
    SELECT "Unidad_Id" INTO u_pct   FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = '%';
    SELECT "Unidad_Id" INTO u_Hz    FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'Hz';
    SELECT "Unidad_Id" INTO u_kWh   FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kWh';
    SELECT "Unidad_Id" INTO u_kVarh FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kVarh';
    SELECT "Unidad_Id" INTO u_C     FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = '°C';
    SELECT "Unidad_Id" INTO u_MOhm  FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'MΩ';
    SELECT "Unidad_Id" INTO u_adim  FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'Adimensional';
    SELECT "Unidad_Id" INTO u_Wm2   FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'W/m2';

    -- ── Señales nuevas (UB, UC not in 007/008; per-phase powers for MED_1) ───
    INSERT INTO ssfv."Tbl_Senales"
        ("TipoVar_Id","Unidad_Id","Nombre","Tipo_Valor","Codigo_Senal","Es_Indexada","Es_Alarma","Activo")
    VALUES
        (v_vac, u_V,    'Voltaje Fase B',           'Instantaneo', 'UB',   false, false, true),
        (v_vac, u_V,    'Voltaje Fase C',           'Instantaneo', 'UC',   false, false, true),
        (v_pot, u_kW,   'Potencia Activa Fase A',   'Instantaneo', 'AP_A', false, false, true),
        (v_pot, u_kW,   'Potencia Activa Fase B',   'Instantaneo', 'AP_B', false, false, true),
        (v_pot, u_kW,   'Potencia Activa Fase C',   'Instantaneo', 'AP_C', false, false, true),
        (v_pot, u_kVar, 'Potencia Reactiva Fase A', 'Instantaneo', 'RP_A', false, false, true),
        (v_pot, u_kVar, 'Potencia Reactiva Fase B', 'Instantaneo', 'RP_B', false, false, true),
        (v_pot, u_kVar, 'Potencia Reactiva Fase C', 'Instantaneo', 'RP_C', false, false, true)
    ON CONFLICT ("Codigo_Senal", "TipoVar_Id") DO NOTHING;

    -- ── Lookup señal IDs (existing catalog + newly inserted above) ────────────
    SELECT "Senal_Id" INTO s_IA    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='IA'    AND "TipoVar_Id"=v_cac;
    SELECT "Senal_Id" INTO s_IB    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='IB'    AND "TipoVar_Id"=v_cac;
    SELECT "Senal_Id" INTO s_IC    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='IC'    AND "TipoVar_Id"=v_cac;
    SELECT "Senal_Id" INTO s_UAB   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='UAB'   AND "TipoVar_Id"=v_vac;
    SELECT "Senal_Id" INTO s_UBC   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='UBC'   AND "TipoVar_Id"=v_vac;
    SELECT "Senal_Id" INTO s_UCA   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='UCA'   AND "TipoVar_Id"=v_vac;
    SELECT "Senal_Id" INTO s_UA    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='UA'    AND "TipoVar_Id"=v_vac;
    SELECT "Senal_Id" INTO s_UB    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='UB'    AND "TipoVar_Id"=v_vac;
    SELECT "Senal_Id" INTO s_UC    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='UC'    AND "TipoVar_Id"=v_vac;
    SELECT "Senal_Id" INTO s_AP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='AP'    AND "TipoVar_Id"=v_pot;
    SELECT "Senal_Id" INTO s_RP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='RP'    AND "TipoVar_Id"=v_pot;
    SELECT "Senal_Id" INTO s_SP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='SP'    AND "TipoVar_Id"=v_pot;
    SELECT "Senal_Id" INTO s_FP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='FP'    AND "TipoVar_Id"=v_pro;
    SELECT "Senal_Id" INTO s_EF    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='EF'    AND "TipoVar_Id"=v_pro;
    SELECT "Senal_Id" INTO s_FR    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='FR'    AND "TipoVar_Id"=v_pro;
    SELECT "Senal_Id" INTO s_ET    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='ET'    AND "TipoVar_Id"=v_ene;
    SELECT "Senal_Id" INTO s_IP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='IP'    AND "TipoVar_Id"=v_pot;
    SELECT "Senal_Id" INTO s_T     FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='T'     AND "TipoVar_Id"=v_tem;
    SELECT "Senal_Id" INTO s_IR    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='IR'    AND "TipoVar_Id"=v_pro;
    SELECT "Senal_Id" INTO s_OS    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='OS'    AND "TipoVar_Id"=v_est;
    SELECT "Senal_Id" INTO s_OSV   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='OSV'   AND "TipoVar_Id"=v_est;
    SELECT "Senal_Id" INTO s_IDCx  FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='IDC_x' AND "TipoVar_Id"=v_cdc;
    SELECT "Senal_Id" INTO s_VDCx  FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='VDC_x' AND "TipoVar_Id"=v_vdc;
    SELECT "Senal_Id" INTO s_EFx   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='EF_x'  AND "TipoVar_Id"=v_est;
    SELECT "Senal_Id" INTO s_EVx   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='EV_x'  AND "TipoVar_Id"=v_est;
    SELECT "Senal_Id" INTO s_ALx   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='AL_x'  AND "TipoVar_Id"=v_ala;
    SELECT "Senal_Id" INTO s_ALCOM FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='AL_COM' AND "TipoVar_Id"=v_ala;
    SELECT "Senal_Id" INTO s_RD    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='RD'    AND "TipoVar_Id"=v_irr;
    SELECT "Senal_Id" INTO s_TA    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='TA'    AND "TipoVar_Id"=v_tem;
    SELECT "Senal_Id" INTO s_TP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='TP'    AND "TipoVar_Id"=v_tem;
    SELECT "Senal_Id" INTO s_API   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='API'   AND "TipoVar_Id"=v_ene;
    SELECT "Senal_Id" INTO s_AN    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='AN'    AND "TipoVar_Id"=v_ene;
    SELECT "Senal_Id" INTO s_QPZ   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='QPZ'   AND "TipoVar_Id"=v_ene;
    SELECT "Senal_Id" INTO s_QN    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='QN'    AND "TipoVar_Id"=v_ene;
    SELECT "Senal_Id" INTO s_AP_A  FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='AP_A'  AND "TipoVar_Id"=v_pot;
    SELECT "Senal_Id" INTO s_AP_B  FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='AP_B'  AND "TipoVar_Id"=v_pot;
    SELECT "Senal_Id" INTO s_AP_C  FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='AP_C'  AND "TipoVar_Id"=v_pot;
    SELECT "Senal_Id" INTO s_RP_A  FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='RP_A'  AND "TipoVar_Id"=v_pot;
    SELECT "Senal_Id" INTO s_RP_B  FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='RP_B'  AND "TipoVar_Id"=v_pot;
    SELECT "Senal_Id" INTO s_RP_C  FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal"='RP_C'  AND "TipoVar_Id"=v_pot;

    -- ── Planta: EPM Sede 30 ───────────────────────────────────────────────────
    INSERT INTO ssfv."Tbl_Planta" ("Nombre","Ubicacion","Propietario","Broker_Base","Estado")
    VALUES ('EPM Sede 30','Medellín, Colombia','EPM','EPM/SSFV/EPM/Sede30',1)
    ON CONFLICT ("Broker_Base") DO NOTHING;
    SELECT "Planta_Id" INTO p_sede30
    FROM ssfv."Tbl_Planta" WHERE "Broker_Base"='EPM/SSFV/EPM/Sede30';

    -- ── Equipos ───────────────────────────────────────────────────────────────
    INSERT INTO ssfv."Tbl_Equipo" ("Planta_Id","Tipo_Id","Nombre_Equipo","Nombre_Topic","Estado")
    VALUES (p_sede30,t_inv,'Inversor 1','EPM/SSFV/EPM/Sede30/INV_1',1)
    ON CONFLICT ("Nombre_Topic") DO NOTHING;
    SELECT "Equipo_Id" INTO e_inv1
    FROM ssfv."Tbl_Equipo" WHERE "Nombre_Topic"='EPM/SSFV/EPM/Sede30/INV_1';

    INSERT INTO ssfv."Tbl_Equipo" ("Planta_Id","Tipo_Id","Nombre_Equipo","Nombre_Topic","Estado")
    VALUES (p_sede30,t_med,'Medidor 1','EPM/SSFV/EPM/Sede30/MED_1',1)
    ON CONFLICT ("Nombre_Topic") DO NOTHING;
    SELECT "Equipo_Id" INTO e_med1
    FROM ssfv."Tbl_Equipo" WHERE "Nombre_Topic"='EPM/SSFV/EPM/Sede30/MED_1';

    INSERT INTO ssfv."Tbl_Equipo" ("Planta_Id","Tipo_Id","Nombre_Equipo","Nombre_Topic","Estado")
    VALUES (p_sede30,t_est,'Estación Met 1','EPM/SSFV/EPM/Sede30/EST_1',1)
    ON CONFLICT ("Nombre_Topic") DO NOTHING;
    SELECT "Equipo_Id" INTO e_est1
    FROM ssfv."Tbl_Equipo" WHERE "Nombre_Topic"='EPM/SSFV/EPM/Sede30/EST_1';

    INSERT INTO ssfv."Tbl_Equipo" ("Planta_Id","Tipo_Id","Nombre_Equipo","Nombre_Topic","Estado")
    VALUES (p_sede30,t_fro,'Frontera 20949722','EPM/SSFV/EPM/Sede30/FRONTERA_20949722',1)
    ON CONFLICT ("Nombre_Topic") DO NOTHING;
    SELECT "Equipo_Id" INTO e_fro
    FROM ssfv."Tbl_Equipo" WHERE "Nombre_Topic"='EPM/SSFV/EPM/Sede30/FRONTERA_20949722';

    -- ── Señales x Equipo: INV_1 ───────────────────────────────────────────────
    -- Non-indexed signals (Indice_Canal = NULL).
    -- UNIQUE("Senal_Id","Equipo_Id","Indice_Canal") does not prevent NULL
    -- duplicates, so guard each row with WHERE NOT EXISTS.
    INSERT INTO ssfv."Tbl_Senales_x_Equipo"
        ("Senal_Id","Equipo_Id","Nombre_Instancia","Activo")
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
        SELECT 1 FROM ssfv."Tbl_Senales_x_Equipo" x
        WHERE x."Senal_Id"=v.sid AND x."Equipo_Id"=e_inv1 AND x."Indice_Canal" IS NULL
    );

    -- Indexed signals: IDC, VDC, EF_x, EV_x, AL_x — 3 channels (num_strings=3)
    INSERT INTO ssfv."Tbl_Senales_x_Equipo"
        ("Senal_Id","Equipo_Id","Indice_Canal","Nombre_Instancia","Activo")
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
    INSERT INTO ssfv."Tbl_Senales_x_Equipo"
        ("Senal_Id","Equipo_Id","Nombre_Instancia","Activo")
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
        SELECT 1 FROM ssfv."Tbl_Senales_x_Equipo" x
        WHERE x."Senal_Id"=v.sid AND x."Equipo_Id"=e_med1 AND x."Indice_Canal" IS NULL
    );

    -- ── Señales x Equipo: EST_1 ───────────────────────────────────────────────
    INSERT INTO ssfv."Tbl_Senales_x_Equipo"
        ("Senal_Id","Equipo_Id","Nombre_Instancia","Activo")
    SELECT v.sid, e_est1, v.inst, TRUE
    FROM (VALUES
        (s_RD,'RD'),(s_TA,'TA'),(s_TP,'TP'),(s_ALCOM,'AL_COM')
    ) AS v(sid, inst)
    WHERE NOT EXISTS (
        SELECT 1 FROM ssfv."Tbl_Senales_x_Equipo" x
        WHERE x."Senal_Id"=v.sid AND x."Equipo_Id"=e_est1 AND x."Indice_Canal" IS NULL
    );

    -- ── Señales x Equipo: FRONTERA_20949722 ───────────────────────────────────
    INSERT INTO ssfv."Tbl_Senales_x_Equipo"
        ("Senal_Id","Equipo_Id","Nombre_Instancia","Activo")
    SELECT v.sid, e_fro, v.inst, TRUE
    FROM (VALUES
        (s_API,'API'),(s_AN,'AN'),(s_QN,'QN'),(s_QPZ,'QPZ'),
        (s_IA,'IA'),(s_IB,'IB'),(s_IC,'IC'),
        (s_UA,'UA'),(s_UB,'UB'),(s_UC,'UC')
    ) AS v(sid, inst)
    WHERE NOT EXISTS (
        SELECT 1 FROM ssfv."Tbl_Senales_x_Equipo" x
        WHERE x."Senal_Id"=v.sid AND x."Equipo_Id"=e_fro AND x."Indice_Canal" IS NULL
    );

END $$;
