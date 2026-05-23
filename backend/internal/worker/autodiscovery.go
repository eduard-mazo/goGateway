package worker

import (
	"encoding/json"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
)

// AutoDiscoveryService tracks Sparkplug B nodes/devices seen on the bus.
// When a NBIRTH or DBIRTH arrives for an entity not yet in the SSFV catalog,
// it is recorded in SQLite with status "pending" for operator review.
type AutoDiscoveryService struct {
	db *sqlx.DB
}

// NewAutoDiscoveryService creates the service using the gateway's SQLite pool.
func NewAutoDiscoveryService(db *sqlx.DB) *AutoDiscoveryService {
	return &AutoDiscoveryService{db: db}
}

// OnBIRTH records a NBIRTH (deviceID="") or DBIRTH (deviceID set) in SQLite.
// Uses an UPSERT that refreshes metric_names and last_seen only when the row
// is still pending — approved/rejected decisions are not overwritten.
// Safe to call from any goroutine; designed to be non-blocking (fire-and-forget).
func (s *AutoDiscoveryService) OnBIRTH(groupID, nodeID, deviceID string, metricNames []string) {
	names, _ := json.Marshal(metricNames)
	now := time.Now().UTC()
	_, err := s.db.Exec(`
		INSERT INTO autodiscovered_entities
		    (group_id, node_id, device_id, metric_names, status, first_seen, last_seen)
		VALUES (?,?,?,?,'pending',?,?)
		ON CONFLICT(group_id, node_id, device_id) DO UPDATE SET
		    metric_names = excluded.metric_names,
		    last_seen    = excluded.last_seen
		WHERE autodiscovered_entities.status = 'pending'`,
		groupID, nodeID, deviceID, string(names), now, now)
	if err != nil {
		log.Printf("autodiscovery: upsert %s/%s/%s: %v", groupID, nodeID, deviceID, err)
	}
}

// SetStatus marks an entity "approved" or "rejected".
func (s *AutoDiscoveryService) SetStatus(id int64, status string) error {
	_, err := s.db.Exec(
		`UPDATE autodiscovered_entities SET status=?, last_seen=? WHERE id=?`,
		status, time.Now().UTC(), id)
	return err
}
