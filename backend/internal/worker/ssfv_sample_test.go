package worker

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSSFVSubjectHelpers(t *testing.T) {
	if s := ssfvSubject("GW"); s != "GW.ssfv.sample" {
		t.Errorf("ssfvSubject = %q", s)
	}
	if f := SSFVSubjectFilter("GW"); f != "GW.ssfv.>" {
		t.Errorf("SSFVSubjectFilter = %q", f)
	}
}

func TestSSFVSampleRoundTrip(t *testing.T) {
	in := SSFVSample{
		Entity: "EPM_SOAK/edge-1/meter-01", Codigo: "Energy_kWh", Instance: "Medidas",
		Value: 12.25, Quality: 192, Timestamp: time.Unix(1_780_000_000, 0).UTC(),
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out SSFVSample
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}
