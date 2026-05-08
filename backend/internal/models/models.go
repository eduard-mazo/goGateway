// Package models defines the shared domain structs used across db, api, and
// worker. All fields map 1:1 to DB columns via sqlx tags.
package models

import "time"

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
}

// Topic = MQTT subscription bound to a device.
type Topic struct {
	ID       int64  `db:"id" json:"id"`
	DeviceID int64  `db:"device_id" json:"device_id"`
	Topic    string `db:"topic" json:"topic"`
	QoS      int    `db:"qos" json:"qos"`
	Enabled  bool   `db:"enabled" json:"enabled"`
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
// QualityKey, if non-empty, names the JSON key in the MQTT payload that carries
// the quality for this signal (overrides the payload-level "quality" field).
// MetricName, if non-empty, is the Sparkplug B metric name used when the
// gateway operates in Sparkplug B mode (SparkplugEnabled=true in MQTTConfig).
type SignalMapping struct {
	ID             int64   `db:"id"             json:"id"`
	ServerID       int64   `db:"server_id"      json:"server_id"`
	TopicID        int64   `db:"topic_id"       json:"topic_id"`
	DeviceName     string  `db:"device_name"    json:"device_name"`
	VariableType   string  `db:"variable_type"  json:"variable_type"`
	Characteristic string  `db:"characteristic" json:"characteristic"`
	JSONKey        string  `db:"json_key"       json:"json_key"`
	QualityKey     string  `db:"quality_key"    json:"quality_key"`
	MetricName     string  `db:"metric_name"    json:"metric_name"`
	IEC104Type     string  `db:"iec104_type"    json:"iec104_type"`
	IOA            int     `db:"ioa"            json:"ioa"`
	Unit           string  `db:"unit"           json:"unit"`
	Scale          float64 `db:"scale"          json:"scale"`
	Enabled        bool    `db:"enabled"        json:"enabled"`
	Business       string  `db:"business"       json:"business"`
	Company        string  `db:"company"        json:"company"`
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


// History = time-series log row.
type History struct {
	ID         int64     `db:"id" json:"id"`
	MappingID  int64     `db:"mapping_id" json:"mapping_id"`
	SignalPath string    `db:"signal_path" json:"signal_path"`
	Value      float64   `db:"value" json:"value"`
	Quality    int       `db:"quality" json:"quality"`
	Timestamp  time.Time `db:"timestamp" json:"timestamp"`
}
