-- Seed EPM Gas UNS — GSANRAFA station
-- Run once against gateway.db:
--   sqlite3 gateway.db < data/seed_epm_gas.sql
-- Safe to re-run: INSERT OR REPLACE / INSERT OR IGNORE used throughout.
-- After running, call GET /api/status to verify the mapping count.

-- ── Task 1: MQTT config ───────────────────────────────────────────────────────
INSERT OR REPLACE INTO mqtt_config(
    id, host, port, username, password, client_id,
    use_tls, sparkplug_enabled, sp_group_id, sp_host_id
) VALUES (
    1, 'broker.epm.internal', 1883, '', '', 'goGateway-primary',
    0, 1, 'GAS_EPM', 'goGateway-primary'
);

-- ── Task 2: IEC-104 server GSANRAFA ──────────────────────────────────────────
-- ASDUAddr=1, port=2404, timers per IEC 60870-5-104 §6.9
INSERT OR REPLACE INTO iec104_servers(
    id, name, port, asdu_addr, scada_ips,
    k, w, t0, t1, t2, t3, enabled
) VALUES (
    1, 'GSANRAFA', 2404, 1, '10.114.199.0/24',
    12, 8, 30, 15, 10, 20, 1
);

-- ── Task 3: Device + Topic ────────────────────────────────────────────────────
INSERT OR REPLACE INTO devices(id, server_id, name, description)
VALUES (1, 1, 'GSANRAFA_Am_CalCatal', 'Catalítico Estación GSANRAFA');

-- Topic stored as nodeBase (no msg-type segment) so the cache index key
-- matches topic.NodeBase() from live Sparkplug B messages.
INSERT OR REPLACE INTO topics(id, device_id, topic, qos, enabled)
VALUES (1, 1, 'spBv1.0/GAS_EPM/GSANRAFA', 0, 1);

-- ── Task 4: SignalMappings — 22 signals ──────────────────────────────────────
-- Columns: server_id, topic_id, device_name, variable_type, characteristic,
--          json_key, quality_key, metric_name, iec104_type, ioa, unit,
--          scale, enabled, business, company
--
-- json_key = '' (Sparkplug B, not JSON mode)
-- quality_key = '' (quality from Metric.IsNull via IEC104Quality())
-- scale = 1
-- UNIQUE constraint: (server_id, ioa)

-- metric_name = path estructural completo relativo al nodo GSANRAFA.
-- Debe coincidir exactamente con lo que session.ResolveName() retorna en goMqtt.
-- Lookup key en cache: nodeBase + "\x00" + metric_name

-- ── Módulo 2 ─────────────────────────────────────────────────────────────────
INSERT OR IGNORE INTO signal_mappings(
    server_id,topic_id,device_name,variable_type,characteristic,
    json_key,quality_key,metric_name,iec104_type,ioa,unit,scale,enabled,business,company
) VALUES
(1,1,'GSANRAFA_Am_CalCatal','','', '','','Modulo2/FaTermic', 'M_SP_TB_1',10100,'',   1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','Modulo2/VNOffL',   'M_SP_TB_1',10101,'',   1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','Modulo2/VNOffR',   'M_SP_TB_1',10102,'',   1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','Modulo2/VNOnL',    'M_SP_TB_1',10103,'',   1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','Modulo2/VNOnR',    'M_SP_TB_1',10104,'',   1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','Modulo2/Vmod',     'M_SP_TB_1',10105,'',   1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','Modulo2/PTg',      'M_ME_TF_1',10200,'bar',1,1,'GAS','EPM');

-- ── Módulo 3 ─────────────────────────────────────────────────────────────────
-- "Modulo3/PTg" es distinto de "Modulo2/PTg" — cada uno tiene su propia entry.
INSERT OR IGNORE INTO signal_mappings(
    server_id,topic_id,device_name,variable_type,characteristic,
    json_key,quality_key,metric_name,iec104_type,ioa,unit,scale,enabled,business,company
) VALUES
(1,1,'GSANRAFA_Am_CalCatal','','', '','','Modulo3/PTg',      'M_ME_TF_1',10201,'bar',1,1,'GAS','EPM');

-- ── Am ────────────────────────────────────────────────────────────────────────
INSERT OR IGNORE INTO signal_mappings(
    server_id,topic_id,device_name,variable_type,characteristic,
    json_key,quality_key,metric_name,iec104_type,ioa,unit,scale,enabled,business,company
) VALUES
(1,1,'GSANRAFA_Am_CalCatal','','', '','','Am/Inventm',       'M_ME_TF_1',10300,'m3', 1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','Am/MasaTot',       'M_ME_TF_1',10301,'kg', 1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','Am/TTAux',         'M_ME_TF_1',10302,'°C', 1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','Am/PTg',           'M_ME_TF_1',10303,'bar',1,1,'GAS','EPM');

-- ── CalCatal ──────────────────────────────────────────────────────────────────
INSERT OR IGNORE INTO signal_mappings(
    server_id,topic_id,device_name,variable_type,characteristic,
    json_key,quality_key,metric_name,iec104_type,ioa,unit,scale,enabled,business,company
) VALUES
(1,1,'GSANRAFA_Am_CalCatal','','', '','','CalCatal/VLOCat',  'M_SP_TB_1',10106,'',   1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','CalCatal/VHiCat',  'M_SP_TB_1',10107,'',   1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','CalCatal/CatDer',  'M_SP_TB_1',10108,'',   1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','CalCatal/CatIzq',  'M_SP_TB_1',10109,'',   1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','CalCatal/FaCat',   'M_SP_TB_1',10110,'',   1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','CalCatal/OnResist','M_SP_TB_1',10111,'',   1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','CalCatal/DyIniF',  'M_ME_TF_1',10304,'',   1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','CalCatal/DyOffR',  'M_ME_TF_1',10305,'',   1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','CalCatal/POffCat', 'M_ME_TF_1',10306,'bar',1,1,'GAS','EPM'),
(1,1,'GSANRAFA_Am_CalCatal','','', '','','CalCatal/StHiCat', 'M_ME_TF_1',10307,'°C', 1,1,'GAS','EPM');

-- ── Task 6: TSDB pipeline ─────────────────────────────────────────────────────
INSERT OR REPLACE INTO tsdb_config(
    id, backend, vm_url, vm_username, vm_password,
    ts_dsn, ts_table,
    wal_path, dlq_path,
    batch_size, flush_ms, enabled
) VALUES (
    1, 'both',
    'http://victoria:8428', '', '',
    'postgres://user:pass@tsdb:5432/epm_ot', 'signals',
    'data/wal.bolt', 'data/dlq.bolt',
    2000, 100, 1
);

-- ── Verify ────────────────────────────────────────────────────────────────────
SELECT 'mappings loaded: ' || COUNT(*) FROM signal_mappings WHERE server_id=1;
SELECT 'distinct MetricNames: ' || COUNT(DISTINCT metric_name) FROM signal_mappings WHERE server_id=1;
-- PTg aparece 3 veces (Mod2, Mod3, Am) — correcto por diseño.
SELECT metric_name, iec104_type, ioa FROM signal_mappings WHERE server_id=1 ORDER BY ioa;
