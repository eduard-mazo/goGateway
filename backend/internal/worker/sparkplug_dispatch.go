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
	// rebirthFn is called when the handler needs to publish an NCMD Rebirth.
	// The mqtt.Manager sets this field after creating the handler.
	rebirthFn func(groupID, nodeID string)
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

// SetSSFVHandler wires an SSFV handler so that metrics whose UNS path matches
// a known SSFV equipment topic are routed to TimescaleDB instead of IEC-104.
func (h *SparkplugHandler) SetSSFVHandler(s *SSFVHandler) { h.ssfvHandler = s }

// Dispatch routes a decoded Sparkplug B topic to the appropriate handler.
func (h *SparkplugHandler) Dispatch(topic sparkplug.Topic, raw []byte) {
	switch topic.MsgType {
	case sparkplug.MsgNBIRTH:
		h.handleNBIRTH(topic, raw)
	case sparkplug.MsgNDEATH:
		h.handleNDEATH(topic, raw)
	case sparkplug.MsgNDATA:
		h.handleNDATA(topic, raw)
	case sparkplug.MsgDBIRTH:
		h.handleDBIRTH(topic, raw)
	case sparkplug.MsgDDEATH:
		h.handleDDEATH(topic, raw)
	case sparkplug.MsgDDATA:
		h.handleDDATA(topic, raw)
	// STATE, NCMD, DCMD are not consumed by the gateway in this direction.
	}
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

func (h *SparkplugHandler) handleNBIRTH(topic sparkplug.Topic, raw []byte) {
	p, err := sparkplug.DecodePayload(raw)
	if err != nil {
		log.Printf("sparkplug: NBIRTH decode error (%s/%s): %v", topic.GroupID, topic.EdgeNodeID, err)
		return
	}
	if p.Seq != 0 {
		log.Printf("sparkplug: NBIRTH seq=%d (expected 0) from %s/%s — ignoring",
			p.Seq, topic.GroupID, topic.EdgeNodeID)
		return
	}

	key := sparkplug.NodeKey{GroupID: topic.GroupID, EdgeNodeID: topic.EdgeNodeID}
	session := h.registry.Session(key)
	session.SetBirth(p)

	ts := msToTime(p.Timestamp)

	for i := range p.Metrics {
		m := &p.Metrics[i]
		name := session.ResolveName(m)
		if name == "" || m.IsTransient {
			continue
		}
		h.dispatchMetric(topic, false, name, m, ts)
	}

	log.Printf("sparkplug: NBIRTH %s/%s — %d metric(s)", topic.GroupID, topic.EdgeNodeID, len(p.Metrics))
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

func (h *SparkplugHandler) handleNDATA(topic sparkplug.Topic, raw []byte) {
	p, err := sparkplug.DecodePayload(raw)
	if err != nil {
		log.Printf("sparkplug: NDATA decode error (%s/%s): %v", topic.GroupID, topic.EdgeNodeID, err)
		return
	}

	key := sparkplug.NodeKey{GroupID: topic.GroupID, EdgeNodeID: topic.EdgeNodeID}
	session := h.registry.Session(key)

	if !session.AdvanceSeq(p.Seq) {
		log.Printf("sparkplug: NDATA out-of-sequence from %s/%s (got %d) — requesting rebirth",
			topic.GroupID, topic.EdgeNodeID, p.Seq)
		if h.rebirthFn != nil {
			h.rebirthFn(topic.GroupID, topic.EdgeNodeID)
		}
		return
	}

	ts := msToTime(p.Timestamp)
	log.Printf("sparkplug: NDATA %s/%s seq=%d — %d metric(s)",
		topic.GroupID, topic.EdgeNodeID, p.Seq, len(p.Metrics))

	for i := range p.Metrics {
		m := &p.Metrics[i]
		name := session.ResolveName(m)
		if name == "" || m.IsTransient {
			log.Printf("sparkplug: NDATA skip metric alias=%d name=%q transient=%v",
				m.Alias, m.Name, m.IsTransient)
			continue
		}
		h.dispatchMetric(topic, false, name, m, ts)
	}
}

// ─── Device-level handlers ───────────────────────────────────────────────────

func (h *SparkplugHandler) handleDBIRTH(topic sparkplug.Topic, raw []byte) {
	p, err := sparkplug.DecodePayload(raw)
	if err != nil {
		log.Printf("sparkplug: DBIRTH decode error (%s/%s/%s): %v",
			topic.GroupID, topic.EdgeNodeID, topic.DeviceID, err)
		return
	}

	key := sparkplug.NodeKey{GroupID: topic.GroupID, EdgeNodeID: topic.EdgeNodeID}
	session := h.registry.Session(key).Device(topic.DeviceID)
	session.SetBirth(p)

	ts := msToTime(p.Timestamp)

	for i := range p.Metrics {
		m := &p.Metrics[i]
		name := session.ResolveName(m)
		if name == "" || m.IsTransient {
			continue
		}
		h.dispatchMetric(topic, true, name, m, ts)
	}

	log.Printf("sparkplug: DBIRTH %s/%s/%s — %d metric(s)",
		topic.GroupID, topic.EdgeNodeID, topic.DeviceID, len(p.Metrics))
}

func (h *SparkplugHandler) handleDDEATH(topic sparkplug.Topic, raw []byte) {
	key := sparkplug.NodeKey{GroupID: topic.GroupID, EdgeNodeID: topic.EdgeNodeID}
	h.registry.Session(key).Device(topic.DeviceID).SetDeath()

	devBase := topic.DeviceBase()
	h.MarkNodeStale(devBase) // reuses same stale-dispatch logic keyed on devBase

	log.Printf("sparkplug: DDEATH %s/%s/%s — all device points marked stale",
		topic.GroupID, topic.EdgeNodeID, topic.DeviceID)
}

func (h *SparkplugHandler) handleDDATA(topic sparkplug.Topic, raw []byte) {
	p, err := sparkplug.DecodePayload(raw)
	if err != nil {
		log.Printf("sparkplug: DDATA decode error (%s/%s/%s): %v",
			topic.GroupID, topic.EdgeNodeID, topic.DeviceID, err)
		return
	}

	key := sparkplug.NodeKey{GroupID: topic.GroupID, EdgeNodeID: topic.EdgeNodeID}
	devSession := h.registry.Session(key).Device(topic.DeviceID)

	if !devSession.AdvanceSeq(p.Seq) {
		log.Printf("sparkplug: DDATA out-of-sequence from %s/%s/%s (got %d) — requesting rebirth",
			topic.GroupID, topic.EdgeNodeID, topic.DeviceID, p.Seq)
		if h.rebirthFn != nil {
			h.rebirthFn(topic.GroupID, topic.EdgeNodeID)
		}
		return
	}

	ts := msToTime(p.Timestamp)

	for i := range p.Metrics {
		m := &p.Metrics[i]
		name := devSession.ResolveName(m)
		if name == "" || m.IsTransient {
			continue
		}
		h.dispatchMetric(topic, true, name, m, ts)
	}
}

// ─── Core dispatch ───────────────────────────────────────────────────────────

// dispatchMetric resolves the metric to signal mappings and forwards values to
// every mapped IEC-104 slave. isDevice=true uses DeviceBase for the cache
// lookup and includes topic.DeviceID in the signal path.
func (h *SparkplugHandler) dispatchMetric(
	topic sparkplug.Topic,
	isDevice bool,
	metricName string,
	m *sparkplug.Metric,
	ts time.Time,
) {
	// SSFV intercept: metric names are full UNS paths (e.g. "EPM/SSFV/.../INV_1/AP").
	// Route to TimescaleDB and skip IEC-104 when the path matches a known SSFV topic.
	if h.ssfvHandler != nil {
		parts := strings.Split(metricName, "/")
		if len(parts) >= 2 {
			mqttTopic := strings.Join(parts[:len(parts)-1], "/")
			code := parts[len(parts)-1]
			val, _ := m.Float64()
			if h.ssfvHandler.HandleMetric(mqttTopic, code, val, ts) {
				return
			}
		}
	}

	var nodeBase string
	if isDevice && topic.DeviceID != "" {
		nodeBase = topic.DeviceBase()
	} else {
		nodeBase = topic.NodeBase()
	}

	maps := h.cache.LookupByMetric(nodeBase, metricName)
	if len(maps) == 0 {
		log.Printf("sparkplug: no mapping for %q in %s", metricName, nodeBase)
		return
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
