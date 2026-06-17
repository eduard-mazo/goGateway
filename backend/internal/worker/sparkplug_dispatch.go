package worker

import (
	"log"
	"strings"
	"time"

	"goGateway/internal/iec104"
	"goGateway/internal/sparkplug"
)

// SparkplugHandler processes incoming Sparkplug B MQTT messages and dispatches
// decoded metric values to the IEC 60870-5-104 slave fleet.
//
// The gateway acts as a Sparkplug B Primary Application (SCADA Host):
//   - It does NOT publish NBIRTH/NDATA of its own.
//   - It subscribes to EoN node traffic and forwards metrics to IEC-104.
//   - It publishes STATE (ONLINE/OFFLINE) via the MQTT client.
//   - It publishes NCMD Rebirth when an out-of-sequence NDATA is detected.
type SparkplugHandler struct {
	registry    *sparkplug.Registry
	cache       *MappingCache
	d           Dispatcher
	ssfvHandler *SSFVHandler
	autoDisc    *AutoDiscoveryService
	// rebirthFn is called when the handler needs to publish an NCMD Rebirth.
	// The mqtt.Manager sets this field after creating the handler.
	rebirthFn func(groupID, nodeID string)
	// dupFn reports a suspected duplicate node id (two producers on one topic).
	// Optional; set by mqtt.Manager.
	dupFn func(groupID, nodeID, reason string)
}

// NewSparkplugHandler returns a handler ready to process Sparkplug B messages.
func NewSparkplugHandler(
	registry *sparkplug.Registry,
	cache *MappingCache,
	d Dispatcher,
) *SparkplugHandler {
	return &SparkplugHandler{
		registry: registry,
		cache:    cache,
		d:        d,
	}
}

// SetRebirthFn sets the callback used to publish an NCMD Rebirth when an
// out-of-sequence message is detected.  The callback is provided by
// mqtt.Manager after the handler is created.
func (h *SparkplugHandler) SetRebirthFn(fn func(groupID, nodeID string)) {
	h.rebirthFn = fn
}

// SetDuplicateFn registers a callback invoked when a duplicate node id is
// suspected (e.g. a bdSeq regression while the node is online). Optional.
func (h *SparkplugHandler) SetDuplicateFn(fn func(groupID, nodeID, reason string)) {
	h.dupFn = fn
}

// SetSSFVHandler wires an SSFV handler so that metrics whose UNS path matches
// a known SSFV equipment topic are routed to TimescaleDB instead of IEC-104.
func (h *SparkplugHandler) SetSSFVHandler(s *SSFVHandler) { h.ssfvHandler = s }

// SetAutoDiscovery wires the auto-discovery service so that NBIRTH/DBIRTH events
// from unconfigured nodes are recorded in SQLite for operator review.
func (h *SparkplugHandler) SetAutoDiscovery(a *AutoDiscoveryService) { h.autoDisc = a }

// Dispatch routes a decoded Sparkplug B topic to the appropriate handler.
// Returns the number of metrics forwarded to the SSFV pipeline in this message.
func (h *SparkplugHandler) Dispatch(topic sparkplug.Topic, raw []byte) int {
	switch topic.MsgType {
	case sparkplug.MsgNBIRTH:
		return h.handleNBIRTH(topic, raw)
	case sparkplug.MsgNDEATH:
		h.handleNDEATH(topic, raw)
	case sparkplug.MsgNDATA:
		return h.handleNDATA(topic, raw)
	case sparkplug.MsgDBIRTH:
		return h.handleDBIRTH(topic, raw)
	case sparkplug.MsgDDEATH:
		h.handleDDEATH(topic, raw)
	case sparkplug.MsgDDATA:
		return h.handleDDATA(topic, raw)
	// STATE, NCMD, DCMD are not consumed by the gateway in this direction.
	}
	return 0
}

// MarkNodeStale dispatches QualityNotTopical to every IEC-104 point mapped to
// nodeBase.  Called by mqtt.Manager on connection loss or NDEATH.
// nodeBase = "spBv1.0/{group}/{node}" or "spBv1.0/{group}/{node}/{device}".
func (h *SparkplugHandler) MarkNodeStale(nodeBase string) {
	maps := h.cache.LookupByNode(nodeBase)
	now := time.Now()
	stripped := strings.TrimPrefix(nodeBase, sparkplug.Namespace+"/")
	for _, m := range maps {
		m.SignalPath = m.Business + "/" + m.Company + "/" + stripped + "/" + m.MetricName
		h.d.Dispatch(m, 0, iec104.QualityNotTopical, now)
	}
}

