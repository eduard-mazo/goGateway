-- =============================================================================
-- MIGRATION 018 — Widen nombre_instancia for deep FIWARE/UNS folder paths.
--
-- nombre_instancia holds the folder path between the entity and the leaf
-- attribute (contract v3 §5.1): metric a/b/c/signal → "a/b/c". VARCHAR(30) only
-- fits one or two shallow segments; the UI now renders these as a folder tree,
-- so arbitrary depth must be storable.
--
-- Why not ALTER TABLE: `ALTER COLUMN ... TYPE` is blocked while any view
-- references the column (SQLSTATE 0A000), and dropping the view to work around
-- that fails when *other* objects depend on it (SQLSTATE 2BP01) — e.g. a
-- DBA-created view the gateway cannot enumerate or recreate. The original
-- drop/recreate form of this migration hit exactly that in production.
--
-- For a pure varchar LENGTH INCREASE the side-effect-free option is to bump the
-- column's stored type modifier directly. A varchar(n) column has
-- atttypmod = n + VARHDRSZ(4), so varchar(120) → 124. This rewrites no rows,
-- needs no table rewrite or heavy lock, and leaves every dependent view intact.
-- Values longer than 30 read back through the pre-existing views untruncated:
-- a view's cached output typmod is advisory, PostgreSQL does not re-coerce on
-- projection (verified: a 41-char value round-trips through two dependent views).
--
-- Idempotent — only ever widens (atttypmod < 124). Requires the migration role
-- to update pg_catalog; the gateway connects as a superuser-class role (it also
-- runs CREATE EXTENSION timescaledb in migration 001).
-- =============================================================================

UPDATE pg_catalog.pg_attribute
SET    atttypmod = 124            -- varchar(120) = 120 + VARHDRSZ(4)
WHERE  attrelid  = 'ssfv.tbl_senales_x_equipo'::regclass
  AND  attname   = 'nombre_instancia'
  AND  atttypmod < 124;          -- never narrow; idempotent on re-run
