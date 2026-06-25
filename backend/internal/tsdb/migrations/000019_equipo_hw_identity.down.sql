ALTER TABLE ssfv.tbl_equipo
    DROP COLUMN IF EXISTS hw_part_number,
    DROP COLUMN IF EXISTS hw_product_type,
    DROP COLUMN IF EXISTS hw_product_name,
    DROP COLUMN IF EXISTS hw_firmware,
    DROP COLUMN IF EXISTS hw_serial,
    DROP COLUMN IF EXISTS hw_uuid,
    DROP COLUMN IF EXISTS hw_reported_at;
