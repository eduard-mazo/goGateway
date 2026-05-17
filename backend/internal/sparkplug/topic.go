package sparkplug

import "strings"

// Namespace is the root element for Sparkplug B topics.
const Namespace = "spBv1.0"

// MessageType enumerates the valid message_type elements.
type MessageType string

const (
	MsgNBIRTH MessageType = "NBIRTH"
	MsgNDEATH MessageType = "NDEATH"
	MsgNDATA  MessageType = "NDATA"
	MsgDBIRTH MessageType = "DBIRTH"
	MsgDDEATH MessageType = "DDEATH"
	MsgDDATA  MessageType = "DDATA"
	MsgNCMD   MessageType = "NCMD"
	MsgDCMD   MessageType = "DCMD"
	MsgSTATE  MessageType = "STATE"
)

// Topic holds the parsed elements of a Sparkplug B topic string.
// Format: spBv1.0/group_id/message_type/edge_node_id[/device_id]
type Topic struct {
	GroupID    string
	MsgType    MessageType
	EdgeNodeID string
	DeviceID   string // empty for node-level messages
}

// ParseTopic parses a raw MQTT topic string into a Topic.
// Returns (topic, true) on success, (zero, false) if the string is not a
// valid spBv1.0 topic.
func ParseTopic(raw string) (Topic, bool) {
	parts := strings.SplitN(raw, "/", 5)
	if len(parts) < 4 {
		return Topic{}, false
	}
	if parts[0] != Namespace {
		return Topic{}, false
	}
	t := Topic{
		GroupID:    parts[1],
		MsgType:    MessageType(parts[2]),
		EdgeNodeID: parts[3],
	}
	if len(parts) == 5 {
		t.DeviceID = parts[4]
	}
	return t, true
}

// NodeBase returns the canonical base path for this node as stored in the
// topics table: "spBv1.0/{group}/{node}".
// Used as the cache lookup key when matching Sparkplug B signal mappings.
func (t Topic) NodeBase() string {
	return Namespace + "/" + t.GroupID + "/" + t.EdgeNodeID
}

// DeviceBase returns the canonical base path for this device:
// "spBv1.0/{group}/{node}/{device}".  Returns NodeBase when DeviceID is empty.
func (t Topic) DeviceBase() string {
	if t.DeviceID == "" {
		return t.NodeBase()
	}
	return t.NodeBase() + "/" + t.DeviceID
}

// StateTopicFor returns the STATE topic for a Primary Application host ID.
// Format per spec §7.5: "STATE/{scada_host_id}"  (no spBv1.0 namespace prefix).
func StateTopicFor(hostID string) string {
	return "STATE/" + hostID
}

// WildcardFor returns the MQTT subscription wildcard that captures all
// Sparkplug B messages for a given group: "spBv1.0/{groupID}/#".
// Use "#" as groupID to subscribe to every group.
func WildcardFor(groupID string) string {
	return Namespace + "/" + groupID + "/#"
}

// IsNodeLevel reports whether the message is addressed to the EoN node itself
// (no device_id component).
func (t Topic) IsNodeLevel() bool { return t.DeviceID == "" }
