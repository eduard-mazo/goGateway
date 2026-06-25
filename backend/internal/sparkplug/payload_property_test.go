package sparkplug

import "testing"

// TestParsePropertyValueTyped verifies that typed PropertyValues (not just
// string_value) are read and stringified — this is how the goMqttDnp3 gateway
// carries the SCADA `quality` code and `dnp3.*` flags. See the shared contract
// in docs/sparkplug-contract.md.
func TestParsePropertyValueTyped(t *testing.T) {
	cases := []struct {
		name string
		pv   []byte
		want string
	}{
		{
			name: "int_value (quality=192)",
			pv:   appendVarintField(appendVarintField(nil, 1, uint64(DtUInt32)), 3, 192),
			want: "192",
		},
		{
			name: "long_value",
			pv:   appendVarintField(appendVarintField(nil, 1, uint64(DtUInt64)), 4, 4096),
			want: "4096",
		},
		{
			name: "boolean_value true (dnp3.online)",
			pv:   appendVarintField(appendVarintField(nil, 1, uint64(DtBoolean)), 7, 1),
			want: "true",
		},
		{
			name: "boolean_value false",
			pv:   appendVarintField(appendVarintField(nil, 1, uint64(DtBoolean)), 7, 0),
			want: "false",
		},
		{
			name: "string_value (engUnit)",
			pv:   appendStrField(appendVarintField(nil, 1, uint64(DtString)), 8, "kWh"),
			want: "kWh",
		},
	}
	for _, c := range cases {
		if got := parsePropertyValueString(c.pv); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestParsePropertySetTypedRoundTrip builds a full PropertySet with mixed value
// types (mirroring mapping.qualityProperties on the producer) and checks every
// key is decoded.
func TestParsePropertySetTypedRoundTrip(t *testing.T) {
	pvQuality := appendVarintField(appendVarintField(nil, 1, uint64(DtUInt32)), 3, 192)
	pvOnline := appendVarintField(appendVarintField(nil, 1, uint64(DtBoolean)), 7, 1)
	pvUnit := appendStrField(appendVarintField(nil, 1, uint64(DtString)), 8, "degC")

	var ps []byte
	ps = appendStrField(ps, 1, "quality")
	ps = appendBytesField(ps, 2, pvQuality)
	ps = appendStrField(ps, 1, "dnp3.online")
	ps = appendBytesField(ps, 2, pvOnline)
	ps = appendStrField(ps, 1, "engUnit")
	ps = appendBytesField(ps, 2, pvUnit)

	got := parsePropertySetStrings(ps)
	want := map[string]string{"quality": "192", "dnp3.online": "true", "engUnit": "degC"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}
