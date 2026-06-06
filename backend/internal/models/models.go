// Package models defines the shared domain structs used across db, api, and
// worker. All fields map 1:1 to DB columns via sqlx tags.
package models

import "time"

// --- RBAC roles ---

const (
	RoleSuperAdmin = "superadmin"
	RoleOperator   = "operator"
	RoleViewer     = "viewer"
)

// ValidRole reports whether r is a recognised role string.
func ValidRole(r string) bool {
	return r == RoleSuperAdmin || r == RoleOperator || r == RoleViewer
}

// User is a gateway operator account.  PasswordHash is never serialised to JSON.
type User struct {
	ID           int64     `db:"id"            json:"id"`
	Username     string    `db:"username"      json:"username"`
	PasswordHash string    `db:"password_hash" json:"-"`
	Email        string    `db:"email"         json:"email"`
	FullName     string    `db:"full_name"     json:"full_name"`
	Role         string    `db:"role"          json:"role"`
	Enabled      bool      `db:"enabled"       json:"enabled"`
	CreatedAt    time.Time `db:"created_at"    json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"    json:"updated_at"`
}

// Session is one authenticated login event.  RefreshTokenHash is a bcrypt
// hash of the raw refresh token; the raw token is given to the client only.
type Session struct {
	ID               string    `db:"id"                 json:"id"`
	UserID           int64     `db:"user_id"            json:"user_id"`
	RefreshTokenHash string    `db:"refresh_token_hash" json:"-"`
	UserAgent        string    `db:"user_agent"         json:"user_agent"`
	RemoteIP         string    `db:"remote_ip"          json:"remote_ip"`
	ExpiresAt        time.Time `db:"expires_at"         json:"expires_at"`
	Revoked          bool      `db:"revoked"            json:"revoked"`
	CreatedAt        time.Time `db:"created_at"         json:"created_at"`
}

// SignalThreshold defines Hi/Lo alarm limits for one signal mapping.
// Each limit is optional (NULL = disabled).  AlarmIOA fields carry the IEC-104
// IOA that is toggled when the corresponding level is breached.
type SignalThreshold struct {
	ID            int64    `db:"id"              json:"id"`
	MappingID     int64    `db:"mapping_id"      json:"mapping_id"`
	HHValue       *float64 `db:"hh_value"        json:"hh_value"`
	HValue        *float64 `db:"h_value"         json:"h_value"`
	LValue        *float64 `db:"l_value"         json:"l_value"`
	LLValue       *float64 `db:"ll_value"        json:"ll_value"`
	HHAlarmIOA    int      `db:"hh_alarm_ioa"    json:"hh_alarm_ioa"`
	HAlarmIOA     int      `db:"h_alarm_ioa"     json:"h_alarm_ioa"`
	LAlarmIOA     int      `db:"l_alarm_ioa"     json:"l_alarm_ioa"`
	LLAlarmIOA    int      `db:"ll_alarm_ioa"    json:"ll_alarm_ioa"`
	AlarmServerID int64    `db:"alarm_server_id" json:"alarm_server_id"`
	Deadband      float64  `db:"deadband"        json:"deadband"`
	Enabled       bool     `db:"enabled"         json:"enabled"`
}

// CalculatedSignal defines a virtual signal derived from two real IOAs.
// Operator is one of: +  -  *  /  abs (unary — operand B ignored).
type CalculatedSignal struct {
	ID            int64   `db:"id"               json:"id"`
	Name          string  `db:"name"             json:"name"`
	Operator      string  `db:"operator"         json:"operator"`
	OperandAServer int64  `db:"operand_a_server" json:"operand_a_server"`
	OperandAIOA   int     `db:"operand_a_ioa"    json:"operand_a_ioa"`
	OperandBServer *int64 `db:"operand_b_server" json:"operand_b_server"`
	OperandBIOA   *int    `db:"operand_b_ioa"    json:"operand_b_ioa"`
	Scale         float64 `db:"scale"            json:"scale"`
	OutServerID   int64   `db:"out_server_id"    json:"out_server_id"`
	OutIOA        int     `db:"out_ioa"          json:"out_ioa"`
	OutType       string  `db:"out_type"         json:"out_type"`
	Enabled       bool    `db:"enabled"          json:"enabled"`
}

// AlarmEvent is one transition in a signal's alarm state.
type AlarmEvent struct {
	ID           int64      `db:"id"             json:"id"`
	MappingID    int64      `db:"mapping_id"     json:"mapping_id"`
	Level        string     `db:"level"          json:"level"`
	Value        float64    `db:"value"          json:"value"`
	Timestamp    time.Time  `db:"timestamp"      json:"timestamp"`
	Acknowledged bool       `db:"acknowledged"   json:"acknowledged"`
	AckAt        *time.Time `db:"ack_at"         json:"ack_at"`
}

