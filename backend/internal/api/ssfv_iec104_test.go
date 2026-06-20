package api

import (
	"path/filepath"
	"testing"

	"goGateway/internal/db"
	"goGateway/internal/models"
	"goGateway/internal/sparkplug"

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

// TestSsfvStorageTopic_RoundTrip is the regression guard for the cache-key bug:
// the stored topic must re-parse (as the worker's spNodeBase does) back to the
// exact base the live dispatch path keys by — NodeBase() for a node entity,
// DeviceBase() for a device entity. A bare 4-segment device base would mis-parse
// (ParseTopic doesn't validate the msg-type segment), so it must NOT be stored.
func TestSsfvStorageTopic_RoundTrip(t *testing.T) {
	cases := []struct {
		nombreTopic string
		wantBase    string // what dispatch computes: spBv1.0/<base>
	}{
		{"EPM_SOAK/edge-1/meter-01", "spBv1.0/EPM_SOAK/edge-1/meter-01"}, // device
		{"EPM_SOAK/edge-1", "spBv1.0/EPM_SOAK/edge-1"},                   // node
	}
	for _, c := range cases {
		stored := ssfvStorageTopic(c.nombreTopic)
		tp, ok := sparkplug.ParseTopic(stored)
		if !ok {
			t.Fatalf("stored topic %q did not parse", stored)
		}
		got := tp.NodeBase()
		if tp.DeviceID != "" {
			got = tp.DeviceBase()
		}
		if got != c.wantBase {
			t.Errorf("%q → stored %q → base %q, want %q", c.nombreTopic, stored, got, c.wantBase)
		}
	}
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
		{Topic: ssfvStorageTopic("EPM/edge-1/meter-01"), MetricName: "Medidas/Energy_kWh", IEC104Type: "M_ME_NC_1", Unit: "kWh"},
		{Topic: ssfvStorageTopic("EPM/edge-1/meter-01"), MetricName: "Medidas/Power_kW", IEC104Type: "M_ME_NC_1", Unit: "kW"},
		{Topic: ssfvStorageTopic("EPM/edge-1"), MetricName: "PLC/tank_level", IEC104Type: "M_ME_NC_1", Unit: "m"},
		{Topic: ssfvStorageTopic("EPM/edge-1"), MetricName: "", IEC104Type: "M_ME_NC_1"}, // unmappable → skipped
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

// TestCascadeDeleteMirrors: mirrors store their SSFV refs, and cascade-deleting
// by equisenal/equipo/planta removes exactly the right ones and reloads the cache.
func TestCascadeDeleteMirrors(t *testing.T) {
	d := newBridgeDB(t)
	reloads := 0
	h := &SSFVHandler{}
	h.SetDB(d)
	h.SetMappingNotifier(func() { reloads++ })

	sigs := []derivedSignal{
		{PlantaID: 7, EquipoID: 70, EquisenalID: 700, Topic: ssfvStorageTopic("G/N/d70"), MetricName: "a/x", IEC104Type: "M_ME_NC_1"},
		{PlantaID: 7, EquipoID: 70, EquisenalID: 701, Topic: ssfvStorageTopic("G/N/d70"), MetricName: "a/y", IEC104Type: "M_ME_NC_1"},
		{PlantaID: 7, EquipoID: 71, EquisenalID: 710, Topic: ssfvStorageTopic("G/N/d71"), MetricName: "b/z", IEC104Type: "M_SP_NA_1"},
	}
	res, err := exposeSignals(d, 1, sigs, 0)
	if err != nil || len(res.Created) != 3 {
		t.Fatalf("expose: want 3 created, got %d err=%v", len(res.Created), err)
	}
	if m := res.Created[0]; m.SSFVPlantaID != 7 || m.SSFVEquipoID != 70 || m.SSFVEquisenalID != 700 {
		t.Fatalf("SSFV refs not stored: %+v", m)
	}

	count := func() int {
		var n int
		d.Get(&n, `SELECT COUNT(*) FROM signal_mappings`)
		return n
	}
	h.cascadeDeleteMirrors("ssfv_equisenal_id", 700)
	if count() != 2 {
		t.Fatalf("after equisenal cascade: want 2, got %d", count())
	}
	h.cascadeDeleteMirrors("ssfv_equipo_id", 70) // removes 701 (the other on equipo 70)
	if count() != 1 {
		t.Fatalf("after equipo cascade: want 1, got %d", count())
	}
	h.cascadeDeleteMirrors("ssfv_planta_id", 7) // removes the last (710)
	if count() != 0 {
		t.Fatalf("after planta cascade: want 0, got %d", count())
	}
	if reloads < 3 {
		t.Fatalf("each cascade that removed rows should reload the cache; got %d reloads", reloads)
	}
}

// TestExposeSignals_AutoIOA: with ioaStart=0, IOAs auto-assign per server (1,2,…).
func TestExposeSignals_AutoIOA(t *testing.T) {
	d := newBridgeDB(t)
	sigs := []derivedSignal{
		{Topic: ssfvStorageTopic("EPM/edge-1"), MetricName: "a/x", IEC104Type: "M_ME_NC_1"},
		{Topic: ssfvStorageTopic("EPM/edge-1"), MetricName: "a/y", IEC104Type: "M_SP_NA_1"},
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
