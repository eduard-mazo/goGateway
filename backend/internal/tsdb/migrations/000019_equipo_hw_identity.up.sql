-- =============================================================================
-- MIGRATION 019 — Device hardware identity on tbl_equipo.
--
-- ICR-class edges (Advantech ICR-323x) publish their board identity as static
-- string metrics in the node's System telemetry: <metricPrefix>Device/PartNumber,
-- /ProductType, /ProductName, /Firmware, /Serial, /UUID (producer sysmon
-- vendor_icr.go). These are not numeric samples, so they don't belong in
-- tbl_valores; store them as auto-reported identity on the node's equipo.
--
-- Kept separate from the manually-entered fabricante/modelo/nro_serie: these are
-- reported by the device itself. hw_reported_at records the last refresh.
-- =============================================================================

ALTER TABLE ssfv.tbl_equipo
    ADD COLUMN IF NOT EXISTS hw_part_number  VARCHAR(120),
    ADD COLUMN IF NOT EXISTS hw_product_type VARCHAR(120),
    ADD COLUMN IF NOT EXISTS hw_product_name VARCHAR(120),
    ADD COLUMN IF NOT EXISTS hw_firmware     VARCHAR(120),
    ADD COLUMN IF NOT EXISTS hw_serial       VARCHAR(120),
    ADD COLUMN IF NOT EXISTS hw_uuid         VARCHAR(120),
    ADD COLUMN IF NOT EXISTS hw_reported_at  TIMESTAMPTZ;