// ─── Node-level handlers ─────────────────────────────────────────────────────

func (h *SparkplugHandler) handleNBIRTH(topic sparkplug.Topic, raw []byte) int {
	p, err := sparkplug.DecodePayload(raw)
	if err != nil {
		log.Printf("sparkplug: NBIRTH decode error (%s/%s): %v", topic.GroupID, topic.EdgeNodeID, err)
		return 0
	}
	if p.Seq != 0 {
		log.Printf("sparkplug: NBIRTH seq=%d (expected 0) from %s/%s — ignoring",
			p.Seq, topic.GroupID, topic.EdgeNodeID)
		return 0
	}

	key := sparkplug.NodeKey{GroupID: topic.GroupID, EdgeNodeID: topic.EdgeNodeID}
	session := h.registry.Session(key)
	if _, regressed := session.SetBirth(p); regressed {
		log.Printf("sparkplug: WARNING NBIRTH bdSeq regressed for %s/%s while online — likely "+
			"DUPLICATE node id (a second producer on the same topic). Sender identity is the "+
			"Sparkplug topic, not the MQTT clientId; enforce per-client broker ACLs.",
			topic.GroupID, topic.EdgeNodeID)
		if h.dupFn != nil {
			h.dupFn(topic.GroupID, topic.EdgeNodeID, "bdseq-regression")
		}
	}

	if h.autoDisc != nil {
		names := collectMetricNames(p.Metrics)
		meta := collectMetricMeta(p.Metrics)
		go h.autoDisc.OnBIRTH(topic.GroupID, topic.EdgeNodeID, "", names, meta, p.Properties)
	}

	ts := msToTime(p.Timestamp)
	ssfvHits := 0

	for i := range p.Metrics {
		m := &p.Metrics[i]
		name := session.ResolveName(m)
		if name == "" || m.IsTransient {
			continue
		}
		if h.dispatchMetric(topic, false, name, m, ts) {
			ssfvHits++
		}
	}

	log.Printf("sparkplug: NBIRTH %s/%s — %d metric(s)", topic.GroupID, topic.EdgeNodeID, len(p.Metrics))
	return ssfvHits
}

func (h *SparkplugHandler) handleNDEATH(topic sparkplug.Topic, raw []byte) {
	key := sparkplug.NodeKey{GroupID: topic.GroupID, EdgeNodeID: topic.EdgeNodeID}
	session := h.registry.Session(key)
	session.SetDeath()

	nodeBase := topic.NodeBase()
	h.MarkNodeStale(nodeBase)

	// Mark all known devices under this node stale too.
	log.Printf("sparkplug: NDEATH %s/%s — all points marked stale", topic.GroupID, topic.EdgeNodeID)
}

func (h *SparkplugHandler) handleNDATA(topic sparkplug.Topic, raw []byte) int {
	p, err := sparkplug.DecodePayload(raw)
	if err != nil {
		log.Printf("sparkplug: NDATA decode error (%s/%s): %v", topic.GroupID, topic.EdgeNodeID, err)
		return 0
	}

	key := sparkplug.NodeKey{GroupID: topic.GroupID, EdgeNodeID: topic.EdgeNodeID}
	session := h.registry.Session(key)

	if !session.AdvanceSeq(p.Seq) {
		log.Printf("sparkplug: NDATA out-of-sequence from %s/%s (got %d) — requesting rebirth",
			topic.GroupID, topic.EdgeNodeID, p.Seq)
		if h.rebirthFn != nil {
			h.rebirthFn(topic.GroupID, topic.EdgeNodeID)
		}
		return 0
	}

	ts := msToTime(p.Timestamp)
	log.Printf("sparkplug: NDATA %s/%s seq=%d — %d metric(s)",
		topic.GroupID, topic.EdgeNodeID, p.Seq, len(p.Metrics))

	ssfvHits := 0
	for i := range p.Metrics {
		m := &p.Metrics[i]
		name := session.ResolveName(m)
		if name == "" || m.IsTransient {
			log.Printf("sparkplug: NDATA skip metric alias=%d name=%q transient=%v",
				m.Alias, m.Name, m.IsTransient)
			continue
		}
		if h.dispatchMetric(topic, false, name, m, ts) {
			ssfvHits++
		}
	}
	return ssfvHits
}

