package tsdb

import (
	"hash/fnv"
	"sync"
	"time"
)

const numShards = 128

// StoredPoint is the latest value for an IOA, held in the sharded cache.
type StoredPoint struct {
	Value     float64
	Timestamp time.Time
	Tags      map[string]string
}

type pointShard struct {
	mu     sync.RWMutex
	points map[string]StoredPoint
	_pad   [24]byte // prevent false sharing across cache lines
}

// PointStore is a 128-shard concurrent map keyed by IOA string.
// Eliminates global lock contention at high signal rates.
type PointStore struct {
	shards [numShards]pointShard
}

func NewPointStore() *PointStore {
	ps := &PointStore{}
	for i := range ps.shards {
		ps.shards[i].points = make(map[string]StoredPoint, 256)
	}
	return ps
}

func (ps *PointStore) shardIdx(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() % numShards)
}

func (ps *PointStore) Set(key string, p StoredPoint) {
	s := &ps.shards[ps.shardIdx(key)]
	s.mu.Lock()
	s.points[key] = p
	s.mu.Unlock()
}

func (ps *PointStore) Get(key string) (StoredPoint, bool) {
	s := &ps.shards[ps.shardIdx(key)]
	s.mu.RLock()
	p, ok := s.points[key]
	s.mu.RUnlock()
	return p, ok
}

// Snapshot returns a full copy of all current values (for the UI live panel).
func (ps *PointStore) Snapshot() map[string]StoredPoint {
	out := make(map[string]StoredPoint, numShards*64)
	for i := range ps.shards {
		s := &ps.shards[i]
		s.mu.RLock()
		for k, v := range s.points {
			out[k] = v
		}
		s.mu.RUnlock()
	}
	return out
}

func (ps *PointStore) Len() (n int) {
	for i := range ps.shards {
		s := &ps.shards[i]
		s.mu.RLock()
		n += len(s.points)
		s.mu.RUnlock()
	}
	return
}
