# Deploy runbook — C1 composite-key cutover (migration `000013`)

How to roll out the C1 catalog model (`fa1e2a7`) to a live `industrial_data`
TimescaleDB. C1 changes the SSFV match identity from a single key to the
composite **`(entity nombre_topic, codigo_senal, nombre_instancia)`** and
normalizes flat `nombre_instancia` to `'default'`.

> **One-line summary:** take a DB backup, stop the old gateway, start the C1
> gateway (it applies `000013` on connect), validate, done. Gateway and DB must
> move together — never run an old gateway against a migrated catalog.

---

## 1. What changes

- **Migration `000013_c1_normalize_instance`** runs automatically when the C1
  gateway's TSDB adapter connects. It does, in place:
  ```sql
  UPDATE ssfv.tbl_senales_x_equipo sxe SET nombre_instancia='default'
  FROM ssfv.tbl_senales s
  WHERE s.senal_id=sxe.senal_id AND sxe.indice_canal IS NULL
    AND sxe.nombre_instancia = s.codigo_senal;
  ```
  Flat signals → `'default'`. **Indexed channels** (`indice_canal IS NOT NULL`,
  e.g. solar `IDC_1/2/3`) are **left untouched**. Validated collision-free
  against a copy of this catalog (63 flat normalized, 15 indexed preserved, every
  active signal resolves to exactly one equisenal).
- **Resolution** becomes composite. A flat signal that the catalog stored as
  `nombre_instancia = codigo_senal` now matches as `(entity, codigo, 'default')`.
  The JSON/solar path adapts internally (`IA → (IA, default)`,
  `IDC_1 → (IDC_x, IDC_1)`).
- **Producers: no change required.** They already emit `uns/code` + `uns/instance`
  (Phase 1); legacy producers fall back to consumer-side parsing.

**Why coordinated:** an *old* gateway against a *migrated* DB would look up flat
signals by `nombre_instancia = codigo_senal` and miss (rows are now `'default'`).
Stop every old gateway instance before starting C1.

---

## 2. Pre-deploy (do once, ahead of the window)

1. **Confirm the gateway build** is at/after `fa1e2a7` and the producers at/after
   the Phase 1 `uns/*` commit (`6728795`).
2. **Dry-run on a copy** (already done in development; repeat against current prod
   data if it changed). Read-only dump → throwaway DB → apply the migration →
   assert zero composite collisions:
   ```bash
   docker exec PROD pg_dump -U postgres -d industrial_data --no-owner \
     -t ssfv.tbl_planta -t ssfv.tbl_equipo -t ssfv.tbl_senales \
     -t ssfv.tbl_senales_x_equipo -t ssfv.tbl_tipo_equipo \
     -t ssfv.tbl_tipo_variable -t ssfv.tbl_unidades \
     -t public.tbl_senales_x_tipo_equipo | psql -d copy
   # apply the 000013 UPDATE on `copy`, then:
   SELECT count(*) FROM (
     SELECT e.nombre_topic, s.codigo_senal, sxe.nombre_instancia, count(*) c
     FROM ssfv.tbl_senales_x_equipo sxe
     JOIN ssfv.tbl_senales s USING(senal_id)
     JOIN ssfv.tbl_equipo  e USING(equipo_id)
     WHERE sxe.activo GROUP BY 1,2,3 HAVING count(*)>1) x;   -- MUST be 0
   ```
3. **Schedule a short window.** The migration is metadata-light (one `UPDATE` on a
   small catalog table) — seconds — but treat it as a brief gateway outage.

---

## 3. Cutover

```bash
# 0) Announce a brief ingest pause (Sparkplug LWT/NDEATH will fire; producers buffer/rebirth).

# 1) BACKUP the catalog (rollback anchor).
docker exec PROD pg_dump -U postgres -d industrial_data \
  -t ssfv.tbl_senales_x_equipo -t ssfv.tbl_senales -t ssfv.tbl_equipo \
  > backup_ssfv_$(date +%F_%H%M).sql

# 2) STOP every old gateway instance (all replicas — none may keep running).
systemctl stop gogateway            # or: docker stop / scale to 0

# 3) DEPLOY + START the C1 gateway. On TSDB connect it applies 000013 automatically.
systemctl start gogateway

# 4) WATCH the log for the migration + a clean adapter connect.
journalctl -u gogateway -f | grep -E "applied migration 000013|SSFV adapter connected|mapping cache loaded"
```

