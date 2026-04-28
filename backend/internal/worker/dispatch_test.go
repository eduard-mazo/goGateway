package worker

import (
	"context"
	"testing"
	"time"

	"goGateway/internal/db"
	"goGateway/internal/iec104"
	"goGateway/internal/models"
)

const inverterPayload = `{
 "date": "2026-04-17T07:30:00-05:00",
 "IA": 9.5, "IB": 8.3, "IC": 8.4,
 "UAB": 490, "UBC": 490.3, "UCA": 488,
 "AP": 6.7, "OS": 3
}`

type fakeSrv struct {
	points    []iec104.Point
	serverIDs []int64
}

func (f *fakeSrv) Start() error { return nil }
func (f *fakeSrv) Stop() error  { return nil }
func (f *fakeSrv) Dispatch(serverID int64, p iec104.Point) {
	f.serverIDs = append(f.serverIDs, serverID)
	f.points = append(f.points, p)
}
func (f *fakeSrv) Reload(_ models.IEC104Gateway, _ []models.IEC104Server) error {
	return nil
}
func (f *fakeSrv) Status() iec104.Status    { return iec104.Status{} }
func (f *fakeSrv) Snapshot() []iec104.Point { return f.points }

func TestParseAndDispatch_Inverter(t *testing.T) {
	dbh, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer dbh.Close()

	hist := NewHistoryLogger(dbh, 64, 10, 100*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	go hist.Run(ctx)

	srv := &fakeSrv{}
	maps := []TopicMapping{
		{MappingID: 1, ServerID: 1, TopicID: 1, Topic: "t1", JSONKey: "IA", IEC104Type: "M_ME_TF_1", IOA: 16385, Scale: 1, SignalKey: "Inversor 1.IA"},
		{MappingID: 2, ServerID: 1, TopicID: 1, Topic: "t1", JSONKey: "UAB", IEC104Type: "M_ME_TF_1", IOA: 16386, Scale: 1, SignalKey: "Inversor 1.UAB"},
		{MappingID: 3, ServerID: 1, TopicID: 1, Topic: "t1", JSONKey: "OS", IEC104Type: "M_ME_NB_1", IOA: 16387, Scale: 1, SignalKey: "Inversor 1.OS"},
		{MappingID: 4, ServerID: 1, TopicID: 1, Topic: "t1", JSONKey: "MISSING", IEC104Type: "M_ME_TF_1", IOA: 16388, Scale: 1, SignalKey: "Inversor 1.MISSING"},
	}

	// seed mapping rows so history FK passes. iec104_servers id=1 is seeded
	// by schema.sql, so the new server_id FK is already satisfied.
	dbh.MustExec(`INSERT INTO devices(id,server_id,name) VALUES(1,1,'INV_1')`)
	dbh.MustExec(`INSERT INTO topics(id,device_id,topic,enabled) VALUES(1,1,'t1',1)`)
	for _, m := range maps {
		dbh.MustExec(`INSERT INTO signal_mappings(id,server_id,topic_id,json_key,iec104_type,ioa,scale,enabled) VALUES(?,1,1,?,?,?,?,1)`,
			m.MappingID, m.JSONKey, m.IEC104Type, m.IOA, m.Scale)
	}

	ParseAndDispatch("t1", []byte(inverterPayload), maps, srv, hist)

	if len(srv.points) != 3 {
		t.Fatalf("want 3 dispatched (missing key skipped), got %d", len(srv.points))
	}
	byIOA := map[int]float64{}
	for _, p := range srv.points {
		byIOA[p.IOA] = p.Value
	}
	if byIOA[16385] != 9.5 {
		t.Errorf("IA: want 9.5 got %v", byIOA[16385])
	}
	if byIOA[16386] != 490 {
		t.Errorf("UAB: want 490 got %v", byIOA[16386])
	}
	if byIOA[16387] != 3 {
		t.Errorf("OS: want 3 got %v", byIOA[16387])
	}

	want, _ := time.Parse(time.RFC3339, "2026-04-17T07:30:00-05:00")
	if !srv.points[0].Timestamp.Equal(want) {
		t.Errorf("ts: want %v got %v", want, srv.points[0].Timestamp)
	}

	// flush history via cancel drain.
	cancel()
	time.Sleep(200 * time.Millisecond)

	var n int
	if err := dbh.Get(&n, `SELECT COUNT(*) FROM history`); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("history rows: want 3 got %d", n)
	}
}

