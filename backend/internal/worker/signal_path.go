package worker

import "strings"

// signalRef is the FIWARE/UNS decomposition of a Sparkplug metric name into an
// Attribute (→ ssfv.tbl_senales.codigo_senal) and an Entity Instance / channel
// (→ ssfv.tbl_senales_x_equipo.nombre_instancia). The catalog match identity is
// the triple (entity topic, codigo, instance).
type signalRef struct {
	Codigo   string
	Instance string
}

// instanceDefault populates nombre_instancia for metrics with no instance
// dimension, so the composite key is always fully specified (never empty).
const instanceDefault = "default"

// invalidCode marks an unparseable metric name; the caller quarantines it.
const invalidCode = "_invalid"

// parseFiwareSignal maps a Sparkplug metric name to (codigo, instance).
//
// It is the FALLBACK used when the producer does not declare the decomposition
// via uns/* metric properties (legacy or third-party producers). When those
// properties are present the consumer uses them directly and never calls this.
//
//	Rule A — System/host telemetry (node scope), Category[/Instance]/Attribute:
//	  System/CPU/Usage_pct           → {"CPU/Usage_pct",   "default"}
//	  System/Network/docker0/Rx_MB   → {"Network/Rx_MB",   "docker0"}
//	  System/Disk/root/Used_pct      → {"Disk/Used_pct",   "root"}
//	  System/Uptime_h                → {"Uptime_h",        "default"}
//	Rule B — flat node metric:
//	  tank_level                     → {"tank_level",      "default"}
//	Rule C — device metric (topic is the entity; leading segment = sub-instance):
//	  Energy_kWh                     → {"Energy_kWh",      "default"}
//	  Feeder1/Voltage                → {"Voltage",         "Feeder1"}
//	  Feeder1/PhaseA/Voltage         → {"PhaseA/Voltage",  "Feeder1"}
func parseFiwareSignal(metricName string, isDevice bool) signalRef {
	parts := splitClean(metricName)
	if len(parts) == 0 {
		return signalRef{Codigo: invalidCode, Instance: instanceDefault}
	}

	// Rule A: System/host telemetry uses Category[/Instance]/Attribute.
	if !isDevice && isHostMetric(metricName) {
		hp := splitClean(strings.TrimPrefix(metricName, "System/"))
		switch len(hp) {
		case 0:
			return signalRef{Codigo: invalidCode, Instance: instanceDefault}
		case 1: // System/Uptime_h
			return signalRef{Codigo: hp[0], Instance: instanceDefault}
		case 2: // System/CPU/Usage_pct — scalar category, no instance
			return signalRef{Codigo: hp[0] + "/" + hp[1], Instance: instanceDefault}
		default: // System/Network/docker0/Rx_MB[/…] — Category / Instance / Attribute…
			return signalRef{
				Codigo:   hp[0] + "/" + strings.Join(hp[2:], "/"),
				Instance: hp[1],
			}
		}
	}

	// Rule B / C: flat or device metric. A leading '/' segment is a sub-instance.
	if len(parts) == 1 {
		return signalRef{Codigo: parts[0], Instance: instanceDefault}
	}
	return signalRef{Codigo: strings.Join(parts[1:], "/"), Instance: parts[0]}
}

// MatchToken builds the catalog match key (→ nombre_instancia) from the FIWARE
// attribute (codigo) and entity instance (C2 backward-safe encoding).
//
// A "default"/empty instance yields the bare codigo, so flat signals match
// exactly as before (no migration, JSON path untouched). A real instance is
// appended as "codigo@instance" to make the key unique per channel — e.g.
// Network/Rx_MB on docker0 → "Network/Rx_MB@docker0". codigo_senal still stores
// the clean attribute; nombre_instancia stores this token.
func MatchToken(codigo, instance string) string {
	if instance == "" || instance == instanceDefault {
		return codigo
	}
	return codigo + "@" + instance
}

// splitClean splits on '/' and drops empty segments — robust against leading,
// trailing, and doubled slashes ("//a/", "/a//b").
func splitClean(s string) []string {
	raw := strings.Split(s, "/")
	out := raw[:0]
	for _, p := range raw {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
