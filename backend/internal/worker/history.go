package worker

import (
	"context"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"goGateway/internal/tsdb"
)

// HistoryEvent = one decoded sample travelling through the hot path.
type HistoryEvent struct {
	MappingID  int64
	SignalPath string
	Value      float64
	Quality    int
	Timestamp  time.Time
	IOA        int
	Business   string
	Company    string
}

// HistoryLogger forwards events to the TSDB pipeline.
// SQLite history writes are disabled — TimescaleDB is the sole time-series store.
type HistoryLogger struct {
	ch       chan HistoryEvent
	batch    int
	interval time.Duration
	tsdbPipe *tsdb.WritePipeline
}

func NewHistoryLogger(_ *sqlx.DB, bufSize, batch int, flush time.Duration) *HistoryLogger {
	if bufSize <= 0 {
		bufSize = 1024
	}
	if batch <= 0 {
		batch = 100
	}
	if flush <= 0 {
		flush = 2 * time.Second
	}
	return &HistoryLogger{ch: make(chan HistoryEvent, bufSize), batch: batch, interval: flush}
}

// SetTSDB attaches the TSDB pipeline. Events are forwarded as DataPoints.
func (h *HistoryLogger) SetTSDB(p *tsdb.WritePipeline) { h.tsdbPipe = p }

// Log enqueues an event non-blocking. Returns false if the buffer is full.
func (h *HistoryLogger) Log(e HistoryEvent) bool {
	if h.tsdbPipe != nil {
		pt := tsdb.DataPoint{
			Measurement: lastPathSegment(e.SignalPath),
			Tags: map[string]string{
				"signal_path": e.SignalPath,
				"business":    e.Business,
				"company":     e.Company,
			},
			Fields:    map[string]float64{"value": e.Value, "quality": float64(e.Quality)},
			Timestamp: e.Timestamp,
		}
		h.tsdbPipe.Push(pt) //nolint:errcheck
	}
	select {
	case h.ch <- e:
		return true
	default:
		return false
	}
}

// Run drains the channel until ctx is cancelled. No SQLite writes.
func (h *HistoryLogger) Run(ctx context.Context) {
	tick := time.NewTicker(h.interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case <-h.ch:
				default:
					return
				}
			}
		case <-h.ch:
		case <-tick.C:
		}
	}
}

// lastPathSegment returns the last "/" segment of path, or the full string.
func lastPathSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
