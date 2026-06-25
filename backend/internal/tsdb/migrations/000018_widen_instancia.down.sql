-- Revert nombre_instancia to VARCHAR(30) by restoring the catalog typmod.
-- NOTE: this only changes the declared length; it does NOT shorten rows that
-- already store >30 chars, so run it only when no such rows exist.
UPDATE pg_catalog.pg_attribute
SET    atttypmod = 34             -- varchar(30) = 30 + VARHDRSZ(4)
WHERE  attrelid  = 'ssfv.tbl_senales_x_equipo'::regclass
  AND  attname   = 'nombre_instancia'
  AND  atttypmod = 124;
