-- Restore the prior composite uniqueness. The dedup/normalization of codigo_senal
-- is data and is NOT reverted (no information to reconstruct the removed rows).
DROP INDEX IF EXISTS ssfv.uq_senal_codigo;
ALTER TABLE ssfv.tbl_senales
    ADD CONSTRAINT uq_senal_codigo_tipvar UNIQUE (codigo_senal, tipovar_id);
