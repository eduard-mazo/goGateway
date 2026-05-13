-- Migration 009 rollback: remove EPM Sede 30 plant and all its data.
-- Order: Tbl_Valores → Tbl_Alarmas → Tbl_Senales_x_Equipo → Tbl_Equipo → Tbl_Planta → new signals.

DO $$
DECLARE
    eq_ids INT[];
    sx_ids INT[];
BEGIN
    SELECT ARRAY(
        SELECT "Equipo_Id" FROM ssfv."Tbl_Equipo"
        WHERE "Nombre_Topic" IN (
            'EPM/SSFV/EPM/Sede30/INV_1',
            'EPM/SSFV/EPM/Sede30/MED_1',
            'EPM/SSFV/EPM/Sede30/EST_1',
            'EPM/SSFV/EPM/Sede30/FRONTERA_20949722'
        )
    ) INTO eq_ids;

    SELECT ARRAY(
        SELECT "EquiSenal_Id" FROM ssfv."Tbl_Senales_x_Equipo"
        WHERE "Equipo_Id" = ANY(eq_ids)
    ) INTO sx_ids;

    DELETE FROM ssfv."Tbl_Valores"  WHERE "EquiSenal_Id" = ANY(sx_ids);
    DELETE FROM ssfv."Tbl_Alarmas"  WHERE "EquiSenal_Id" = ANY(sx_ids);
    DELETE FROM ssfv."Tbl_Senales_x_Equipo" WHERE "EquiSenal_Id" = ANY(sx_ids);
    DELETE FROM ssfv."Tbl_Equipo"   WHERE "Equipo_Id"    = ANY(eq_ids);
    DELETE FROM ssfv."Tbl_Planta"   WHERE "Broker_Base"  = 'EPM/SSFV/EPM/Sede30';

    DELETE FROM ssfv."Tbl_Senales"
    WHERE "Codigo_Senal" IN ('UB','UC','AP_A','AP_B','AP_C','RP_A','RP_B','RP_C');
END $$;
