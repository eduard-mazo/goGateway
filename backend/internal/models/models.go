package models

import "time"

// Device = logical device (inverter, weather station, ...).
type Device struct {
	ID          int64     `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	Description string    `db:"description" json:"description"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

// MQTTConfig = broker connection params. Single row expected (id=1).
type MQTTConfig struct {
	ID       int64  `db:"id" json:"id"`
	Host     string `db:"host" json:"host"`
	Port     int    `db:"port" json:"port"`
	Username string `db:"username" json:"username"`
	Password string `db:"password" json:"password"`
	ClientID string `db:"client_id" json:"client_id"`
	UseTLS   bool   `db:"use_tls" json:"use_tls"`
}

// Topic = MQTT subscription bound to a device.
type Topic struct {
	ID       int64  `db:"id" json:"id"`
	DeviceID int64  `db:"device_id" json:"device_id"`
	Topic    string `db:"topic" json:"topic"`
	QoS      int    `db:"qos" json:"qos"`
	Enabled  bool   `db:"enabled" json:"enabled"`
}

// IEC104Server = one passive IEC 60870-5-104 slave endpoint.
// Multiple rows yield a fleet: each exposes the full point set under its own
// Common ASDU Address so several SCADA masters can coexist.
type IEC104Server struct {
	ID         int64  `db:"id"          json:"id"`
	Name       string `db:"name"        json:"name"`
	ListenAddr string `db:"listen_addr" json:"listen_addr"`
	Port       int    `db:"port"        json:"port"`
	ASDUAddr   int    `db:"asdu_addr"   json:"asdu_addr"`
	K          int    `db:"k"           json:"k"`
	W          int    `db:"w"           json:"w"`
	T0         int    `db:"t0"          json:"t0"`
	T1         int    `db:"t1"          json:"t1"`
	T2         int    `db:"t2"          json:"t2"`
	T3         int    `db:"t3"          json:"t3"`
	Enabled    bool   `db:"enabled"     json:"enabled"`
}

// SignalMapping = core row. MQTT key → IEC 104 point.
type SignalMapping struct {
	ID             int64   `db:"id" json:"id"`
	TopicID        int64   `db:"topic_id" json:"topic_id"`
	DeviceName     string  `db:"device_name" json:"device_name"`
	VariableType   string  `db:"variable_type" json:"variable_type"`
	Characteristic string  `db:"characteristic" json:"characteristic"`
	JSONKey        string  `db:"json_key" json:"json_key"`
	IEC104Type     string  `db:"iec104_type" json:"iec104_type"`
	IOA            int     `db:"ioa" json:"ioa"`
	Unit           string  `db:"unit" json:"unit"`
	Scale          float64 `db:"scale" json:"scale"`
	Enabled        bool    `db:"enabled" json:"enabled"`
}

// History = time-series log row.
type History struct {
	ID        int64     `db:"id" json:"id"`
	MappingID int64     `db:"mapping_id" json:"mapping_id"`
	SignalKey string    `db:"signal_key" json:"signal_key"`
	Value     float64   `db:"value" json:"value"`
	Quality   int       `db:"quality" json:"quality"`
	Timestamp time.Time `db:"timestamp" json:"timestamp"`
}
