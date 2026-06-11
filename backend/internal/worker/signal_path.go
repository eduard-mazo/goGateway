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
// Contract v3 (§5.1): codigo is the LEAF attribute only; instance is the folder
// path between the entity and the leaf ("default" when flat). For host
// telemetry the producer's cosmetic metricPrefix ("System/"/"SYSTEM/") is
// stripped before the split.
//
//	System/CPU/Usage_pct           → {"Usage_pct",  "CPU"}
//	SYSTEM/Memory/Free_MB          → {"Free_MB",    "Memory"}
//	System/Network/docker0/Rx_MB   → {"Rx_MB",      "Network/docker0"}
//	System/Uptime_h                → {"Uptime_h",   "default"}
//	tank_level                     → {"tank_level", "default"}
//	PLC/tank_level                 → {"tank_level", "PLC"}
//	VALV/VALV_ON                   → {"VALV_ON",    "VALV"}
//	Feeder1/PhaseA/Voltage         → {"Voltage",    "Feeder1/PhaseA"}
func parseFiwareSignal(metricName string, isDevice bool) signalRef {
	name := metricName
	if !isDevice && isHostMetric(metricName) {
		name = trimHostPrefix(metricName)
	}
	parts := splitClean(name)
	switch len(parts) {
	case 0:
		return signalRef{Codigo: invalidCode, Instance: instanceDefault}
	case 1:
		return signalRef{Codigo: parts[0], Instance: instanceDefault}
	default:
		return signalRef{
			Codigo:   parts[len(parts)-1],
			Instance: strings.Join(parts[:len(parts)-1], "/"),
		}
	}
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
