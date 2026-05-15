package tsdb

import (
	"context"
	"embed"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migFS embed.FS

// runMigrations applies numbered *.up.sql files in order, tracking applied
// versions in a schema_migrations table. Idempotent — safe to call on every start.
func (a *TimescaleAdapter) runMigrations(ctx context.Context) error {
	return applyMigrations(ctx, a.pool)
}

// applyMigrations is the pool-agnostic core shared by all adapters.
func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
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
		pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE version=$1`,
			m.version).Scan(&count) //nolint:errcheck
		if count > 0 {
			continue
		}
		data, err := migFS.ReadFile("migrations/" + m.name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", m.name, err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.name, err)
		}
		pool.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, m.version) //nolint:errcheck
		log.Printf("tsdb: applied migration %s", m.name)
	}
	return nil
}

// resetStaleSsfvMarkers removes schema_migrations entries for ssfv migrations
// (version >= 7) when the ssfv schema itself no longer exists.
// This handles the case where a DBA drops the ssfv schema manually — without
// this, the migration runner would skip all ssfv .up.sql files because their
// version numbers are still recorded in schema_migrations.
func resetStaleSsfvMarkers(ctx context.Context, pool *pgxpool.Pool) {
	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.schemata
			WHERE schema_name = 'ssfv'
		)`).Scan(&exists); err != nil || exists {
		return // can't check, or schema is present — nothing to reset
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM public.schema_migrations WHERE version >= 7`); err != nil {
		log.Printf("tsdb: reset ssfv markers (ignored: %v)", err)
		return
	}
	log.Printf("tsdb: ssfv schema absent — reset migration markers ≥7, will re-apply")
}

// MigrationFS exposes the embedded migration FS for external tooling.
func MigrationFS() embed.FS { return migFS }
