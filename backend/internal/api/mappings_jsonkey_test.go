package api

import (
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// oldShapeSignalMappings recreates a signal_mappings table as it exists on DBs
// migrated through the multi-server table rebuild: json_key is NOT NULL with NO
// default (db.go migrate), while the later-added columns carry defaults. A fresh
// db.Open() would instead give json_key a default (schema.sql), so this raw
// table is what reproduces the production "NOT NULL constraint failed:
// signal_mappings.json_key" on an insert that omits json_key.
func oldShapeDB(t *testing.T) *sqlx.DB {
	t.Helper()
	d, err := sqlx.Connect("sqlite", "file:"+filepath.Join(t.TempDir(), "old.db")+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if _, err := d.Exec(`
		CREATE TABLE signal_mappings (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id      INTEGER NOT NULL,
			topic_id       INTEGER NOT NULL,
			signal_id      INTEGER,
			device_name    TEXT DEFAULT '',
			variable_type  TEXT DEFAULT '',
			characteristic TEXT DEFAULT '',
			json_key       TEXT NOT NULL,             -- old shape: NOT NULL, NO default
			quality_key    TEXT NOT NULL DEFAULT '',
			metric_name    TEXT NOT NULL DEFAULT '',
			iec104_type    TEXT NOT NULL,
			ioa            INTEGER NOT NULL,
			unit           TEXT DEFAULT '',
			scale          REAL NOT NULL DEFAULT 1.0,
			enabled        INTEGER NOT NULL DEFAULT 1,
			business       TEXT NOT NULL DEFAULT '',
			company        TEXT NOT NULL DEFAULT '',
			deadband_abs   REAL NOT NULL DEFAULT 0.0,
			deadband_pct   REAL NOT NULL DEFAULT 0.0,
			ssfv_planta_id    INTEGER NOT NULL DEFAULT 0,
			ssfv_equipo_id    INTEGER NOT NULL DEFAULT 0,
			ssfv_equisenal_id INTEGER NOT NULL DEFAULT 0,
			UNIQUE (server_id, ioa)
		)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return d
}

// TestAssignMapping_JSONKeyNotNull guards the regression: assignMapping must
// supply json_key so it works on a DB where the column has no default. Before
// the fix this failed with "NOT NULL constraint failed: signal_mappings.json_key".
func TestAssignMapping_JSONKeyNotNull(t *testing.T) {
	d := oldShapeDB(t)
	m, err := assignMapping(d, mappingSpec{
		ServerID: 1, TopicID: 1, MetricName: "Medidas/Energy_kWh",
		IEC104Type: "M_ME_NC_1", SSFVPlantaID: 7, SSFVEquipoID: 70, SSFVEquisenalID: 700,
	})
	if err != nil {
		t.Fatalf("assignMapping on json_key-NOT-NULL DB: %v", err)
	}
	if m.IOA != 1 || m.IEC104Type != "M_ME_NC_1" || m.MetricName != "Medidas/Energy_kWh" {
		t.Fatalf("unexpected mapping: %+v", m)
	}
	if m.JSONKey != "" {
		t.Fatalf("json_key should be empty string, got %q", m.JSONKey)
	}
	if m.SSFVPlantaID != 7 || m.SSFVEquisenalID != 700 {
		t.Fatalf("SSFV refs not stored: %+v", m)
	}
}
