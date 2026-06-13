-- Revert nombre_instancia back to VARCHAR(30). Fails if any row exceeds 30 chars.
ALTER TABLE ssfv.tbl_senales_x_equipo
    ALTER COLUMN nombre_instancia TYPE VARCHAR(30);