Expected:
```
tsdb: applied migration 000013_c1_normalize_instance.up.sql
tsdb: SSFV adapter connected → ssfv.tbl_valores (ON CONFLICT DO NOTHING)
ssfv: mapping cache loaded (N signal entries, M equipos known)
```

> If you prefer a DBA-applied migration instead of auto-apply, run the `000013`
> `UPDATE` manually **after** the old gateway is stopped and **before** starting
> the new one; the runner records it in `schema_migrations` and the gateway skips
> it.

---

## 4. Post-deploy validation (within minutes)

```sql
-- A) flat rows normalized; indexed preserved
SELECT CASE WHEN indice_canal IS NOT NULL THEN 'indexed' ELSE 'flat' END, count(*)
FROM ssfv.tbl_senales_x_equipo GROUP BY 1;          -- flat all 'default'; indexed unchanged

-- B) no ambiguity
SELECT count(*) FROM (SELECT e.nombre_topic,s.codigo_senal,sxe.nombre_instancia,count(*) c
  FROM ssfv.tbl_senales_x_equipo sxe JOIN ssfv.tbl_senales s USING(senal_id)
  JOIN ssfv.tbl_equipo e USING(equipo_id) WHERE sxe.activo GROUP BY 1,2,3 HAVING count(*)>1) x;  -- 0
```

```bash
# C) live ingest resumed for existing signals (counts climbing)
curl -s localhost:8080/api/tsdb/status | jq '.backends[0] | {writeRate, errorRate, circuitOpen}'
psql -d industrial_data -c "SELECT count(*) FROM ssfv.tbl_valores WHERE timestamp_utc > now()-interval '5 min';"

# D) Descartados not flooded (a spike = an unmigrated/mis-resolving signal)
curl -s localhost:8080/api/ssfv/missed | jq 'length'
```

Green = `writeRate > 0`, `errorRate = 0`, `circuitOpen = false`, recent rows
growing, Descartados flat. Spot-check a solar inverter's signals and a host
station's channelized metric (`Network/Rx_MB` instance `docker0`) appear in
`tbl_valores`.

---

## 5. Rollback

The migration is **not** cleanly reversible (the original per-row instance is
lost — all flats became `'default'`), so roll back by **restore + old binary**:

```bash
systemctl stop gogateway
psql -d industrial_data -f backup_ssfv_YYYY-MM-DD_HHMM.sql   # restore pre-cutover catalog
# delete the 000013 marker so a later re-attempt re-runs it:
psql -d industrial_data -c "DELETE FROM schema_migrations WHERE version=13;"
# deploy the previous (C2/pre-C1) gateway binary, then:
systemctl start gogateway-previous
```

Because the catalog tables are small, restore is fast. `tbl_valores` written
during the failed window stays (idempotent `ON CONFLICT DO NOTHING`); no data is
corrupted by a rollback.

---

## 6. Notes

- **HA / multiple replicas:** scale old replicas to **0** before scaling C1 up.
  A rolling deploy that runs both versions against the migrated DB will mis-resolve
  flat signals on the old replicas — do a stop-the-world swap of this one
  component.
- **Approve UI:** after cutover, the Pendientes → Approve dialog pre-fills
  `codigo_senal` + `nombre_instancia` from the producer's `uns/*` (Phase 3); the
  operator confirms instead of typing.
- **`codigo_senal`/`nombre_instancia` are still `VARCHAR(20)/(30)`.** Channelized
  codes fit (`Network/Rx_MB`=13, `docker0`=7); genuinely longer codes are
  quarantined loudly (HTTP 207) at approval, never dropped. Widening remains a
  separate task (blocked by dependent views).