// ─── Device-level handlers ───────────────────────────────────────────────────

func (h *SparkplugHandler) handleDBIRTH(topic sparkplug.Topic, raw []byte) int {
	p, err := sparkplug.DecodePayload(raw)
	if err != nil {
		log.Printf("sparkplug: DBIRTH decode error (%s/%s/%s): %v",
			topic.GroupID, topic.EdgeNodeID, topic.DeviceID, err)
		return 0
	}

	key := sparkplug.NodeKey{GroupID: topic.GroupID, EdgeNodeID: topic.EdgeNodeID}
	nodeSession := h.registry.Session(key)
	session := nodeSession.Device(topic.DeviceID)
	session.SetBirth(p)
	// DBIRTH consumes a node-level sequence number (Sparkplug uses one seq per
	// EoN node across NBIRTH/NDATA/DBIRTH/DDATA). Sync it forward so subsequent
	// NDATA/DDATA validate against the right expected value.
	nodeSession.SyncSeq(p.Seq)

	if h.autoDisc != nil {
		names := collectMetricNames(p.Metrics)
		meta := collectMetricMeta(p.Metrics)
		go h.autoDisc.OnBIRTH(topic.GroupID, topic.EdgeNodeID, topic.DeviceID, names, meta, p.Properties)
	}

	ts := msToTime(p.Timestamp)
	ssfvHits := 0

	for i := range p.Metrics {
		m := &p.Metrics[i]
		name := session.ResolveName(m)
		if name == "" || m.IsTransient {
			continue
		}
		if h.dispatchMetric(topic, true, name, m, ts) {
			ssfvHits++
		}
	}

	log.Printf("sparkplug: DBIRTH %s/%s/%s — %d metric(s)",
		topic.GroupID, topic.EdgeNodeID, topic.DeviceID, len(p.Metrics))
	return ssfvHits
}

func (h *SparkplugHandler) handleDDEATH(topic sparkplug.Topic, raw []byte) {
	key := sparkplug.NodeKey{GroupID: topic.GroupID, EdgeNodeID: topic.EdgeNodeID}
	h.registry.Session(key).Device(topic.DeviceID).SetDeath()

	devBase := topic.DeviceBase()
	h.MarkNodeStale(devBase) // reuses same stale-dispatch logic keyed on devBase

	log.Printf("sparkplug: DDEATH %s/%s/%s — all device points marked stale",
		topic.GroupID, topic.EdgeNodeID, topic.DeviceID)
}

func (h *SparkplugHandler) handleDDATA(topic sparkplug.Topic, raw []byte) int {
	p, err := sparkplug.DecodePayload(raw)
	if err != nil {
		log.Printf("sparkplug: DDATA decode error (%s/%s/%s): %v",
			topic.GroupID, topic.EdgeNodeID, topic.DeviceID, err)
		return 0
	}

	key := sparkplug.NodeKey{GroupID: topic.GroupID, EdgeNodeID: topic.EdgeNodeID}
	nodeSession := h.registry.Session(key)
	devSession := nodeSession.Device(topic.DeviceID)

	// Validate against the NODE seq (one counter per EoN node), not a per-device
	// counter. The device session is used only for alias→name resolution.
	if !nodeSession.AdvanceSeq(p.Seq) {
		log.Printf("sparkplug: DDATA out-of-sequence from %s/%s/%s (got %d) — requesting rebirth",
			topic.GroupID, topic.EdgeNodeID, topic.DeviceID, p.Seq)
		if h.rebirthFn != nil {
			h.rebirthFn(topic.GroupID, topic.EdgeNodeID)
		}
		return 0
	}

	ts := msToTime(p.Timestamp)
	ssfvHits := 0

	for i := range p.Metrics {
		m := &p.Metrics[i]
		name := devSession.ResolveName(m)
		if name == "" || m.IsTransient {
			continue
		}
		if h.dispatchMetric(topic, true, name, m, ts) {
			ssfvHits++
		}
	}
	return ssfvHits
}

