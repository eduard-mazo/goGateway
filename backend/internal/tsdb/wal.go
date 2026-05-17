package tsdb

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/vmihailenco/msgpack/v5"
	bolt "go.etcd.io/bbolt"
)

var walBucket = []byte("wal")

// WALEntry is one persisted batch, identified by a monotonic ID.
type WALEntry struct {
	ID        uint64      `msgpack:"id"`
	Timestamp time.Time   `msgpack:"ts"`
	Batch     []DataPoint `msgpack:"batch"`
}

// WAL is a crash-safe write-ahead log backed by BoltDB.
// Batches are appended before network writes and deleted only when all
// configured backends confirm success (via ackTracker).
type WAL struct {
	db      *bolt.DB
	seq     atomic.Uint64
	pending atomic.Int64
}

func NewWAL(path string) (*WAL, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{
		Timeout:      2 * time.Second,
		NoSync:       false,
		FreelistType: bolt.FreelistMapType,
	})
	if err != nil {
		return nil, fmt.Errorf("wal open: %w", err)
	}
	w := &WAL{db: db}
	return w, db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(walBucket)
		if err != nil {
			return err
		}
		c := b.Cursor()
		if k, _ := c.Last(); k != nil {
			w.seq.Store(binary.BigEndian.Uint64(k))
		}
		w.pending.Store(int64(b.Stats().KeyN))
		return nil
	})
}

// Append writes the batch to WAL and returns its assigned ID.
func (w *WAL) Append(batch []DataPoint) (uint64, error) {
	id := w.seq.Add(1)
	data, err := msgpack.Marshal(WALEntry{ID: id, Timestamp: time.Now(), Batch: batch})
	if err != nil {
		return 0, fmt.Errorf("wal marshal: %w", err)
	}
	if err = w.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(walBucket).Put(walKey(id), data)
	}); err != nil {
		return 0, fmt.Errorf("wal write: %w", err)
	}
	w.pending.Add(1)
	return id, nil
}

// Ack deletes the WAL entry once all backends have confirmed the write.
func (w *WAL) Ack(id uint64) {
	w.db.Update(func(tx *bolt.Tx) error { //nolint:errcheck
		return tx.Bucket(walBucket).Delete(walKey(id))
	})
	w.pending.Add(-1)
}

// Pending returns the count of unacknowledged WAL entries.
func (w *WAL) Pending() int { return int(w.pending.Load()) }

// Replay iterates unacked entries on startup for crash recovery.
// fn returning true acks (deletes) the entry.
func (w *WAL) Replay(fn func(WALEntry) bool) error {
	return w.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(walBucket)
		return b.ForEach(func(k, v []byte) error {
			var e WALEntry
			if msgpack.Unmarshal(v, &e) != nil {
				return nil // skip corrupt entries
			}
			if fn(e) {
				b.Delete(k) //nolint:errcheck
				w.pending.Add(-1)
			}
			return nil
		})
	})
}

func (w *WAL) Close() error { return w.db.Close() }

func walKey(id uint64) []byte {
	k := make([]byte, 8)
	binary.BigEndian.PutUint64(k, id)
	return k
}