// Device = logical device (inverter, weather station, ...) on one IEC-104
// slave. ServerID pins the device to a server row; topics + mappings under
// it inherit that scope. Same physical asset on two SCADA endpoints = two
// device rows.
type Device struct {
	ID          int64     `db:"id" json:"id"`
	ServerID    int64     `db:"server_id" json:"server_id"`
	Name        string    `db:"name" json:"name"`
	Description string    `db:"description" json:"description"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

// MQTTConfig = broker connection params. Single row expected (id=1).
// When SparkplugEnabled is true the client operates in Sparkplug B mode:
// topics are spBv1.0 namespaced, payloads are protobuf, and signal mappings
// are matched by MetricName instead of JSONKey.
type MQTTConfig struct {
	ID               int64  `db:"id"                json:"id"`
	Host             string `db:"host"              json:"host"`
	Port             int    `db:"port"              json:"port"`
	Username         string `db:"username"          json:"username"`
	Password         string `db:"password"          json:"password"`
	ClientID         string `db:"client_id"         json:"client_id"`
	UseTLS           bool   `db:"use_tls"           json:"use_tls"`
	SparkplugEnabled bool   `db:"sparkplug_enabled" json:"sparkplug_enabled"`
	SpGroupID        string `db:"sp_group_id"       json:"sp_group_id"`
	SpHostID         string `db:"sp_host_id"        json:"sp_host_id"`
	SpTopics         string `db:"sp_topics"          json:"sp_topics"`
	QoS              int    `db:"qos"               json:"qos"`

	// TLS (used when UseTLS). CAFile enables a private/self-signed broker CA;
	// CertFile+KeyFile enable mutual TLS; TLSInsecure skips verification.
	CAFile      string `db:"tls_ca_file"   json:"tls_ca_file"`
	CertFile    string `db:"tls_cert_file" json:"tls_cert_file"`
	KeyFile     string `db:"tls_key_file"  json:"tls_key_file"`
	TLSInsecure bool   `db:"tls_insecure"  json:"tls_insecure"`
}

// Topic = MQTT subscription bound to a device.
// PayloadFormat selects the ingestion decoder: "json" (default) or "sparkplug".
// Sparkplug topics carry protobuf payloads matched by MetricName; JSON topics
// are decoded by JSONKey.  The gateway MQTT client still uses the global
// SparkplugEnabled flag to switch protocol mode, but per-topic format allows
// mixing JSON and Sparkplug streams on the same broker.
type Topic struct {
	ID            int64  `db:"id"             json:"id"`
	DeviceID      int64  `db:"device_id"      json:"device_id"`
	Topic         string `db:"topic"          json:"topic"`
	QoS           int    `db:"qos"            json:"qos"`
	Enabled       bool   `db:"enabled"        json:"enabled"`
	PayloadFormat string `db:"payload_format" json:"payload_format"`
}

// GatewaySignal is the normalisation layer between an inbound MQTT topic stream
// and the rest of the pipeline.  It defines one logical measurement point:
//   - json_key or metric_name identifies the value inside the payload.
//   - persist_to_db=true tells the history writer to record samples in SQLite.
//   - A corresponding signal_mappings row (signal_id FK) exposes it on IEC-104.
type GatewaySignal struct {
	ID             int64   `db:"id"             json:"id"`
	TopicID        int64   `db:"topic_id"       json:"topic_id"`
	Name           string  `db:"name"           json:"name"`
	JSONKey        string  `db:"json_key"       json:"json_key"`
	MetricName     string  `db:"metric_name"    json:"metric_name"`
	QualityKey     string  `db:"quality_key"    json:"quality_key"`
	VariableType   string  `db:"variable_type"  json:"variable_type"`
	Characteristic string  `db:"characteristic" json:"characteristic"`
	Unit           string  `db:"unit"           json:"unit"`
	Scale          float64 `db:"scale"          json:"scale"`
	PersistToDB    bool    `db:"persist_to_db"  json:"persist_to_db"`
	Enabled        bool    `db:"enabled"        json:"enabled"`
	CreatedAt      time.Time `db:"created_at"   json:"created_at"`
}

// IEC104Gateway = host-wide IEC-104 settings. Singleton (id=1). ListenIP is
// the local NIC IP that every slave endpoint binds on; SCADA masters reach
// the gateway at ListenIP:<server.Port>. Use "0.0.0.0" to bind every NIC.
type IEC104Gateway struct {
	ID       int64  `db:"id"        json:"id"`
	ListenIP string `db:"listen_ip" json:"listen_ip"`
}

// IEC104Server = one passive IEC 60870-5-104 slave endpoint.
// Multiple rows yield a fleet: each exposes the full point set under its own
// Common ASDU Address so several SCADA masters can coexist on this gateway.
// ScadaIPs is a CSV allowlist of remote IPs permitted to complete the TCP
// handshake — empty = block all.
type IEC104Server struct {
	ID       int64  `db:"id"        json:"id"`
	Name     string `db:"name"      json:"name"`
	Port     int    `db:"port"      json:"port"`
	ASDUAddr int    `db:"asdu_addr" json:"asdu_addr"`
	ScadaIPs string `db:"scada_ips" json:"scada_ips"`
	K        int    `db:"k"         json:"k"`
	W        int    `db:"w"         json:"w"`
	T0       int    `db:"t0"        json:"t0"`
	T1       int    `db:"t1"        json:"t1"`
	T2       int    `db:"t2"        json:"t2"`
	T3       int    `db:"t3"        json:"t3"`
	Enabled  bool   `db:"enabled"   json:"enabled"`
}

// SignalMapping = core row. MQTT key → IEC 104 point on a specific slave.
// ServerID pins the mapping to one iec104_servers row; values are dispatched
// only to that endpoint and (server_id, ioa) is the uniqueness key.
// SignalID (optional) links to a gateway_signals row when created via the
// 4-menu workflow — the signal definition is inherited from there.
// Legacy rows created directly keep SignalID nil and carry json_key/metric_name
// inline as before.
type SignalMapping struct {
	ID             int64    `db:"id"             json:"id"`
	ServerID       int64    `db:"server_id"      json:"server_id"`
	TopicID        int64    `db:"topic_id"       json:"topic_id"`
	SignalID       *int64   `db:"signal_id"      json:"signal_id"`
	DeviceName     string   `db:"device_name"    json:"device_name"`
	VariableType   string   `db:"variable_type"  json:"variable_type"`
	Characteristic string   `db:"characteristic" json:"characteristic"`
	JSONKey        string   `db:"json_key"       json:"json_key"`
	QualityKey     string   `db:"quality_key"    json:"quality_key"`
	MetricName     string   `db:"metric_name"    json:"metric_name"`
	IEC104Type     string   `db:"iec104_type"    json:"iec104_type"`
	IOA            int      `db:"ioa"            json:"ioa"`
	Unit           string   `db:"unit"           json:"unit"`
	Scale          float64  `db:"scale"          json:"scale"`
	Enabled        bool     `db:"enabled"        json:"enabled"`
	Business       string   `db:"business"       json:"business"`
	Company        string   `db:"company"        json:"company"`
	DeadbandAbs    float64  `db:"deadband_abs"   json:"deadband_abs"`
	DeadbandPct    float64  `db:"deadband_pct"   json:"deadband_pct"`
}

// TSDBConfig = time-series pipeline settings. Singleton (id=1).
type TSDBConfig struct {
	ID         int64  `db:"id" json:"id"`
	Backend    string `db:"backend" json:"backend"`
	VMUrl      string `db:"vm_url" json:"vm_url"`
	VMUsername string `db:"vm_username" json:"vm_username"`
	VMPassword string `db:"vm_password" json:"vm_password"`
	TsDSN      string `db:"ts_dsn" json:"ts_dsn"`
	TsTable    string `db:"ts_table" json:"ts_table"`
	WALPath    string `db:"wal_path" json:"wal_path"`
	DLQPath    string `db:"dlq_path" json:"dlq_path"`
	BatchSize  int    `db:"batch_size" json:"batch_size"`
	FlushMs    int    `db:"flush_ms" json:"flush_ms"`
	Enabled    bool   `db:"enabled" json:"enabled"`
}

// NATSConfig = settings for the NATS JetStream fan-out buffer. Singleton (id=1).
type NATSConfig struct {
	ID         int64  `db:"id" json:"id"`
	Host       string `db:"host" json:"host"`
	Port       int    `db:"port" json:"port"`
	StreamName string `db:"stream_name" json:"stream_name"`
	Enabled    bool   `db:"enabled" json:"enabled"`
}


// AutodiscoveredEntity is a Sparkplug B node or device seen on the bus that
// has no matching SSFV catalog entry yet. metric_names is a JSON-encoded array.
type AutodiscoveredEntity struct {
	ID          int64     `db:"id"           json:"id"`
	GroupID     string    `db:"group_id"     json:"group_id"`
	NodeID      string    `db:"node_id"      json:"node_id"`
	DeviceID    string    `db:"device_id"    json:"device_id"`
	MetricNames string    `db:"metric_names" json:"metric_names"`
	Status      string    `db:"status"       json:"status"`
	FirstSeen   time.Time `db:"first_seen"   json:"first_seen"`
	LastSeen    time.Time `db:"last_seen"    json:"last_seen"`
}

// History = time-series log row.
type History struct {
	ID         int64     `db:"id" json:"id"`
	MappingID  int64     `db:"mapping_id" json:"mapping_id"`
	SignalPath string    `db:"signal_path" json:"signal_path"`
	Value      float64   `db:"value" json:"value"`
	Quality    int       `db:"quality" json:"quality"`
	Timestamp  time.Time `db:"timestamp" json:"timestamp"`
}