// ─── Core dispatch ───────────────────────────────────────────────────────────

// dispatchMetric resolves the metric to signal mappings and forwards values to
// every mapped IEC-104 slave. isDevice=true uses DeviceBase for the cache
// lookup and includes topic.DeviceID in the signal path.
// Returns true when the metric was forwarded to the SSFV pipeline.
func (h *SparkplugHandler) dispatchMetric(
	topic sparkplug.Topic,
	isDevice bool,
	metricName string,
	m *sparkplug.Metric,
	ts time.Time,
) bool {
	// SSFV intercept: write to TimescaleDB (ssfv.tbl_valores) when the metric
	// resolves to a registered catalog signal. Execution continues so an IEC-104
	// mapping can also be served (dual routing). Only signals that have a
	// signal_mappings entry reach IEC-104.
	//
	// System/host metrics (CPU/Memory/Disk/Network/…) use the SAME catalog gating
	// as plant signals: the edge node itself is the host "station"
	// (topic = group/node) and the signal code is the System path with its prefix
	// stripped (e.g. "CPU/Usage_pct"). Register them on that station to persist
	// them to tbl_valores; unregistered host metrics land in Descartados and are
	// not written — identical to process signals.
	ssfvHandled := false
	var ssfvTopic string // the SSFV equipment topic attempted (for log suppression)
	if h.ssfvHandler != nil {
		// C1 composite identity (entity, codigo, instance) — FIWARE Entity →
		// Attribute → channel. Produced from the producer-declared uns/* when
		// present, else from name parsing (fallbacks).
		var entity, codigo, instance string
		nodeEntity := topic.GroupID + "/" + topic.EdgeNodeID
		devEntity := nodeEntity
		if isDevice && topic.DeviceID != "" {
			devEntity = nodeEntity + "/" + topic.DeviceID
		}

		// ICR device hardware identity: <prefix>Device/<Field> string metrics
		// from node System telemetry. Persist to the node's equipo — never a
		// tbl_valores sample or IEC-104 route. Treat as handled when the node is
		// a registered equipo so it doesn't log as an unmapped signal.
		if col, ok := parseDeviceIdentity(metricName); ok {
			h.ssfvHandler.UpdateDeviceIdentity(nodeEntity, col, m.StringValue)
			return h.ssfvHandler.IsKnownTopic(nodeEntity)
		}
		switch {
		case m.Properties["uns/code"] != "":
			// Producer-DECLARED UNS decomposition (Phase 1, contract §5.1):
			// deterministic, no name parsing. Universal across protocols.
			entity = devEntity
			codigo = m.Properties["uns/code"]
			instance = m.Properties["uns/instance"]
		case isHostMetric(metricName):
			// FALLBACK: parse the System path (Category[/Instance]/Attribute).
			entity = nodeEntity
			ref := parseFiwareSignal(metricName, false)
			codigo, instance = ref.Codigo, ref.Instance
		default:
			parts := strings.Split(metricName, "/")
			if len(parts) >= 2 && parts[0] == topic.GroupID {
				// FALLBACK (legacy): full UNS path embedded in the name
				// ("EPM_SSFV/Sede30/INV_1/OSV") → entity = path, attribute = leaf.
				// Only when the name starts with the topic's own group — otherwise
				// a folder-grouped name ("PLC/tank_level") would be misread as a
				// foreign entity.
				entity = strings.Join(parts[:len(parts)-1], "/")
				codigo = parts[len(parts)-1]
				instance = "default"
			} else {
				// FALLBACK: folder-grouped or flat name on the node/device entity
				// (contract v3 §5.1): codigo = leaf, instance = folder path.
				entity = devEntity
				ref := parseFiwareSignal(metricName, isDevice)
				codigo, instance = ref.Codigo, ref.Instance
			}
		}
		if instance == "" {
			instance = "default"
		}
		ssfvTopic = entity
		val, _ := m.Float64()
		ssfvHandled = h.ssfvHandler.HandleMetric(entity, codigo, instance, val, ts)
	}

	var nodeBase string
	if isDevice && topic.DeviceID != "" {
		nodeBase = topic.DeviceBase()
	} else {
		nodeBase = topic.NodeBase()
	}

	maps := h.cache.LookupByMetric(nodeBase, metricName)
	if len(maps) == 0 {
		if !ssfvHandled {
			// Suppress "no mapping" for signals from known SSFV equipment topics:
			// they are recorded in the Descartados ring via HandleMetric's missFn.
			isSsfvEquip := ssfvTopic != "" && h.ssfvHandler.IsKnownTopic(ssfvTopic)
			if !isSsfvEquip {
				log.Printf("sparkplug: no mapping for %q in %s", metricName, nodeBase)
			}
		}
		return ssfvHandled
	}

	val, hasVal := m.Float64()
	quality := m.IEC104Quality()

	for _, tm := range maps {
		tm.SignalPath = spSignalPath(tm.Business, tm.Company, topic, isDevice, metricName)
		scaled := val * tm.Scale
		if hasVal {
			log.Printf("sparkplug: dispatch IOA=%d val=%.4f q=%d path=%s", tm.IOA, scaled, quality, tm.SignalPath)
			h.d.Dispatch(tm, scaled, quality, ts)
		} else {
			log.Printf("sparkplug: dispatch IOA=%d val=null (invalid) path=%s", tm.IOA, tm.SignalPath)
			h.d.Dispatch(tm, 0, iec104.QualityInvalid, ts)
		}
	}
	return ssfvHandled
}

