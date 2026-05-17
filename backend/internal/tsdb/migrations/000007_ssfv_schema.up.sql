-- =============================================================================
-- MIGRATION 007 — Schema ssfv (Sistemas Solares Fotovoltaicos)
-- Table definitions match schema_gio4db.sql exactly (lowercase snake_case).
-- Idempotent: uses IF NOT EXISTS throughout.
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE SCHEMA IF NOT EXISTS ssfv;

-- ---------------------------------------------------------------------------
-- BLOQUE 1: CATÁLOGOS
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS ssfv.tbl_tipo_equipo (
    tipo_id        SERIAL       PRIMARY KEY,
    nombre         VARCHAR(50)  NOT NULL UNIQUE,
    descripcion    VARCHAR(200),
    activo         BOOLEAN      NOT NULL DEFAULT TRUE,
    fecha_creacion TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    fecha_modif    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ssfv.tbl_tipo_variable (
    tipovar_id     SERIAL       PRIMARY KEY,
    nombre         VARCHAR(60)  NOT NULL UNIQUE,
    descripcion    VARCHAR(200),
    activo         BOOLEAN      NOT NULL DEFAULT TRUE,
    fecha_creacion TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    fecha_modif    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ssfv.tbl_unidades (
    unidad_id      SERIAL       PRIMARY KEY,
    simbolo        VARCHAR(20)  NOT NULL UNIQUE,
    nombre         VARCHAR(60)  NOT NULL,
    magnitud       VARCHAR(60)  NOT NULL,
    activo         BOOLEAN      NOT NULL DEFAULT TRUE,
    fecha_creacion TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- ---------------------------------------------------------------------------
-- BLOQUE 2: INFRAESTRUCTURA
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS ssfv.tbl_planta (
    planta_id              SERIAL        PRIMARY KEY,
    nombre                 VARCHAR(100)  NOT NULL,
    ubicacion              VARCHAR(150),
    propietario            VARCHAR(100),
    broker_base            VARCHAR(120)  NOT NULL UNIQUE,
    capacidad_kwp          NUMERIC(10,2),
    fecha_comisionamiento  DATE,
    estado                 SMALLINT      NOT NULL DEFAULT 1
                                         CHECK (estado IN (0, 1, 2)),
    fecha_creacion         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    fecha_modif            TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ssfv.tbl_equipo (
    equipo_id      SERIAL        PRIMARY KEY,
    planta_id      INT           NOT NULL REFERENCES ssfv.tbl_planta(planta_id),
    tipo_id        INT           NOT NULL REFERENCES ssfv.tbl_tipo_equipo(tipo_id),
    nombre_equipo  VARCHAR(80)   NOT NULL,
    nombre_topic   VARCHAR(150)  NOT NULL UNIQUE,
    fabricante     VARCHAR(80),
    modelo         VARCHAR(80),
    nro_serie      VARCHAR(60),
    estado         SMALLINT      NOT NULL DEFAULT 1
                                 CHECK (estado IN (0, 1, 2)),
    fecha_creacion TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    fecha_modif    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_equipo_planta_nombre UNIQUE (planta_id, nombre_equipo)
);

CREATE INDEX IF NOT EXISTS idx_equipo_planta ON ssfv.tbl_equipo(planta_id);
CREATE INDEX IF NOT EXISTS idx_equipo_tipo   ON ssfv.tbl_equipo(tipo_id);

CREATE TABLE IF NOT EXISTS ssfv.tbl_frontera_comercial (
    frontera_id    SERIAL       PRIMARY KEY,
    planta_id      INT          NOT NULL REFERENCES ssfv.tbl_planta(planta_id),
    codigo_nie     VARCHAR(20)  NOT NULL UNIQUE,
    nombre         VARCHAR(100) NOT NULL,
    tipo_conexion  VARCHAR(40),
    activo         BOOLEAN      NOT NULL DEFAULT TRUE,
    fecha_creacion TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    fecha_modif    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_frontera_planta ON ssfv.tbl_frontera_comercial(planta_id);

-- ---------------------------------------------------------------------------
-- BLOQUE 3: CATÁLOGO DE SEÑALES
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS ssfv.tbl_senales (
    senal_id      SERIAL       PRIMARY KEY,
    tipovar_id    INT          NOT NULL REFERENCES ssfv.tbl_tipo_variable(tipovar_id),
    unidad_id     INT          NOT NULL REFERENCES ssfv.tbl_unidades(unidad_id),
    nombre        VARCHAR(80)  NOT NULL,
    descripcion   VARCHAR(200),
    tipo_valor    VARCHAR(20)  NOT NULL
                               CHECK (tipo_valor IN ('Instantaneo', 'Acumulado')),
    codigo_senal  VARCHAR(20)  NOT NULL,
    es_indexada   BOOLEAN      NOT NULL DEFAULT FALSE,
    activo        BOOLEAN      NOT NULL DEFAULT TRUE,
    fecha_alta    DATE         NOT NULL DEFAULT CURRENT_DATE,

    CONSTRAINT uq_senal_codigo_tipvar UNIQUE (codigo_senal, tipovar_id)
);

CREATE INDEX IF NOT EXISTS idx_senales_tipvar ON ssfv.tbl_senales(tipovar_id);
CREATE INDEX IF NOT EXISTS idx_senales_unidad ON ssfv.tbl_senales(unidad_id);

CREATE TABLE IF NOT EXISTS ssfv.tbl_senales_x_equipo (
    equisenal_id     SERIAL       PRIMARY KEY,
    senal_id         INT          NOT NULL REFERENCES ssfv.tbl_senales(senal_id),
    equipo_id        INT          NOT NULL REFERENCES ssfv.tbl_equipo(equipo_id),
    indice_canal     SMALLINT,
    nombre_instancia VARCHAR(30)  NOT NULL,
    activo           BOOLEAN      NOT NULL DEFAULT TRUE,
    fecha_alta       DATE         NOT NULL DEFAULT CURRENT_DATE,

    CONSTRAINT uq_senal_equipo_canal UNIQUE (senal_id, equipo_id, indice_canal),
    CONSTRAINT chk_indice_canal CHECK (
        (indice_canal IS NULL) OR (indice_canal >= 1)
    )
);

CREATE INDEX IF NOT EXISTS idx_sxe_senal  ON ssfv.tbl_senales_x_equipo(senal_id);
CREATE INDEX IF NOT EXISTS idx_sxe_equipo ON ssfv.tbl_senales_x_equipo(equipo_id);

-- ---------------------------------------------------------------------------
-- BLOQUE 4: SERIES TEMPORALES (hypertables)
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS ssfv.tbl_valores (
    timestamp_utc TIMESTAMPTZ  NOT NULL,
    equisenal_id  INT          NOT NULL REFERENCES ssfv.tbl_senales_x_equipo(equisenal_id),
    valor         NUMERIC(18,6),
    calidad       VARCHAR(10)  NOT NULL DEFAULT 'Buena'
                               CHECK (calidad IN ('Buena', 'Dudosa', 'Mala')),

    CONSTRAINT pk_valores PRIMARY KEY (timestamp_utc, equisenal_id)
);

SELECT create_hypertable(
    'ssfv.tbl_valores',
    'timestamp_utc',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_valores_equisenal_time
    ON ssfv.tbl_valores(equisenal_id, timestamp_utc DESC);

DO $$
BEGIN
    ALTER TABLE ssfv.tbl_valores SET (
        timescaledb.compress,
        timescaledb.compress_orderby   = 'timestamp_utc DESC',
        timescaledb.compress_segmentby = 'equisenal_id'
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $$;

DO $$
BEGIN
    PERFORM add_compression_policy('ssfv.tbl_valores', compress_after => INTERVAL '7 days');
EXCEPTION WHEN OTHERS THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS ssfv.tbl_alarmas (
    alarma_id   BIGSERIAL,
    equisenal_id INT         NOT NULL REFERENCES ssfv.tbl_senales_x_equipo(equisenal_id),
    ts_inicio   TIMESTAMPTZ  NOT NULL,
    ts_fin      TIMESTAMPTZ,
    tipo_alarma VARCHAR(20)  NOT NULL
                             CHECK (tipo_alarma IN ('Dispositivo','Comunicacion','Proceso','Fabricante')),
    descripcion VARCHAR(300),
    severidad   VARCHAR(10)  NOT NULL DEFAULT 'Media'
                             CHECK (severidad IN ('Critica','Alta','Media','Baja')),
    activa      BOOLEAN      NOT NULL DEFAULT TRUE,

    CONSTRAINT pk_alarmas PRIMARY KEY (alarma_id, ts_inicio)
);

SELECT create_hypertable(
    'ssfv.tbl_alarmas',
    'ts_inicio',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_alarmas_equisenal_time
    ON ssfv.tbl_alarmas(equisenal_id, ts_inicio DESC);

CREATE INDEX IF NOT EXISTS idx_alarmas_activas
    ON ssfv.tbl_alarmas(activa, ts_inicio DESC)
    WHERE activa = TRUE;

DO $$
BEGIN
    ALTER TABLE ssfv.tbl_alarmas SET (
        timescaledb.compress,
        timescaledb.compress_orderby   = 'ts_inicio DESC',
        timescaledb.compress_segmentby = 'equisenal_id'
    );
EXCEPTION WHEN OTHERS THEN NULL;
END $$;

DO $$
BEGIN
    PERFORM add_compression_policy('ssfv.tbl_alarmas', compress_after => INTERVAL '30 days');
EXCEPTION WHEN OTHERS THEN NULL;
END $$;

-- Fallback table: SSFV signals without a catalog match (public schema — not ssfv).
CREATE TABLE IF NOT EXISTS public.signals_raw (
    ts          TIMESTAMPTZ  NOT NULL,
    signal_path TEXT         NOT NULL,
    signal      TEXT,
    value       DOUBLE PRECISION,
    quality     SMALLINT     DEFAULT 0,
    tags        JSONB        DEFAULT '{}'::jsonb,
    PRIMARY KEY (ts, signal_path)
);

SELECT create_hypertable(
    'public.signals_raw',
    'ts',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists       => TRUE
);

CREATE INDEX IF NOT EXISTS idx_signals_raw_path_time
    ON public.signals_raw(signal_path, ts DESC);

-- ---------------------------------------------------------------------------
-- BLOQUE 5: VISTAS
-- ---------------------------------------------------------------------------

CREATE OR REPLACE VIEW ssfv.v_senales_contexto AS
SELECT
    sxe.equisenal_id,
    sxe.nombre_instancia,
    sxe.indice_canal,
    sxe.activo                  AS senal_activa,
    s.senal_id,
    s.nombre                    AS senal_nombre,
    s.codigo_senal,
    s.tipo_valor,
    s.es_indexada,
    tv.nombre                   AS tipo_variable,
    u.simbolo                   AS unidad,
    u.magnitud,
    e.equipo_id,
    e.nombre_equipo,
    e.nombre_topic,
    e.fabricante,
    e.modelo,
    te.nombre                   AS tipo_equipo,
    p.planta_id,
    p.nombre                    AS planta_nombre,
    p.broker_base
FROM ssfv.tbl_senales_x_equipo  sxe
JOIN ssfv.tbl_senales            s   ON s.senal_id    = sxe.senal_id
JOIN ssfv.tbl_tipo_variable      tv  ON tv.tipovar_id = s.tipovar_id
JOIN ssfv.tbl_unidades           u   ON u.unidad_id   = s.unidad_id
JOIN ssfv.tbl_equipo             e   ON e.equipo_id   = sxe.equipo_id
JOIN ssfv.tbl_tipo_equipo        te  ON te.tipo_id    = e.tipo_id
JOIN ssfv.tbl_planta             p   ON p.planta_id   = e.planta_id;

CREATE OR REPLACE VIEW ssfv.v_ultimas_lecturas AS
SELECT DISTINCT ON (equisenal_id)
    v.equisenal_id,
    v.timestamp_utc,
    v.valor,
    v.calidad
FROM ssfv.tbl_valores v
ORDER BY equisenal_id, timestamp_utc DESC;

CREATE OR REPLACE VIEW ssfv.v_alarmas_activas AS
SELECT
    a.alarma_id,
    a.ts_inicio,
    a.tipo_alarma,
    a.severidad,
    a.descripcion,
    sc.planta_nombre,
    sc.nombre_equipo,
    sc.tipo_equipo,
    sc.nombre_instancia,
    sc.tipo_variable
FROM ssfv.tbl_alarmas        a
JOIN ssfv.v_senales_contexto sc ON sc.equisenal_id = a.equisenal_id
WHERE a.activa = TRUE
ORDER BY a.severidad DESC, a.ts_inicio DESC;

-- ---------------------------------------------------------------------------
-- BLOQUE 6: DATOS MAESTROS INICIALES
-- ---------------------------------------------------------------------------

INSERT INTO ssfv.tbl_tipo_equipo (nombre, descripcion) VALUES
    ('Inversor',               'Convertidor DC/AC de paneles solares'),
    ('Medidor',                'Medidor de energía eléctrica'),
    ('Estación Meteorológica', 'Estación de variables ambientales'),
    ('Frontera Comercial',     'Medidor de frontera con la red eléctrica')
ON CONFLICT (nombre) DO NOTHING;

INSERT INTO ssfv.tbl_tipo_variable (nombre, descripcion) VALUES
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
ON CONFLICT (nombre) DO NOTHING;

INSERT INTO ssfv.tbl_unidades (simbolo, nombre, magnitud) VALUES
    ('A',            'Amperio',                          'Corriente'),
    ('V',            'Voltio',                           'Tensión'),
    ('kW',           'Kilovatio',                        'Potencia'),
    ('kVar',         'Kilovoltamperio reactivo',         'Potencia reactiva'),
    ('kVA',          'Kilovoltamperio',                  'Potencia aparente'),
    ('kWh',          'Kilovatio hora',                   'Energía'),
    ('kVarh',        'Kilovoltamperio reactivo hora',    'Energía reactiva'),
    ('%',            'Porcentaje',                       'Proceso'),
    ('Hz',           'Hercio',                           'Frecuencia'),
    ('°C',           'Grado Celsius',                    'Temperatura'),
    ('W/m2',         'Vatio por metro cuadrado',         'Irradiancia'),
    ('MΩ',           'Megaohmio',                        'Resistencia'),
    ('Adimensional', 'Sin unidad',                       'Estado/Proceso')
ON CONFLICT (simbolo) DO NOTHING;
