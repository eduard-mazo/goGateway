-- =============================================================================
-- MIGRATION 007 — Schema ssfv (Sistemas Solares Fotovoltaicos)
-- Applies the full ssfv entity model on top of the existing gateway schema.
-- Idempotent: uses IF NOT EXISTS throughout.
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE SCHEMA IF NOT EXISTS ssfv;

-- ---------------------------------------------------------------------------
-- BLOQUE 1: CATÁLOGOS
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS ssfv."Tbl_Tipo_Equipo" (
    "Tipo_Id"        SERIAL       PRIMARY KEY,
    "Nombre"         VARCHAR(50)  NOT NULL UNIQUE,
    "Descripcion"    VARCHAR(200),
    "Activo"         BOOLEAN      NOT NULL DEFAULT TRUE,
    "Fecha_Creacion" TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    "Fecha_Modif"    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ssfv."Tbl_Tipo_Variable" (
    "TipoVar_Id"     SERIAL       PRIMARY KEY,
    "Nombre"         VARCHAR(60)  NOT NULL UNIQUE,
    "Descripcion"    VARCHAR(200),
    "Activo"         BOOLEAN      NOT NULL DEFAULT TRUE,
    "Fecha_Creacion" TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    "Fecha_Modif"    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ssfv."Tbl_Unidades" (
    "Unidad_Id"      SERIAL       PRIMARY KEY,
    "Simbolo"        VARCHAR(20)  NOT NULL UNIQUE,
    "Nombre"         VARCHAR(60)  NOT NULL,
    "Magnitud"       VARCHAR(60)  NOT NULL,
    "Activo"         BOOLEAN      NOT NULL DEFAULT TRUE,
    "Fecha_Creacion" TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- ---------------------------------------------------------------------------
-- BLOQUE 2: INFRAESTRUCTURA
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS ssfv."Tbl_Planta" (
    "Planta_Id"             SERIAL         PRIMARY KEY,
    "Nombre"                VARCHAR(100)   NOT NULL,
    "Ubicacion"             VARCHAR(150),
    "Propietario"           VARCHAR(100),
    "Broker_Base"           VARCHAR(120)   NOT NULL UNIQUE,
    "Capacidad_kWp"         NUMERIC(10,2),
    "Fecha_Comisionamiento" DATE,
    "Estado"                SMALLINT       NOT NULL DEFAULT 1
                                           CHECK ("Estado" IN (0, 1, 2)),
    "Fecha_Creacion"        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    "Fecha_Modif"           TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ssfv."Tbl_Equipo" (
    "Equipo_Id"     SERIAL        PRIMARY KEY,
    "Planta_Id"     INT           NOT NULL REFERENCES ssfv."Tbl_Planta"("Planta_Id"),
    "Tipo_Id"       INT           NOT NULL REFERENCES ssfv."Tbl_Tipo_Equipo"("Tipo_Id"),
    "Nombre_Equipo" VARCHAR(80)   NOT NULL,
    "Nombre_Topic"  VARCHAR(150)  NOT NULL UNIQUE,
    "Fabricante"    VARCHAR(80),
    "Modelo"        VARCHAR(80),
    "Nro_Serie"     VARCHAR(60),
    "Estado"        SMALLINT      NOT NULL DEFAULT 1
                                  CHECK ("Estado" IN (0, 1, 2)),
    "Fecha_Creacion" TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    "Fecha_Modif"   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_equipo_planta_nombre UNIQUE ("Planta_Id", "Nombre_Equipo")
);

CREATE INDEX IF NOT EXISTS idx_equipo_planta ON ssfv."Tbl_Equipo"("Planta_Id");
CREATE INDEX IF NOT EXISTS idx_equipo_tipo   ON ssfv."Tbl_Equipo"("Tipo_Id");

CREATE TABLE IF NOT EXISTS ssfv."Tbl_Frontera_Comercial" (
    "Frontera_Id"   SERIAL       PRIMARY KEY,
    "Planta_Id"     INT          NOT NULL REFERENCES ssfv."Tbl_Planta"("Planta_Id"),
    "Codigo_NIE"    VARCHAR(20)  NOT NULL UNIQUE,
    "Nombre"        VARCHAR(100) NOT NULL,
    "Tipo_Conexion" VARCHAR(40),
    "Activo"        BOOLEAN      NOT NULL DEFAULT TRUE,
    "Fecha_Creacion" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "Fecha_Modif"   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_frontera_planta ON ssfv."Tbl_Frontera_Comercial"("Planta_Id");

-- ---------------------------------------------------------------------------
-- BLOQUE 3: CATÁLOGO DE SEÑALES
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS ssfv."Tbl_Senales" (
    "Senal_Id"      SERIAL       PRIMARY KEY,
    "TipoVar_Id"    INT          NOT NULL REFERENCES ssfv."Tbl_Tipo_Variable"("TipoVar_Id"),
    "Unidad_Id"     INT          NOT NULL REFERENCES ssfv."Tbl_Unidades"("Unidad_Id"),
    "Nombre"        VARCHAR(80)  NOT NULL,
    "Descripcion"   VARCHAR(200),
    "Tipo_Valor"    VARCHAR(20)  NOT NULL
                                 CHECK ("Tipo_Valor" IN ('Instantaneo', 'Acumulado')),
    "Codigo_Senal"  VARCHAR(20)  NOT NULL,
    "Es_Indexada"   BOOLEAN      NOT NULL DEFAULT FALSE,
    "Activo"        BOOLEAN      NOT NULL DEFAULT TRUE,
    "Fecha_Alta"    DATE         NOT NULL DEFAULT CURRENT_DATE,

    CONSTRAINT uq_senal_codigo_tipvar UNIQUE ("Codigo_Senal", "TipoVar_Id")
);

CREATE INDEX IF NOT EXISTS idx_senales_tipvar ON ssfv."Tbl_Senales"("TipoVar_Id");
CREATE INDEX IF NOT EXISTS idx_senales_unidad ON ssfv."Tbl_Senales"("Unidad_Id");

CREATE TABLE IF NOT EXISTS ssfv."Tbl_Senales_x_Equipo" (
    "EquiSenal_Id"    SERIAL      PRIMARY KEY,
    "Senal_Id"        INT         NOT NULL REFERENCES ssfv."Tbl_Senales"("Senal_Id"),
    "Equipo_Id"       INT         NOT NULL REFERENCES ssfv."Tbl_Equipo"("Equipo_Id"),
    "Indice_Canal"    SMALLINT,
    "Nombre_Instancia" VARCHAR(30) NOT NULL,
    "Activo"          BOOLEAN     NOT NULL DEFAULT TRUE,
    "Fecha_Alta"      DATE        NOT NULL DEFAULT CURRENT_DATE,

    CONSTRAINT uq_senal_equipo_canal UNIQUE ("Senal_Id", "Equipo_Id", "Indice_Canal"),
    CONSTRAINT chk_indice_canal CHECK (
        ("Indice_Canal" IS NULL) OR ("Indice_Canal" >= 1)
    )
);

CREATE INDEX IF NOT EXISTS idx_sxe_senal  ON ssfv."Tbl_Senales_x_Equipo"("Senal_Id");
CREATE INDEX IF NOT EXISTS idx_sxe_equipo ON ssfv."Tbl_Senales_x_Equipo"("Equipo_Id");

-- ---------------------------------------------------------------------------
-- BLOQUE 4: SERIES TEMPORALES (hypertables)
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS ssfv."Tbl_Valores" (
    "Timestamp_UTC" TIMESTAMPTZ  NOT NULL,
    "EquiSenal_Id"  INT          NOT NULL REFERENCES ssfv."Tbl_Senales_x_Equipo"("EquiSenal_Id"),
    "Valor"         NUMERIC(18,6),
    "Calidad"       VARCHAR(10)  NOT NULL DEFAULT 'Buena'
                                 CHECK ("Calidad" IN ('Buena', 'Dudosa', 'Mala')),

    CONSTRAINT pk_valores PRIMARY KEY ("Timestamp_UTC", "EquiSenal_Id")
);

SELECT create_hypertable(
    'ssfv."Tbl_Valores"',
    'Timestamp_UTC',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_valores_equisenal_time
    ON ssfv."Tbl_Valores"("EquiSenal_Id", "Timestamp_UTC" DESC);

DO $$
BEGIN
    ALTER TABLE ssfv."Tbl_Valores" SET (
        timescaledb.compress,
        timescaledb.compress_orderby     = '"Timestamp_UTC" DESC',
        timescaledb.compress_segmentby   = '"EquiSenal_Id"'
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $$;

DO $$
BEGIN
    PERFORM add_compression_policy('ssfv."Tbl_Valores"', compress_after => INTERVAL '7 days');
EXCEPTION WHEN OTHERS THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS ssfv."Tbl_Alarmas" (
    "Alarma_Id"     BIGSERIAL,
    "EquiSenal_Id"  INT          NOT NULL REFERENCES ssfv."Tbl_Senales_x_Equipo"("EquiSenal_Id"),
    "Ts_Inicio"     TIMESTAMPTZ  NOT NULL,
    "Ts_Fin"        TIMESTAMPTZ,
    "Tipo_Alarma"   VARCHAR(20)  NOT NULL
                                 CHECK ("Tipo_Alarma" IN ('Dispositivo','Comunicacion','Proceso','Fabricante')),
    "Descripcion"   VARCHAR(300),
    "Severidad"     VARCHAR(10)  NOT NULL DEFAULT 'Media'
                                 CHECK ("Severidad" IN ('Critica','Alta','Media','Baja')),
    "Activa"        BOOLEAN      NOT NULL DEFAULT TRUE,

    CONSTRAINT pk_alarmas PRIMARY KEY ("Alarma_Id", "Ts_Inicio")
);

SELECT create_hypertable(
    'ssfv."Tbl_Alarmas"',
    'Ts_Inicio',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_alarmas_equisenal_time
    ON ssfv."Tbl_Alarmas"("EquiSenal_Id", "Ts_Inicio" DESC);

CREATE INDEX IF NOT EXISTS idx_alarmas_activas
    ON ssfv."Tbl_Alarmas"("Activa", "Ts_Inicio" DESC)
    WHERE "Activa" = TRUE;

DO $$
BEGIN
    ALTER TABLE ssfv."Tbl_Alarmas" SET (
        timescaledb.compress,
        timescaledb.compress_orderby   = '"Ts_Inicio" DESC',
        timescaledb.compress_segmentby = '"EquiSenal_Id"'
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $$;

DO $$
BEGIN
    PERFORM add_compression_policy('ssfv."Tbl_Alarmas"', compress_after => INTERVAL '30 days');
EXCEPTION WHEN OTHERS THEN NULL;
END $$;

-- Tabla raw: fallback para señales sin EquiSenal_Id mapeado.
CREATE TABLE IF NOT EXISTS ssfv.signals_raw (
    ts          TIMESTAMPTZ  NOT NULL,
    signal_path TEXT         NOT NULL,
    signal      TEXT,
    value       DOUBLE PRECISION,
    quality     SMALLINT     DEFAULT 0,
    tags        JSONB        DEFAULT '{}'::jsonb,
    PRIMARY KEY (ts, signal_path)
);

SELECT create_hypertable(
    'ssfv.signals_raw',
    'ts',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_signals_raw_path_time
    ON ssfv.signals_raw(signal_path, ts DESC);

-- ---------------------------------------------------------------------------
-- BLOQUE 5: VISTAS
-- ---------------------------------------------------------------------------

CREATE OR REPLACE VIEW ssfv.v_Senales_Contexto AS
SELECT
    sxe."EquiSenal_Id",
    sxe."Nombre_Instancia",
    sxe."Indice_Canal",
    sxe."Activo"                  AS "Senal_Activa",
    s."Senal_Id",
    s."Nombre"                    AS "Senal_Nombre",
    s."Codigo_Senal",
    s."Tipo_Valor",
    s."Es_Indexada",
    tv."Nombre"                   AS "Tipo_Variable",
    u."Simbolo"                   AS "Unidad",
    u."Magnitud",
    e."Equipo_Id",
    e."Nombre_Equipo",
    e."Nombre_Topic",
    e."Fabricante",
    e."Modelo",
    te."Nombre"                   AS "Tipo_Equipo",
    p."Planta_Id",
    p."Nombre"                    AS "Planta_Nombre",
    p."Broker_Base"
FROM ssfv."Tbl_Senales_x_Equipo"  sxe
JOIN ssfv."Tbl_Senales"            s   ON s."Senal_Id"    = sxe."Senal_Id"
JOIN ssfv."Tbl_Tipo_Variable"      tv  ON tv."TipoVar_Id" = s."TipoVar_Id"
JOIN ssfv."Tbl_Unidades"           u   ON u."Unidad_Id"   = s."Unidad_Id"
JOIN ssfv."Tbl_Equipo"             e   ON e."Equipo_Id"   = sxe."Equipo_Id"
JOIN ssfv."Tbl_Tipo_Equipo"        te  ON te."Tipo_Id"    = e."Tipo_Id"
JOIN ssfv."Tbl_Planta"             p   ON p."Planta_Id"   = e."Planta_Id";

CREATE OR REPLACE VIEW ssfv.v_Ultimas_Lecturas AS
SELECT DISTINCT ON ("EquiSenal_Id")
    v."EquiSenal_Id",
    v."Timestamp_UTC",
    v."Valor",
    v."Calidad"
FROM ssfv."Tbl_Valores" v
ORDER BY "EquiSenal_Id", "Timestamp_UTC" DESC;

CREATE OR REPLACE VIEW ssfv.v_Alarmas_Activas AS
SELECT
    a."Alarma_Id",
    a."Ts_Inicio",
    a."Tipo_Alarma",
    a."Severidad",
    a."Descripcion",
    sc."Planta_Nombre",
    sc."Nombre_Equipo",
    sc."Tipo_Equipo",
    sc."Nombre_Instancia",
    sc."Tipo_Variable"
FROM ssfv."Tbl_Alarmas"        a
JOIN ssfv.v_Senales_Contexto   sc ON sc."EquiSenal_Id" = a."EquiSenal_Id"
WHERE a."Activa" = TRUE
ORDER BY a."Severidad" DESC, a."Ts_Inicio" DESC;

-- ---------------------------------------------------------------------------
-- BLOQUE 6: DATOS MAESTROS INICIALES
-- ---------------------------------------------------------------------------

INSERT INTO ssfv."Tbl_Tipo_Equipo" ("Nombre", "Descripcion") VALUES
    ('Inversor',               'Convertidor DC/AC de paneles solares'),
    ('Medidor',                'Medidor de energía eléctrica'),
    ('Estación Meteorológica', 'Estación de variables ambientales'),
    ('Frontera Comercial',     'Medidor de frontera con la red eléctrica')
ON CONFLICT ("Nombre") DO NOTHING;

INSERT INTO ssfv."Tbl_Tipo_Variable" ("Nombre", "Descripcion") VALUES
    ('Corriente AC',   'Corriente alterna'),
    ('Corriente DC',   'Corriente continua DC string'),
    ('Voltage AC',     'Tensión alterna'),
    ('Voltage DC',     'Tensión DC string'),
    ('Potencia',       'Potencia activa, reactiva o aparente'),
    ('Energía',        'Energía acumulada activa o reactiva'),
    ('Proceso',        'Variables de proceso: eficiencia, frecuencia, factor de potencia'),
    ('Temperatura',    'Temperatura de inversor, ambiente o panel'),
    ('Irradiancia',    'Irradiancia solar W/m²'),
    ('Estado',         'Estado de operación entero o booleano'),
    ('Alarma',         'Alarma de dispositivo o comunicación')
ON CONFLICT ("Nombre") DO NOTHING;

INSERT INTO ssfv."Tbl_Unidades" ("Simbolo", "Nombre", "Magnitud") VALUES
    ('A',            'Amperio',              'Corriente'),
    ('V',            'Voltio',               'Tensión'),
    ('kW',           'Kilovatio',            'Potencia'),
    ('kVar',         'Kilovoltamperio reactivo', 'Potencia reactiva'),
    ('kVA',          'Kilovoltamperio',      'Potencia aparente'),
    ('kWh',          'Kilovatio hora',       'Energía'),
    ('kVarh',        'Kilovoltamperio reactivo hora', 'Energía reactiva'),
    ('%',            'Porcentaje',           'Proceso'),
    ('Hz',           'Hercio',               'Frecuencia'),
    ('°C',           'Grado Celsius',        'Temperatura'),
    ('W/m2',         'Vatio por metro cuadrado', 'Irradiancia'),
    ('MΩ',           'Megaohmio',            'Resistencia'),
    ('Adimensional', 'Sin unidad',           'Estado/Proceso')
ON CONFLICT ("Simbolo") DO NOTHING;

-- ---------------------------------------------------------------------------
-- BLOQUE 7: Columna Es_Alarma en Tbl_Senales (idempotente)
-- ---------------------------------------------------------------------------

DO $$
BEGIN
    ALTER TABLE ssfv."Tbl_Senales" ADD COLUMN "Es_Alarma" BOOLEAN NOT NULL DEFAULT FALSE;
EXCEPTION WHEN duplicate_column THEN NULL;
END $$;

-- ---------------------------------------------------------------------------
-- BLOQUE 8: Tabla de junctions Tipo_Equipo ↔ Señales (catálogo de señales por tipo)
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS ssfv."Tbl_Senales_x_Tipo_Equipo" (
    "SenalTipo_Id" SERIAL   PRIMARY KEY,
    "Senal_Id"     INT      NOT NULL REFERENCES ssfv."Tbl_Senales"("Senal_Id"),
    "Tipo_Id"      INT      NOT NULL REFERENCES ssfv."Tbl_Tipo_Equipo"("Tipo_Id"),
    "Num_Canales"  SMALLINT NOT NULL DEFAULT 1,
    CONSTRAINT uq_senal_tipo_equipo UNIQUE ("Senal_Id", "Tipo_Id")
);

-- ---------------------------------------------------------------------------
-- BLOQUE 9: Catálogo completo de señales SSFV + asignaciones por tipo de equipo
-- ---------------------------------------------------------------------------

DO $$
DECLARE
    -- Tipo Variable IDs
    v_cac INT; v_cdc INT; v_vac INT; v_vdc INT;
    v_pot INT; v_ene INT; v_pro INT; v_tem INT;
    v_irr INT; v_est INT; v_ala INT;
    -- Unidad IDs
    u_A    INT; u_V  INT; u_kW   INT; u_kVar INT; u_kVA  INT;
    u_kWh  INT; u_kVarh INT; u_pct INT; u_Hz   INT;
    u_C    INT; u_Wm2 INT; u_MOhm INT; u_adim INT;
    -- Tipo Equipo IDs
    t_inv INT; t_med INT; t_est INT; t_fro INT;
    -- Señal IDs (para junction table)
    s_IA    INT; s_IB    INT; s_IC   INT;
    s_UAB   INT; s_UBC   INT; s_UCA  INT;
    s_AP    INT; s_RP    INT; s_SP   INT;
    s_FP    INT; s_EF    INT; s_FR   INT;
    s_ET    INT; s_IP    INT; s_T    INT;
    s_IR    INT; s_OS    INT; s_OSV  INT;
    s_IDCx  INT; s_VDCx  INT; s_EFx  INT;
    s_EVx   INT; s_ALx   INT; s_ALCOM INT;
    s_RD    INT; s_TA    INT; s_TP   INT;
    s_UA    INT; s_API   INT; s_AN   INT;
    s_QPZ   INT; s_QN    INT;
BEGIN
    -- Lookup tipo_variable IDs
    SELECT "TipoVar_Id" INTO v_cac FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Corriente AC';
    SELECT "TipoVar_Id" INTO v_cdc FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Corriente DC';
    SELECT "TipoVar_Id" INTO v_vac FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Voltage AC';
    SELECT "TipoVar_Id" INTO v_vdc FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Voltage DC';
    SELECT "TipoVar_Id" INTO v_pot FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Potencia';
    SELECT "TipoVar_Id" INTO v_ene FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Energía';
    SELECT "TipoVar_Id" INTO v_pro FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Proceso';
    SELECT "TipoVar_Id" INTO v_tem FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Temperatura';
    SELECT "TipoVar_Id" INTO v_irr FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Irradiancia';
    SELECT "TipoVar_Id" INTO v_est FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Estado';
    SELECT "TipoVar_Id" INTO v_ala FROM ssfv."Tbl_Tipo_Variable" WHERE "Nombre" = 'Alarma';

    -- Lookup unidad IDs
    SELECT "Unidad_Id" INTO u_A     FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'A';
    SELECT "Unidad_Id" INTO u_V     FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'V';
    SELECT "Unidad_Id" INTO u_kW    FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kW';
    SELECT "Unidad_Id" INTO u_kVar  FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kVar';
    SELECT "Unidad_Id" INTO u_kVA   FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kVA';
    SELECT "Unidad_Id" INTO u_kWh   FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kWh';
    SELECT "Unidad_Id" INTO u_kVarh FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'kVarh';
    SELECT "Unidad_Id" INTO u_pct   FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = '%';
    SELECT "Unidad_Id" INTO u_Hz    FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'Hz';
    SELECT "Unidad_Id" INTO u_C     FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = '°C';
    SELECT "Unidad_Id" INTO u_Wm2   FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'W/m2';
    SELECT "Unidad_Id" INTO u_MOhm  FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'MΩ';
    SELECT "Unidad_Id" INTO u_adim  FROM ssfv."Tbl_Unidades" WHERE "Simbolo" = 'Adimensional';

    -- Lookup tipo_equipo IDs
    SELECT "Tipo_Id" INTO t_inv FROM ssfv."Tbl_Tipo_Equipo" WHERE "Nombre" = 'Inversor';
    SELECT "Tipo_Id" INTO t_med FROM ssfv."Tbl_Tipo_Equipo" WHERE "Nombre" = 'Medidor';
    SELECT "Tipo_Id" INTO t_est FROM ssfv."Tbl_Tipo_Equipo" WHERE "Nombre" = 'Estación Meteorológica';
    SELECT "Tipo_Id" INTO t_fro FROM ssfv."Tbl_Tipo_Equipo" WHERE "Nombre" = 'Frontera Comercial';

    -- ── Insertar señales de catálogo (ON CONFLICT DO NOTHING = idempotente) ──

    -- Inversor / compartidas
    INSERT INTO ssfv."Tbl_Senales"
        ("TipoVar_Id","Unidad_Id","Nombre","Tipo_Valor","Codigo_Senal","Es_Indexada","Es_Alarma","Activo")
    VALUES
        (v_cac, u_A,    'Corriente Fase A AC',        'Instantaneo', 'IA',     false, false, true),
        (v_cac, u_A,    'Corriente Fase B AC',        'Instantaneo', 'IB',     false, false, true),
        (v_cac, u_A,    'Corriente Fase C AC',        'Instantaneo', 'IC',     false, false, true),
        (v_vac, u_V,    'Voltaje Línea AB',           'Instantaneo', 'UAB',    false, false, true),
        (v_vac, u_V,    'Voltaje Línea BC',           'Instantaneo', 'UBC',    false, false, true),
        (v_vac, u_V,    'Voltaje Línea CA',           'Instantaneo', 'UCA',    false, false, true),
        (v_pot, u_kW,   'Potencia Activa',            'Instantaneo', 'AP',     false, false, true),
        (v_pot, u_kVar, 'Potencia Reactiva',          'Instantaneo', 'RP',     false, false, true),
        (v_pot, u_kVA,  'Potencia Aparente',          'Instantaneo', 'SP',     false, false, true),
        (v_pro, u_adim, 'Factor de Potencia',         'Instantaneo', 'FP',     false, false, true),
        (v_pro, u_pct,  'Eficiencia Inversor',        'Instantaneo', 'EF',     false, false, true),
        (v_pro, u_Hz,   'Frecuencia Red',             'Instantaneo', 'FR',     false, false, true),
        (v_ene, u_kWh,  'Energía Acumulada',          'Acumulado',   'ET',     false, false, true),
        (v_pot, u_kW,   'Potencia Entrada DC',        'Instantaneo', 'IP',     false, false, true),
        (v_tem, u_C,    'Temperatura Inversor',       'Instantaneo', 'T',      false, false, true),
        (v_pro, u_MOhm, 'Resistencia Aislamiento',   'Instantaneo', 'IR',     false, false, true),
        (v_est, u_adim, 'Estado Operación',           'Instantaneo', 'OS',     false, false, true),
        (v_est, u_adim, 'OS Fabricante',              'Instantaneo', 'OSV',    false, false, true),
        (v_cdc, u_A,    'Corriente DC String',        'Instantaneo', 'IDC_x',  true,  false, true),
        (v_vdc, u_V,    'Voltaje DC String',          'Instantaneo', 'VDC_x',  true,  false, true),
        (v_est, u_adim, 'Alarma Dispositivo EF',      'Instantaneo', 'EF_x',   true,  true,  true),
        (v_est, u_adim, 'Alarma Fabricante EV',       'Instantaneo', 'EV_x',   true,  true,  true),
        (v_ala, u_adim, 'Alarma Dispositivo AL',      'Instantaneo', 'AL_x',   true,  true,  true),
        (v_ala, u_adim, 'Alarma Comunicación',        'Instantaneo', 'AL_COM', false, true,  true),
        -- Estación Meteorológica
        (v_irr, u_Wm2,  'Irradiancia Principal',     'Instantaneo', 'RD',     false, false, true),
        (v_tem, u_C,    'Temperatura Ambiente',       'Instantaneo', 'TA',     false, false, true),
        (v_tem, u_C,    'Temperatura Panel',          'Instantaneo', 'TP',     false, false, true),
        -- Medidor / Frontera Comercial
        (v_vac, u_V,    'Voltaje Fase A',             'Instantaneo', 'UA',     false, false, true),
        (v_ene, u_kWh,  'Energía Activa Importada',  'Acumulado',   'API',    false, false, true),
        (v_ene, u_kWh,  'Energía Activa Exportada',  'Acumulado',   'AN',     false, false, true),
        (v_ene, u_kVarh,'Energía Reactiva Importada','Acumulado',   'QPZ',    false, false, true),
        (v_ene, u_kVarh,'Energía Reactiva Exportada','Acumulado',   'QN',     false, false, true)
    ON CONFLICT ("Codigo_Senal", "TipoVar_Id") DO NOTHING;

    -- ── Lookup IDs de señales recién insertadas ──
    SELECT "Senal_Id" INTO s_IA    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'IA'     AND "TipoVar_Id" = v_cac;
    SELECT "Senal_Id" INTO s_IB    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'IB'     AND "TipoVar_Id" = v_cac;
    SELECT "Senal_Id" INTO s_IC    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'IC'     AND "TipoVar_Id" = v_cac;
    SELECT "Senal_Id" INTO s_UAB   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'UAB'    AND "TipoVar_Id" = v_vac;
    SELECT "Senal_Id" INTO s_UBC   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'UBC'    AND "TipoVar_Id" = v_vac;
    SELECT "Senal_Id" INTO s_UCA   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'UCA'    AND "TipoVar_Id" = v_vac;
    SELECT "Senal_Id" INTO s_AP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'AP'     AND "TipoVar_Id" = v_pot;
    SELECT "Senal_Id" INTO s_RP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'RP'     AND "TipoVar_Id" = v_pot;
    SELECT "Senal_Id" INTO s_SP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'SP'     AND "TipoVar_Id" = v_pot;
    SELECT "Senal_Id" INTO s_FP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'FP'     AND "TipoVar_Id" = v_pro;
    SELECT "Senal_Id" INTO s_EF    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'EF'     AND "TipoVar_Id" = v_pro;
    SELECT "Senal_Id" INTO s_FR    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'FR'     AND "TipoVar_Id" = v_pro;
    SELECT "Senal_Id" INTO s_ET    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'ET'     AND "TipoVar_Id" = v_ene;
    SELECT "Senal_Id" INTO s_IP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'IP'     AND "TipoVar_Id" = v_pot;
    SELECT "Senal_Id" INTO s_T     FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'T'      AND "TipoVar_Id" = v_tem;
    SELECT "Senal_Id" INTO s_IR    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'IR'     AND "TipoVar_Id" = v_pro;
    SELECT "Senal_Id" INTO s_OS    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'OS'     AND "TipoVar_Id" = v_est;
    SELECT "Senal_Id" INTO s_OSV   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'OSV'    AND "TipoVar_Id" = v_est;
    SELECT "Senal_Id" INTO s_IDCx  FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'IDC_x'  AND "TipoVar_Id" = v_cdc;
    SELECT "Senal_Id" INTO s_VDCx  FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'VDC_x'  AND "TipoVar_Id" = v_vdc;
    SELECT "Senal_Id" INTO s_EFx   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'EF_x'   AND "TipoVar_Id" = v_est;
    SELECT "Senal_Id" INTO s_EVx   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'EV_x'   AND "TipoVar_Id" = v_est;
    SELECT "Senal_Id" INTO s_ALx   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'AL_x'   AND "TipoVar_Id" = v_ala;
    SELECT "Senal_Id" INTO s_ALCOM FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'AL_COM' AND "TipoVar_Id" = v_ala;
    SELECT "Senal_Id" INTO s_RD    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'RD'     AND "TipoVar_Id" = v_irr;
    SELECT "Senal_Id" INTO s_TA    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'TA'     AND "TipoVar_Id" = v_tem;
    SELECT "Senal_Id" INTO s_TP    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'TP'     AND "TipoVar_Id" = v_tem;
    SELECT "Senal_Id" INTO s_UA    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'UA'     AND "TipoVar_Id" = v_vac;
    SELECT "Senal_Id" INTO s_API   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'API'    AND "TipoVar_Id" = v_ene;
    SELECT "Senal_Id" INTO s_AN    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'AN'     AND "TipoVar_Id" = v_ene;
    SELECT "Senal_Id" INTO s_QPZ   FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'QPZ'    AND "TipoVar_Id" = v_ene;
    SELECT "Senal_Id" INTO s_QN    FROM ssfv."Tbl_Senales" WHERE "Codigo_Senal" = 'QN'     AND "TipoVar_Id" = v_ene;

    -- ── Junction Inversor ──
    INSERT INTO ssfv."Tbl_Senales_x_Tipo_Equipo" ("Senal_Id","Tipo_Id","Num_Canales")
    VALUES
        (s_IA, t_inv, 1), (s_IB, t_inv, 1), (s_IC, t_inv, 1),
        (s_UAB, t_inv, 1), (s_UBC, t_inv, 1), (s_UCA, t_inv, 1),
        (s_AP, t_inv, 1), (s_RP, t_inv, 1), (s_SP, t_inv, 1),
        (s_FP, t_inv, 1), (s_EF, t_inv, 1), (s_FR, t_inv, 1),
        (s_ET, t_inv, 1), (s_IP, t_inv, 1), (s_T, t_inv, 1),
        (s_IR, t_inv, 1), (s_OS, t_inv, 1), (s_OSV, t_inv, 1),
        (s_IDCx, t_inv, 3),   -- 3 string DC channels
        (s_VDCx, t_inv, 3),
        (s_EFx,  t_inv, 1),
        (s_EVx,  t_inv, 1),
        (s_ALx,  t_inv, 1),
        (s_ALCOM, t_inv, 1)
    ON CONFLICT DO NOTHING;

    -- ── Junction Medidor ──
    INSERT INTO ssfv."Tbl_Senales_x_Tipo_Equipo" ("Senal_Id","Tipo_Id","Num_Canales")
    VALUES
        (s_UA, t_med, 1), (s_UAB, t_med, 1), (s_UBC, t_med, 1), (s_UCA, t_med, 1),
        (s_IA, t_med, 1), (s_IB, t_med, 1), (s_IC, t_med, 1),
        (s_AP, t_med, 1), (s_RP, t_med, 1), (s_SP, t_med, 1), (s_FP, t_med, 1),
        (s_ET, t_med, 1),
        (s_API, t_med, 1), (s_AN, t_med, 1), (s_QPZ, t_med, 1), (s_QN, t_med, 1),
        (s_ALCOM, t_med, 1)
    ON CONFLICT DO NOTHING;

    -- ── Junction Estación Meteorológica ──
    INSERT INTO ssfv."Tbl_Senales_x_Tipo_Equipo" ("Senal_Id","Tipo_Id","Num_Canales")
    VALUES
        (s_RD, t_est, 1), (s_TA, t_est, 1), (s_TP, t_est, 1), (s_ALCOM, t_est, 1)
    ON CONFLICT DO NOTHING;

    -- ── Junction Frontera Comercial ──
    INSERT INTO ssfv."Tbl_Senales_x_Tipo_Equipo" ("Senal_Id","Tipo_Id","Num_Canales")
    VALUES
        (s_API, t_fro, 1), (s_AN, t_fro, 1), (s_QPZ, t_fro, 1), (s_QN, t_fro, 1),
        (s_IA,  t_fro, 1), (s_UAB, t_fro, 1), (s_ALCOM, t_fro, 1)
    ON CONFLICT DO NOTHING;
END $$;

-- ---------------------------------------------------------------------------
-- BLOQUE 10: Agregados continuos (15 min y diario) — TimescaleDB >= 2.x
-- ---------------------------------------------------------------------------

DO $$
BEGIN
    CREATE MATERIALIZED VIEW ssfv.mv_valores_15min
    WITH (timescaledb.continuous) AS
    SELECT
        time_bucket('15 minutes', "Timestamp_UTC") AS bucket,
        "EquiSenal_Id",
        AVG("Valor")  AS avg_val,
        MIN("Valor")  AS min_val,
        MAX("Valor")  AS max_val,
        COUNT(*)      AS n_total,
        COUNT(*) FILTER (WHERE "Calidad" = 'Buena') AS n_validas
    FROM ssfv."Tbl_Valores"
    GROUP BY bucket, "EquiSenal_Id";
EXCEPTION WHEN OTHERS THEN NULL;  -- view may already exist
END $$;

DO $$
BEGIN
    PERFORM add_continuous_aggregate_policy('ssfv.mv_valores_15min',
        start_offset     => INTERVAL '2 hours',
        end_offset       => INTERVAL '15 minutes',
        schedule_interval => INTERVAL '15 minutes');
EXCEPTION WHEN OTHERS THEN NULL;
END $$;

DO $$
BEGIN
    CREATE MATERIALIZED VIEW ssfv.mv_valores_diario
    WITH (timescaledb.continuous) AS
    SELECT
        time_bucket('1 day', "Timestamp_UTC") AS bucket,
        "EquiSenal_Id",
        AVG("Valor")  AS avg_val,
        MIN("Valor")  AS min_val,
        MAX("Valor")  AS max_val,
        COUNT(*) FILTER (WHERE "Calidad" = 'Buena') AS n_validas
    FROM ssfv."Tbl_Valores"
    GROUP BY bucket, "EquiSenal_Id";
EXCEPTION WHEN OTHERS THEN NULL;
END $$;

DO $$
BEGIN
    PERFORM add_continuous_aggregate_policy('ssfv.mv_valores_diario',
        start_offset     => INTERVAL '3 days',
        end_offset       => INTERVAL '1 day',
        schedule_interval => INTERVAL '1 hour');
EXCEPTION WHEN OTHERS THEN NULL;
END $$;