// spSignalPath builds the full signal path for a Sparkplug B metric.
// When metricName is already a full UNS path (starts with business/company),
// it is used directly to avoid duplicate prefix segments.
// Otherwise format is: business/company/group/node[/device]/metricName
func spSignalPath(business, company string, topic sparkplug.Topic, isDevice bool, metricName string) string {
	prefix := business + "/" + company + "/"
	if strings.HasPrefix(metricName, prefix) {
		return metricName
	}
	parts := []string{business, company, topic.GroupID, topic.EdgeNodeID}
	if isDevice && topic.DeviceID != "" {
		parts = append(parts, topic.DeviceID)
	}
	parts = append(parts, metricName)
	return strings.Join(parts, "/")
}

// msToTime converts a Sparkplug B millisecond-epoch timestamp to time.Time.
// Falls back to time.Now() when timestamp is zero (not set in payload).
func msToTime(ms uint64) time.Time {
	if ms == 0 {
		return time.Now()
	}
	return time.UnixMilli(int64(ms))
}

// collectMetricNames extracts non-empty metric names from a Sparkplug B payload.
func collectMetricNames(metrics []sparkplug.Metric) []string {
	out := make([]string, 0, len(metrics))
	for _, m := range metrics {
		if m.Name != "" {
			out = append(out, m.Name)
		}
	}
	return out
}

// collectMetricMeta builds MetricMeta entries from per-metric PropertySets.
// Only metrics with a non-empty Name are included (NBIRTH always carries names).
func collectMetricMeta(metrics []sparkplug.Metric) []MetricMeta {
	out := make([]MetricMeta, 0, len(metrics))
	for _, m := range metrics {
		if m.Name == "" {
			continue
		}
		mm := MetricMeta{Name: m.Name}
		if m.Properties != nil {
			mm.EngUnit = m.Properties["engUnit"]
			mm.TipoVariable = m.Properties["tipo_variable"]
			mm.TipoValor = m.Properties["tipo_valor"]
			// Contract v3 §5: uns/name + uns/description are the canonical
			// birth-only metadata keys; the bare "description" key is the
			// pre-v3 spelling kept as fallback.
			mm.UnsName = m.Properties["uns/name"]
			mm.Description = m.Properties["uns/description"]
			if mm.Description == "" {
				mm.Description = m.Properties["description"]
			}
			mm.UnsCode = m.Properties["uns/code"]
			mm.UnsInstance = m.Properties["uns/instance"]
			mm.DeviceTopic = m.Properties["device_topic"]
			mm.DeviceType = m.Properties["device_type"]
		}
		// Capture the string value for device-identity metrics so the approve UI
		// can display the actual hardware identity (only set for string metrics).
		if _, ok := parseDeviceIdentity(m.Name); ok {
			mm.Value = m.StringValue
		}
		out = append(out, mm)
	}
	return out
}
