// Package db manages the SQLite database: schema application, migrations, and
// the Open helper that returns a ready-to-use *sqlx.DB.
package db

import (
	_ "embed"
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Open opens SQLite file, applies schema (idempotent), returns handle.
func Open(path string) (*sqlx.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
	db, err := sqlx.Connect("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return db, nil
}

// migrate handles cross-version DDL changes that schema.sql alone cannot
// express idempotently (column drops, UNIQUE-constraint reshapes, etc.).
// Runs before schema.sql; safe on a fresh DB (no-ops when target tables are
// absent or already on the current shape).
func migrate(db *sqlx.DB) error {
	// iec104_servers: drop legacy listen_addr column. Listen IP is now a
	// gateway-wide setting in iec104_gateway. The simplest path is to drop
	// the table when the legacy column is detected; schema.sql then recreates
	// it with the current shape. Acceptable because the project has not
	// shipped — only the seed default row is at risk.
	var n int
	if err := db.Get(&n, `SELECT COUNT(*) FROM pragma_table_info('iec104_servers') WHERE name='listen_addr'`); err != nil {
		return err
	}
	if n > 0 {
		if _, err := db.Exec(`DROP TABLE iec104_servers`); err != nil {
			return err
		}
	}

	// devices: scope to a server (server_id). Old shape had a global UNIQUE
	// on name; new shape is UNIQUE(server_id, name). Same recipe as the
	// signal_mappings rebuild — detect by missing column, copy rows pinned to
	// the lowest server id, swap the table.
	var hasDevices, hasDevServerID int
	if err := db.Get(&hasDevices, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='devices'`); err != nil {
		return err
	}
	if hasDevices > 0 {
		if err := db.Get(&hasDevServerID, `SELECT COUNT(*) FROM pragma_table_info('devices') WHERE name='server_id'`); err != nil {
			return err
		}
		if hasDevServerID == 0 {
			if err := rebuildDevices(db); err != nil {
				return fmt.Errorf("devices rebuild: %w", err)
			}
		}
	}

	// signal_mappings: introduce server_id (each mapping lives on exactly one
	// IEC-104 slave) and switch IOA uniqueness from global to (server_id, ioa).
	// SQLite cannot drop a table-level UNIQUE in place, so we rebuild the
	// table when the legacy shape is detected. Existing rows are pinned to the
	// lowest server id (the historical "broadcast to all" semantics collapse
	// to "first server" so the operator can re-pick after upgrading).
	var hasMappings, hasServerID int
	if err := db.Get(&hasMappings, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='signal_mappings'`); err != nil {
		return err
	}
	if hasMappings > 0 {
		if err := db.Get(&hasServerID, `SELECT COUNT(*) FROM pragma_table_info('signal_mappings') WHERE name='server_id'`); err != nil {
			return err
		}
		if hasServerID == 0 {
			if err := rebuildSignalMappings(db); err != nil {
				return fmt.Errorf("signal_mappings rebuild: %w", err)
			}
		}

		// signal_mappings: add quality_key column (per-signal quality JSON key).
		var hasQualityKey int
		if err := db.Get(&hasQualityKey, `SELECT COUNT(*) FROM pragma_table_info('signal_mappings') WHERE name='quality_key'`); err != nil {
			return err
		}
		if hasQualityKey == 0 {
			if _, err := db.Exec(`ALTER TABLE signal_mappings ADD COLUMN quality_key TEXT NOT NULL DEFAULT ''`); err != nil {
				return fmt.Errorf("add quality_key column: %w", err)
			}
		}

		// signal_mappings: add metric_name column (Sparkplug B metric identifier).
		var hasMetricName int
		if err := db.Get(&hasMetricName, `SELECT COUNT(*) FROM pragma_table_info('signal_mappings') WHERE name='metric_name'`); err != nil {
			return err
		}
		if hasMetricName == 0 {
			if _, err := db.Exec(`ALTER TABLE signal_mappings ADD COLUMN metric_name TEXT NOT NULL DEFAULT ''`); err != nil {
				return fmt.Errorf("add metric_name column: %w", err)
			}
		}
	}

	// mqtt_config: add Sparkplug B columns.
	// Guard with table-existence check: on a fresh DB mqtt_config does not exist
	// yet at migrate() time — schema.sql creates it with the columns already
	// present, so there is nothing to do.
	var hasMQTTConfig int
	if err := db.Get(&hasMQTTConfig, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='mqtt_config'`); err != nil {
		return err
	}
	if hasMQTTConfig > 0 {
		for _, col := range []struct{ name, def string }{
			{"sparkplug_enabled", "INTEGER NOT NULL DEFAULT 0"},
			{"sp_group_id", "TEXT NOT NULL DEFAULT 'goGateway'"},
			{"sp_host_id", "TEXT NOT NULL DEFAULT 'goGateway-host'"},
		} {
			var has int
			if err := db.Get(&has, `SELECT COUNT(*) FROM pragma_table_info('mqtt_config') WHERE name=?`, col.name); err != nil {
				return err
			}
			if has == 0 {
				if _, err := db.Exec(`ALTER TABLE mqtt_config ADD COLUMN ` + col.name + ` ` + col.def); err != nil {
					return fmt.Errorf("add mqtt_config.%s: %w", col.name, err)
				}
			}
		}
	}

	return nil
}

// rebuildDevices adds server_id and switches the UNIQUE constraint to
// (server_id, name) by recreating the table. Existing rows pin to the lowest
// existing iec104_servers row (or the seeded id=1 if none exist yet).
func rebuildDevices(db *sqlx.DB) error {
	var defaultServer int64
	if err := db.Get(&defaultServer, `SELECT MIN(id) FROM iec104_servers`); err != nil {
		return fmt.Errorf("rebuildDevices: query min server: %w", err)
	}
	if defaultServer == 0 {
		if _, err := db.Exec(`INSERT OR IGNORE INTO iec104_servers (id, name, port, asdu_addr, scada_ips, k, w, t0, t1, t2, t3, enabled) VALUES (1, 'default', 2404, 1, '', 12, 8, 30, 15, 10, 20, 0)`); err != nil {
			return err
		}
		defaultServer = 1
	}
	stmts := []string{
		`PRAGMA foreign_keys = OFF`,
		`BEGIN`,
		`CREATE TABLE devices_new (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id   INTEGER NOT NULL REFERENCES iec104_servers(id) ON DELETE CASCADE,
			name        TEXT NOT NULL,
			description TEXT DEFAULT '',
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (server_id, name)
		)`,
		fmt.Sprintf(`INSERT INTO devices_new (id, server_id, name, description, created_at)
			SELECT id, %d, name, description, created_at FROM devices`, defaultServer),
		`DROP TABLE devices`,
		`ALTER TABLE devices_new RENAME TO devices`,
		`COMMIT`,
		`PRAGMA foreign_keys = ON`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			_, _ = db.Exec(`ROLLBACK`)
			_, _ = db.Exec(`PRAGMA foreign_keys = ON`)
			return fmt.Errorf("step %q: %w", s, err)
		}
	}
	return nil
}

// rebuildSignalMappings adds server_id by recreating the table. Runs inside a
// transaction; foreign keys are paused so the FK on signal_mappings_new pointing
// to iec104_servers does not trip on the first insert under defer-disabled FKs.
func rebuildSignalMappings(db *sqlx.DB) error {
	// Pick a default server: lowest existing id, or seed 1.
	var defaultServer int64
	if err := db.Get(&defaultServer, `SELECT MIN(id) FROM iec104_servers`); err != nil {
		return fmt.Errorf("rebuildSignalMappings: query min server: %w", err)
	}
	if defaultServer == 0 {
		// Seed an iec104_servers row so the FK has a target. schema.sql will
		// idempotently re-seed id=1 anyway.
		if _, err := db.Exec(`INSERT OR IGNORE INTO iec104_servers (id, name, port, asdu_addr, scada_ips, k, w, t0, t1, t2, t3, enabled) VALUES (1, 'default', 2404, 1, '', 12, 8, 30, 15, 10, 20, 0)`); err != nil {
			return err
		}
		defaultServer = 1
	}

	stmts := []string{
		`PRAGMA foreign_keys = OFF`,
		`BEGIN`,
		`CREATE TABLE signal_mappings_new (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id      INTEGER NOT NULL REFERENCES iec104_servers(id) ON DELETE CASCADE,
			topic_id       INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
			device_name    TEXT DEFAULT '',
			variable_type  TEXT DEFAULT '',
			characteristic TEXT DEFAULT '',
			json_key       TEXT NOT NULL,
			iec104_type    TEXT NOT NULL,
			ioa            INTEGER NOT NULL,
			unit           TEXT DEFAULT '',
			scale          REAL NOT NULL DEFAULT 1.0,
			enabled        INTEGER NOT NULL DEFAULT 1,
			UNIQUE (server_id, ioa)
		)`,
		fmt.Sprintf(`INSERT INTO signal_mappings_new
			(id, server_id, topic_id, device_name, variable_type, characteristic,
			 json_key, iec104_type, ioa, unit, scale, enabled)
			SELECT id, %d, topic_id, device_name, variable_type, characteristic,
			       json_key, iec104_type, ioa, unit, scale, enabled
			  FROM signal_mappings`, defaultServer),
		`DROP TABLE signal_mappings`,
		`ALTER TABLE signal_mappings_new RENAME TO signal_mappings`,
		`COMMIT`,
		`PRAGMA foreign_keys = ON`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			_, _ = db.Exec(`ROLLBACK`)
			_, _ = db.Exec(`PRAGMA foreign_keys = ON`)
			return fmt.Errorf("step %q: %w", s, err)
		}
	}
	return nil
}
