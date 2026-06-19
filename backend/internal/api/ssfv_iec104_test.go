package api

import (
	"path/filepath"
	"testing"

	"goGateway/internal/db"
	"goGateway/internal/models"

	"github.com/jmoiron/sqlx"
)

func newBridgeDB(t *testing.T) *sqlx.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "bridge.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestBridgeIEC104Type(t *testing.T) {
	cases := []struct {
		tv, tval string
		alarm    bool
		want     string
	}{
		{"Analogica", "", false, "M_ME_NC_1"},
		{"", "Instantaneo", false, "M_ME_NC_1"},
		{"", "Acumulado", false, "M_ME_NC_1"},
		{"Digital", "", false, "M_SP_NA_1"},
		{"", "", true, "M_SP_NA_1"},  // unknown but alarm → single point
		{"", "", false, "M_ME_NC_1"}, // unknown, not alarm → analog default
		{"Potencia", "Instantaneo", false, "M_ME_NC_1"},
	}
	for _, c := range cases {
		if got := bridgeIEC104Type(c.tv, c.tval, c.alarm); got != c.want {
			t.Errorf("bridgeIEC104Type(%q,%q,%v)=%q want %q", c.tv, c.tval, c.alarm, got, c.want)
		}
	}
}

func TestEnsureIEC104Topic_FindOrCreate(t *testing.T) {
	d := newBridgeDB(t)
	const nodeBase = "spBv1.0/EPM/edge-1/meter-01"

	id1, err := ensureIEC104Topic(d, 1, nodeBase)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id2, err := ensureIEC104Topic(d, 1, nodeBase)
	if err != nil {
		t.Fatalf("reuse: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("idempotent: want same topic id, got %d vs %d", id1, id2)
	}
	// Exactly one device and one topic were created.
	var nDev, nTopic int
	d.Get(&nDev, `SELECT COUNT(*) FROM devices WHERE server_id=1`)
	d.Get(&nTopic, `SELECT COUNT(*) FROM topics`)
	if nDev != 1 || nTopic != 1 {
		t.Fatalf("want 1 device + 1 topic, got %d / %d", nDev, nTopic)
	}
	var format, topic string
	d.Get(&format, `SELECT payload_format FROM topics WHERE id=?`, id1)
	d.Get(&topic, `SELECT topic FROM topics WHERE id=?`, id1)
	if format != "sparkplug" || topic != nodeBase {
		t.Fatalf("topic row wrong: format=%q topic=%q", format, topic)
	}
}

// TestExposeSignals covers the bulk core: contiguous IOA assignment, dedupe on
// re-run, per-node topic reuse, and skipping unmappable rows.
func TestExposeSignals(t *testing.T) {
	d := newBridgeDB(t)
	sigs := []derivedSignal{
		{NodeBase: "spBv1.0/EPM/edge-1/meter-01", MetricName: "Medidas/Energy_kWh", IEC104Type: "M_ME_NC_1", Unit: "kWh"},
		{NodeBase: "spBv1.0/EPM/edge-1/meter-01", MetricName: "Medidas/Power_kW", IEC104Type: "M_ME_NC_1", Unit: "kW"},
		{NodeBase: "spBv1.0/EPM/edge-1", MetricName: "PLC/tank_level", IEC104Type: "M_ME_NC_1", Unit: "m"},
		{NodeBase: "spBv1.0/EPM/edge-1", MetricName: "", IEC104Type: "M_ME_NC_1"}, // unmappable → skipped
	}

	res, err := exposeSignals(d, 1, sigs, 100) // explicit base 100
	if err != nil {
		t.Fatalf("exposeSignals: %v", err)
	}
	if len(res.Created) != 3 {
		t.Fatalf("want 3 created, got %d (%+v)", len(res.Created), res.Created)
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("want 1 skipped, got %v", res.Skipped)
	}
	// Contiguous IOAs from the base, in order.
	wantIOA := []int{100, 101, 102}
	for i, m := range res.Created {
		if m.IOA != wantIOA[i] {
			t.Errorf("created[%d] IOA=%d want %d", i, m.IOA, wantIOA[i])
		}
		if !m.Enabled || m.Scale != 1.0 {
			t.Errorf("created[%d] not enabled / scale!=1: %+v", i, m)
		}
	}
	// Two metrics on meter-01 share ONE topic; tank_level uses another → 2 topics.
	var nTopic int
	d.Get(&nTopic, `SELECT COUNT(*) FROM topics`)
	if nTopic != 2 {
		t.Fatalf("want 2 topics (one per node base), got %d", nTopic)
	}

	// Re-run: everything already mapped → 0 created, 3 skipped.
	res2, err := exposeSignals(d, 1, sigs, 0)
	if err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if len(res2.Created) != 0 {
		t.Fatalf("rerun should create nothing, got %d", len(res2.Created))
	}
	var total int
	d.Get(&total, `SELECT COUNT(*) FROM signal_mappings`)
	if total != 3 {
		t.Fatalf("rerun must not duplicate: want 3 mappings total, got %d", total)
	}
}

// TestExposeSignals_AutoIOA: with ioaStart=0, IOAs auto-assign per server (1,2,…).
func TestExposeSignals_AutoIOA(t *testing.T) {
	d := newBridgeDB(t)
	sigs := []derivedSignal{
		{NodeBase: "spBv1.0/EPM/edge-1", MetricName: "a/x", IEC104Type: "M_ME_NC_1"},
		{NodeBase: "spBv1.0/EPM/edge-1", MetricName: "a/y", IEC104Type: "M_SP_NA_1"},
	}
	res, err := exposeSignals(d, 1, sigs, 0)
	if err != nil {
		t.Fatalf("exposeSignals: %v", err)
	}
	var got []models.SignalMapping = res.Created
	if len(got) != 2 || got[0].IOA != 1 || got[1].IOA != 2 {
		t.Fatalf("auto IOA want 1,2 got %+v", got)
	}
}
