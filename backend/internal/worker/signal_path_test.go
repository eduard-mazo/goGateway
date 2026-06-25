package worker

import "testing"

func TestParseFiwareSignal(t *testing.T) {
	cases := []struct {
		name     string
		metric   string
		isDevice bool
		code     string
		instance string
	}{
		// Host telemetry — cosmetic System/SYSTEM prefix stripped, then
		// codigo = leaf, instance = folder path (contract v3 §5.1).
		{"sys scalar", "System/CPU/Usage_pct", false, "Usage_pct", "CPU"},
		{"sys scalar upper prefix", "SYSTEM/CPU/Usage_pct", false, "Usage_pct", "CPU"},
		{"sys mem", "SYSTEM/Memory/Free_MB", false, "Free_MB", "Memory"},
		{"sys net channel", "System/Network/docker0/Rx_MB", false, "Rx_MB", "Network/docker0"},
		{"sys net rate channel", "System/Network/docker0/RxRate_kbps", false, "RxRate_kbps", "Network/docker0"},
		{"sys disk channel", "System/Disk/root/Used_pct", false, "Used_pct", "Disk/root"},
		{"sys uptime", "System/Uptime_h", false, "Uptime_h", "default"},
		// host metric without the optional System/ prefix
		{"host noprefix", "CPU/Usage_pct", false, "Usage_pct", "CPU"},

		// Flat node metric
		{"flat node", "tank_level", false, "tank_level", "default"},

		// Folder-grouped process metrics (node or device)
		{"node folder", "PLC/tank_level", false, "tank_level", "PLC"},
		{"device folder", "VALV/VALV_ON", true, "VALV_ON", "VALV"},

		// Device metrics
		{"device flat", "Energy_kWh", true, "Energy_kWh", "default"},
		{"device subcomponent", "Feeder1/Voltage", true, "Voltage", "Feeder1"},
		{"device deep subcomponent", "Feeder1/PhaseA/Voltage", true, "Voltage", "Feeder1/PhaseA"},

		// Robustness — malformed paths
		{"trailing slash", "Feeder1/Voltage/", true, "Voltage", "Feeder1"},
		{"leading slash", "/Feeder1/Voltage", true, "Voltage", "Feeder1"},
		{"double slash", "Feeder1//Voltage", true, "Voltage", "Feeder1"},
		{"empty", "", false, invalidCode, instanceDefault},
		{"only slashes", "///", false, invalidCode, instanceDefault},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFiwareSignal(tc.metric, tc.isDevice)
			if got.Codigo != tc.code || got.Instance != tc.instance {
				t.Fatalf("parseFiwareSignal(%q, dev=%v) = {%q, %q}; want {%q, %q}",
					tc.metric, tc.isDevice, got.Codigo, got.Instance, tc.code, tc.instance)
			}
		})
	}
}

func TestTrimHostPrefix(t *testing.T) {
	cases := map[string]string{
		"System/CPU/Usage_pct": "CPU/Usage_pct",
		"SYSTEM/CPU/Usage_pct": "CPU/Usage_pct",
		"system/Uptime_h":      "Uptime_h",
		"CPU/Usage_pct":        "CPU/Usage_pct",
		"Systema/x":            "Systema/x", // not the prefix — different segment
		"Uptime_h":             "Uptime_h",
	}
	for in, want := range cases {
		if got := trimHostPrefix(in); got != want {
			t.Errorf("trimHostPrefix(%q) = %q; want %q", in, got, want)
		}
	}
}
