-- Migration 009: extend the generic señal catalog (per-phase powers + phase B/C
-- voltages). Idempotent: ON CONFLICT DO NOTHING.
--
-- NOTE: this migration originally also seeded a demo plant ("EPM Sede 30") with
-- 4 equipos and all their tbl_senales_x_equipo bindings. That pre-population was
-- removed: under the contract-v3 model nothing is pre-bound — a plant's equipos
-- and their signals are created from the device's NBIRTH/DBIRTH on autodiscovery
-- approval, never seeded ahead of the device. Only the generic catalog señal
-- DEFINITIONS remain here (they carry no equipo binding on their own).
--
-- Databases that already ran the old 009 keep whatever it created (this version
-- only ADDS señales via ON CONFLICT DO NOTHING, so re-applying is a safe no-op).

DO $$
DECLARE
    v_vac  INT; v_pot INT;
    u_V    INT; u_kW  INT; u_kVar INT;
BEGIN
    SELECT tipovar_id INTO v_vac FROM ssfv.tbl_tipo_variable WHERE nombre = 'Voltage AC';
    SELECT tipovar_id INTO v_pot FROM ssfv.tbl_tipo_variable WHERE nombre = 'Potencia';

    SELECT unidad_id INTO u_V    FROM ssfv.tbl_unidades WHERE simbolo = 'V';
    SELECT unidad_id INTO u_kW   FROM ssfv.tbl_unidades WHERE simbolo = 'kW';
    SELECT unidad_id INTO u_kVar FROM ssfv.tbl_unidades WHERE simbolo = 'kVar';

    -- Generic catalog señales (phase B/C voltage, per-phase active/reactive power).
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
    -- Conflict target = the uniqueness in force at THIS migration's point in
    -- the chain: (codigo_senal, tipovar_id). Migration 020 later collapses it to
    -- UNIQUE(codigo_senal); don't reference that here or a fresh install fails.
    ON CONFLICT (codigo_senal, tipovar_id) DO NOTHING;
END $$;
