package tsdb

import (
	"context"
	"embed"
	"fmt"
	"log"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migFS embed.FS

// runMigrations applies numbered *.up.sql files in order, tracking applied
// versions in a schema_migrations table. Idempotent — safe to call on every start.
func (a *TimescaleAdapter) runMigrations(ctx context.Context) error {
	// Ensure tracking table exists.
	if _, err := a.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INT PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT NOW()
		)`); err != nil {
		return fmt.Errorf("schema_migrations table: %w", err)
	}

	entries, err := migFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Collect and sort .up.sql files by version number.
	type migration struct {
		version int
		name    string
	}
	var migs []migration
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		var v int
		fmt.Sscanf(e.Name(), "%d_", &v)
		migs = append(migs, migration{version: v, name: e.Name()})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })

	for _, m := range migs {
		var count int
		a.pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE version=$1`,
			m.version).Scan(&count) //nolint:errcheck
		if count > 0 {
			continue
		}
		data, err := migFS.ReadFile("migrations/" + m.name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", m.name, err)
		}
		if _, err := a.pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.name, err)
		}
		a.pool.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, m.version) //nolint:errcheck
		log.Printf("tsdb: applied migration %s", m.name)
	}
	return nil
}

// ReadDir exposes the embedded migration FS for external tooling.
func MigrationFS() embed.FS { return migFS }
