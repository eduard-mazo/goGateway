package tsdb

import (
	"fmt"
	"sync"
	"time"

	"github.com/vmihailenco/msgpack/v5"
	bolt "go.etcd.io/bbolt"
)

var dlqBucket = []byte("dlq")

// DLQEntry holds a batch that exhausted all retry attempts.
type DLQEntry struct {
	ID        uint64      `msgpack:"id"      json:"id"`
	Backend   string      `msgpack:"backend" json:"backend"`
	Timestamp time.Time   `msgpack:"ts"      json:"ts"`
	Batch     []DataPoint `msgpack:"batch"   json:"batch"`
	Reason    string      `msgpack:"reason"  json:"reason"`
	Retries   int         `msgpack:"retries" json:"retries"`
}

// DLQ is an embedded BoltDB dead-letter queue.
// Data that fails all retry attempts is parked here for operator review
// and manual replay via the UI.
type DLQ struct {
	db  *bolt.DB
	mu  sync.Mutex
	seq uint64
}

func NewDLQ(path string) (*DLQ, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("dlq open: %w", err)
	}
	d := &DLQ{db: db}
	return d, db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(dlqBucket)
		return err
	})
}

// Push parks a failed batch in the DLQ.
func (d *DLQ) Push(backend string, batch []DataPoint, reason string, retries int) error {
	d.mu.Lock()
	d.seq++
	id := d.seq
	d.mu.Unlock()

	entry := DLQEntry{
		ID: id, Backend: backend,
		Timestamp: time.Now(), Batch: batch,
		Reason: reason, Retries: retries,
	}
	data, err := msgpack.Marshal(entry)
	if err != nil {
		return err
	}
	return d.db.Update(func(tx *bolt.Tx) error {
		key := []byte(fmt.Sprintf("%020d", id))
		return tx.Bucket(dlqBucket).Put(key, data)
	})
}

// Len returns the number of entries in the DLQ.
func (d *DLQ) Len() int {
	var n int
	d.db.View(func(tx *bolt.Tx) error { //nolint:errcheck
		n = tx.Bucket(dlqBucket).Stats().KeyN
		return nil
	})
	return n
}

// Replay iterates all DLQ entries; fn returning true acks (deletes) the entry.
func (d *DLQ) Replay(fn func(DLQEntry) bool) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(dlqBucket)
		return b.ForEach(func(k, v []byte) error {
			var e DLQEntry
			if msgpack.Unmarshal(v, &e) != nil {
				return nil
			}
			if fn(e) {
				b.Delete(k) //nolint:errcheck
			}
			return nil
		})
	})
}

func (d *DLQ) Close() error { return d.db.Close() }
