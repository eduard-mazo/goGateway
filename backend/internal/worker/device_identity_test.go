package worker

import (
	"testing"

	"goGateway/internal/sparkplug"
)

// collectMetricMeta must capture the string value only for device-identity
// metrics, so the approve UI can show the reported hardware identity.
func TestCollectMetricMetaIdentityValue(t *testing.T) {
	metrics := []sparkplug.Metric{
		{Name: "System/Device/PartNumber", StringValue: "ICR-3232"},
		{Name: "System/Device/Firmware", StringValue: "6.6.1 (2026-04-24)"},
		{Name: "PLC/CAUDAL"},                          // process signal: no value capture
		{Name: "System/CPU/Usage_pct"},                // host metric: no value capture
	}
	meta := collectMetricMeta(metrics)
	got := map[string]string{}
	for _, m := range meta {
		got[m.Name] = m.Value
	}
	if got["System/Device/PartNumber"] != "ICR-3232" {
		t.Errorf("PartNumber value = %q, want ICR-3232", got["System/Device/PartNumber"])
	}
	if got["System/Device/Firmware"] != "6.6.1 (2026-04-24)" {
		t.Errorf("Firmware value = %q, want 6.6.1 (2026-04-24)", got["System/Device/Firmware"])
	}
	if got["PLC/CAUDAL"] != "" || got["System/CPU/Usage_pct"] != "" {
		t.Errorf("non-identity metrics should carry no value, got %q / %q",
			got["PLC/CAUDAL"], got["System/CPU/Usage_pct"])
	}
}

func TestParseDeviceIdentity(t *testing.T) {
	cases := []struct {
		name   string
		metric string
		wantCol string
		wantOK  bool
	}{
		{"prefixed part number", "SYSTEM/Device/PartNumber", "hw_part_number", true},
		{"lowercase system prefix", "System/Device/Firmware", "hw_firmware", true},
		{"no prefix", "Device/UUID", "hw_uuid", true},
		{"product type", "SYSTEM/Device/ProductType", "hw_product_type", true},
		{"product name", "SYSTEM/Device/ProductName", "hw_product_name", true},
		{"serial", "SYSTEM/Device/Serial", "hw_serial", true},
		{"unknown device field", "SYSTEM/Device/Mac", "", false},
		{"host metric, not identity", "SYSTEM/CPU/Usage_pct", "", false},
		{"process signal", "Feeder1/PhaseA/Voltage", "", false},
		{"device prefix but deeper", "SYSTEM/Device/Sub/PartNumber", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			col, ok := parseDeviceIdentity(c.metric)
			if ok != c.wantOK || col != c.wantCol {
				t.Fatalf("parseDeviceIdentity(%q) = (%q, %v), want (%q, %v)",
					c.metric, col, ok, c.wantCol, c.wantOK)
			}
		})
	}
}
