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
	return nil
}
