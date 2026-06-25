package worker

import "testing"

// TestJSONSignalParts pins the C1 mapping from a JSON-mode field key to the
// composite (codigo, instance): a numeric-suffixed key is an indexed channel
// (base code + the channel as the instance); anything else is a flat signal on
// the "default" instance. This is what worker.Handle feeds the composite resolve;
// it must stay aligned with the catalog (validated against industrial_data:
// IA→(IA,default)→eq#1, IDC_1→(IDC_x,IDC_1)→eq#23).
func TestJSONSignalParts(t *testing.T) {
	cases := []struct {
		key      string
		codigo   string
		instance string
	}{
		// Indexed channels — last "_<n>" segment is the channel.
		{"IDC_1", "IDC_x", "IDC_1"},
		{"IDC_2", "IDC_x", "IDC_2"},
		{"IDC_12", "IDC_x", "IDC_12"},
		{"TEMP_MOD_3", "TEMP_MOD_x", "TEMP_MOD_3"}, // multi-underscore base

		// Flat signals — no numeric suffix → instance "default".
		{"AP", "AP", "default"},
		{"AL_COM", "AL_COM", "default"},     // underscore but non-numeric tail
		{"Energy_kWh", "Energy_kWh", "default"},
		{"IA", "IA", "default"},

		// Edge cases.
		{"IDC_0", "IDC_0", "default"}, // channel 0 is invalid (CHECK >=1) → treated flat
		{"", "", "default"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			codigo, instance := jsonSignalParts(tc.key)
			if codigo != tc.codigo || instance != tc.instance {
				t.Fatalf("jsonSignalParts(%q) = (%q, %q); want (%q, %q)",
					tc.key, codigo, instance, tc.codigo, tc.instance)
			}
		})
	}
}
