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
		// Rule A — System/host telemetry
		{"sys scalar", "System/CPU/Usage_pct", false, "CPU/Usage_pct", "default"},
		{"sys mem", "System/Memory/Used_pct", false, "Memory/Used_pct", "default"},
		{"sys net channel", "System/Network/docker0/Rx_MB", false, "Network/Rx_MB", "docker0"},
		{"sys net rate channel", "System/Network/docker0/RxRate_kbps", false, "Network/RxRate_kbps", "docker0"},
		{"sys disk channel", "System/Disk/root/Used_pct", false, "Disk/Used_pct", "root"},
		{"sys uptime", "System/Uptime_h", false, "Uptime_h", "default"},
		// host metric without the optional System/ prefix
		{"host noprefix", "CPU/Usage_pct", false, "CPU/Usage_pct", "default"},

		// Rule B — flat node metric
		{"flat node", "tank_level", false, "tank_level", "default"},

		// Rule C — device metrics
		{"device flat", "Energy_kWh", true, "Energy_kWh", "default"},
		{"device subcomponent", "Feeder1/Voltage", true, "Voltage", "Feeder1"},
		{"device deep subcomponent", "Feeder1/PhaseA/Voltage", true, "PhaseA/Voltage", "Feeder1"},

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
