package worker

import (
	"encoding/json"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
)

// MetricMeta carries FIWARE/SSFV metadata extracted from a Sparkplug B
// metric's PropertySet (field 9). Fields map to ssfv.tbl_senales columns.
type MetricMeta struct {
	Name         string `json:"name"`
	EngUnit      string `json:"engUnit,omitempty"`
	TipoVariable string `json:"tipo_variable,omitempty"`
	TipoValor    string `json:"tipo_valor,omitempty"`
	Description  string `json:"description,omitempty"`
	// UnsName is the producer-declared display name (birth-only property
	// uns/name, contract v3 §5) → pre-fills tbl_senales.nombre on approval
	// (e.g. "Valvula cerrada"). Description above is fed from uns/description.
	UnsName string `json:"uns_name,omitempty"`
	// UnsCode / UnsInstance carry the producer-declared FIWARE decomposition
	// (contract §5.1) so the approve UI pre-fills codigo_senal / nombre_instancia
	// instead of making the operator type them.
	UnsCode     string `json:"uns_code,omitempty"`
	UnsInstance string `json:"uns_instance,omitempty"`
	// DeviceTopic is the lookup key used by SSFVMappingCache (e.g. "EPM/SSFV/EPM/Sede30/INV_1").
	// Non-empty for solar/UNS multi-device nodes; empty for single-entity nodes (valve).
	DeviceTopic string `json:"device_topic,omitempty"`
	// DeviceType is the tipo_equipo name for the device owning this metric.
	DeviceType string `json:"device_type,omitempty"`
	// Value is the birth string value, captured for device-identity metrics
	// (Device/* strings, contract §5) so the approve UI can show the actual
	// hardware identity (PartNumber, Firmware, Serial, …) before approval.
	Value string `json:"value,omitempty"`
}

// AutoDiscoveryService tracks Sparkplug B nodes/devices seen on the bus.
// When a NBIRTH or DBIRTH arrives for an entity not yet in the SSFV catalog,
// it is recorded in SQLite with status "pending" for operator review.
// The operator selects the correct planta and tipo_equipo via the UI before
// any provisioning happens — this prevents nodes with stale/wrong planta_id
// metadata from being auto-assigned to the wrong plant.
type AutoDiscoveryService struct {
	db        *sqlx.DB
	rebirthFn func(groupID, nodeID string)
}

// NewAutoDiscoveryService creates the service using the gateway's SQLite pool.
func NewAutoDiscoveryService(db *sqlx.DB) *AutoDiscoveryService {
	return &AutoDiscoveryService{db: db}
}

// SetRebirthFn wires a callback that sends NCMD Rebirth to a node.
// Called after operator approval so NBIRTH+NDATA flows immediately
// once the mapping cache has been reloaded.
func (s *AutoDiscoveryService) SetRebirthFn(fn func(groupID, nodeID string)) {
	s.rebirthFn = fn
}

// OnBIRTH records a NBIRTH (deviceID="") or DBIRTH (deviceID set) in SQLite.
// Uses an UPSERT that refreshes metric_names, metric_meta, node_properties,
// and last_seen only when the row is still pending — approved/rejected
// decisions are not overwritten.
// Safe to call from any goroutine; designed to be non-blocking (fire-and-forget).
func (s *AutoDiscoveryService) OnBIRTH(groupID, nodeID, deviceID string, metricNames []string, meta []MetricMeta, nodeProps map[string]string) {
	names, _ := json.Marshal(metricNames)
	metaJSON, _ := json.Marshal(meta)
	propsJSON, _ := json.Marshal(nodeProps)
	if metaJSON == nil {
		metaJSON = []byte("[]")
	}
	if propsJSON == nil {
		propsJSON = []byte("{}")
	}
	now := time.Now().UTC()
	_, err := s.db.Exec(`
		INSERT INTO autodiscovered_entities
		    (group_id, node_id, device_id, metric_names, metric_meta, node_properties, status, first_seen, last_seen)
		VALUES (?,?,?,?,?,?,'pending',?,?)
		ON CONFLICT(group_id, node_id, device_id) DO UPDATE SET
		    metric_names    = excluded.metric_names,
		    metric_meta     = excluded.metric_meta,
		    node_properties = excluded.node_properties,
		    last_seen       = excluded.last_seen
		WHERE autodiscovered_entities.status = 'pending'`,
		groupID, nodeID, deviceID, string(names), string(metaJSON), string(propsJSON), now, now)
	if err != nil {
		log.Printf("autodiscovery: upsert %s/%s/%s: %v", groupID, nodeID, deviceID, err)
	}

	// Auto-provisioning is intentionally disabled here.
	// Nodes are recorded as 'pending' and must be approved by an operator via
	// the UI, which lets them select the correct planta and tipo_equipo.
	// Provisioning + NCMD Rebirth happens in approveAutodiscovered (API handler).
}

// TriggerRebirth requests an NCMD Rebirth from the named node, causing it to
// re-publish its NBIRTH so fresh NDATA flows after a mapping cache reload.
func (s *AutoDiscoveryService) TriggerRebirth(groupID, nodeID string) {
	if s.rebirthFn != nil {
		s.rebirthFn(groupID, nodeID)
	}
}

// SetStatus marks an entity "approved" or "rejected".
func (s *AutoDiscoveryService) SetStatus(id int64, status string) error {
	_, err := s.db.Exec(
		`UPDATE autodiscovered_entities SET status=?, last_seen=? WHERE id=?`,
		status, time.Now().UTC(), id)
	return err
}
