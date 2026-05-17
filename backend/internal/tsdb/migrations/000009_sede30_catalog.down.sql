-- Migration 009 rollback: remove EPM Sede 30 plant and all its data.
-- Order: tbl_valores → tbl_alarmas → tbl_senales_x_equipo → tbl_equipo → tbl_planta → new signals.

DO $$
DECLARE
    eq_ids INT[];
    sx_ids INT[];
BEGIN
    SELECT ARRAY(
        SELECT equipo_id FROM ssfv.tbl_equipo
        WHERE nombre_topic IN (
            'EPM/SSFV/EPM/Sede30/INV_1',
            'EPM/SSFV/EPM/Sede30/MED_1',
            'EPM/SSFV/EPM/Sede30/EST_1',
            'EPM/SSFV/EPM/Sede30/FRONTERA_20949722'
        )
    ) INTO eq_ids;

    SELECT ARRAY(
        SELECT equisenal_id FROM ssfv.tbl_senales_x_equipo
        WHERE equipo_id = ANY(eq_ids)
    ) INTO sx_ids;

    DELETE FROM ssfv.tbl_valores          WHERE equisenal_id = ANY(sx_ids);
    DELETE FROM ssfv.tbl_alarmas          WHERE equisenal_id = ANY(sx_ids);
    DELETE FROM ssfv.tbl_senales_x_equipo WHERE equisenal_id = ANY(sx_ids);
    DELETE FROM ssfv.tbl_equipo           WHERE equipo_id    = ANY(eq_ids);
    DELETE FROM ssfv.tbl_planta           WHERE broker_base  = 'EPM/SSFV/EPM/Sede30';

    DELETE FROM ssfv.tbl_senales
    WHERE codigo_senal IN ('UB','UC','AP_A','AP_B','AP_C','RP_A','RP_B','RP_C');
END $$;