func TestParseMQTTQuality(t *testing.T) {
	cases := []struct {
		json string
		want int
	}{
		{`0`, iec104.QualityGood},
		{`128`, iec104.QualityInvalid},
		{`192`, iec104.QualityInvalid | iec104.QualityNotTopical},
		{`"GOOD"`, iec104.QualityGood},
		{`"BAD"`, iec104.QualityInvalid},
		{`"UNCERTAIN"`, iec104.QualityNotTopical},
		{`"STALE"`, iec104.QualityNotTopical},
		{`"SUBSTITUTED"`, iec104.QualitySubstituted},
		{`"BLOCKED"`, iec104.QualityBlocked},
		{`"unknown"`, iec104.QualityGood},
	}
	for _, c := range cases {
		got := parseMQTTQuality([]byte(c.json))
		if got != c.want {
			t.Errorf("parseMQTTQuality(%s) = 0x%02x, want 0x%02x", c.json, got, c.want)
		}
	}
}

func TestParseAndDispatch_Quality(t *testing.T) {
	dbh, _ := db.Open(":memory:")
	defer dbh.Close()
	hist := NewHistoryLogger(dbh, 64, 10, 100*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hist.Run(ctx)

	dbh.MustExec(`INSERT INTO devices(id,server_id,name) VALUES(1,1,'dev')`)
	dbh.MustExec(`INSERT INTO topics(id,device_id,topic,enabled) VALUES(1,1,'t',1)`)

	t.Run("payload_level_integer", func(t *testing.T) {
		srv := &fakeSrv{}
		payload := `{"quality":128,"power":1.0}`
		maps := []TopicMapping{
			{MappingID: 1, ServerID: 1, JSONKey: "power", IEC104Type: "M_ME_NC_1", IOA: 1, Scale: 1},
		}
		dbh.MustExec(`INSERT OR IGNORE INTO signal_mappings(id,server_id,topic_id,json_key,iec104_type,ioa,scale,enabled) VALUES(1,1,1,'power','M_ME_NC_1',1,1,1)`)
		ParseAndDispatch("t", []byte(payload), maps, srv, hist)
		if len(srv.points) != 1 || srv.points[0].Quality != iec104.QualityInvalid {
			t.Errorf("want QualityInvalid (0x80), got 0x%02x", srv.points[0].Quality)
		}
	})

	t.Run("payload_level_string", func(t *testing.T) {
		srv := &fakeSrv{}
		payload := `{"quality":"UNCERTAIN","power":2.0}`
		maps := []TopicMapping{
			{MappingID: 1, ServerID: 1, JSONKey: "power", IEC104Type: "M_ME_NC_1", IOA: 1, Scale: 1},
		}
		ParseAndDispatch("t", []byte(payload), maps, srv, hist)
		if len(srv.points) != 1 || srv.points[0].Quality != iec104.QualityNotTopical {
			t.Errorf("want QualityNotTopical (0x40), got 0x%02x", srv.points[0].Quality)
		}
	})

	t.Run("per_signal_key_overrides_payload", func(t *testing.T) {
		srv := &fakeSrv{}
		// payload quality is GOOD but per-signal key says BAD
		payload := `{"quality":0,"power":3.0,"power_q":128}`
		maps := []TopicMapping{
			{MappingID: 1, ServerID: 1, JSONKey: "power", QualityKey: "power_q", IEC104Type: "M_ME_NC_1", IOA: 1, Scale: 1},
		}
		ParseAndDispatch("t", []byte(payload), maps, srv, hist)
		if len(srv.points) != 1 || srv.points[0].Quality != iec104.QualityInvalid {
			t.Errorf("want QualityInvalid from per-signal key, got 0x%02x", srv.points[0].Quality)
		}
	})

	t.Run("no_quality_key_defaults_good", func(t *testing.T) {
		srv := &fakeSrv{}
		payload := `{"power":4.0}`
		maps := []TopicMapping{
			{MappingID: 1, ServerID: 1, JSONKey: "power", IEC104Type: "M_ME_NC_1", IOA: 1, Scale: 1},
		}
		ParseAndDispatch("t", []byte(payload), maps, srv, hist)
		if len(srv.points) != 1 || srv.points[0].Quality != iec104.QualityGood {
			t.Errorf("want QualityGood, got 0x%02x", srv.points[0].Quality)
		}
	})
}

// Compile-time check: fakeSrv satisfies iec104.Server.
var _ iec104.Server = (*fakeSrv)(nil)
