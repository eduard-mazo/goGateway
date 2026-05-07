package worker

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"goGateway/internal/tsdb"
)

// HistoryEvent = one row to persist.
type HistoryEvent struct {
	MappingID  int64
	SignalPath string // full path: business/company/B1/.../signal
	Value      float64
	Quality    int
	Timestamp  time.Time
	IOA        int // IEC-104 Information Object Address (dispatch only, not stored in TSDB)
}

// HistoryLogger = async batched writer. Drops on full buffer (never blocks hot path).
type HistoryLogger struct {
	db       *sqlx.DB
	ch       chan HistoryEvent
	batch    int
	flush    time.Duration
	tsdbPipe *tsdb.WritePipeline // optional; nil = disabled
}

func NewHistoryLogger(db *sqlx.DB, bufSize, batch int, flush time.Duration) *HistoryLogger {
	if bufSize <= 0 {
		bufSize = 1024
	}
	if batch <= 0 {
		batch = 100
	}
	if flush <= 0 {
		flush = 2 * time.Second
	}
	return &HistoryLogger{db: db, ch: make(chan HistoryEvent, bufSize), batch: batch, flush: flush}
}

// SetTSDB attaches an optional TSDB write pipeline. Each logged event is
// also forwarded as a DataPoint for high-throughput time-series storage.
func (h *HistoryLogger) SetTSDB(p *tsdb.WritePipeline) { h.tsdbPipe = p }

// Log = non-blocking enqueue. Returns false if buffer full.
func (h *HistoryLogger) Log(e HistoryEvent) bool {
	if h.tsdbPipe != nil {
		pt := tsdb.DataPoint{
			Measurement: lastPathSegment(e.SignalPath),
			Tags:        map[string]string{"path": e.SignalPath},
			Fields:      map[string]float64{"value": e.Value, "quality": float64(e.Quality)},
			Timestamp:   e.Timestamp,
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

// Run = blocks until ctx done. Flushes remainder on exit.
func (h *HistoryLogger) Run(ctx context.Context) {
	buf := make([]HistoryEvent, 0, h.batch)
	tick := time.NewTicker(h.flush)
	defer tick.Stop()

	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := h.writeBatch(buf); err != nil {
			log.Printf("history write: %v", err)
		}
		buf = buf[:0]
	}

	for {
		select {
		case <-ctx.Done():
			// drain
			for {
				select {
				case e := <-h.ch:
					buf = append(buf, e)
				default:
					flush()
					return
				}
			}
		case e := <-h.ch:
			buf = append(buf, e)
			if len(buf) >= h.batch {
				flush()
			}
		case <-tick.C:
			flush()
			h.trim()
		}
	}
}

const histMaxBytes = 250 * 1024 * 1024 // 250 MB

// trim deletes the oldest 25 % of history rows when the DB data exceeds 250 MB.
// Uses (page_count - freelist_count) * page_size so free pages after DELETE are
// correctly excluded without needing a VACUUM.
func (h *HistoryLogger) trim() {
	var pageCount, freeCount, pageSize int64
	if h.db.Get(&pageCount, `PRAGMA page_count`) != nil ||
		h.db.Get(&freeCount, `PRAGMA freelist_count`) != nil ||
		h.db.Get(&pageSize, `PRAGMA page_size`) != nil {
		return
	}
	if (pageCount-freeCount)*pageSize <= histMaxBytes {
		return
	}
	var total int64
	if err := h.db.Get(&total, `SELECT COUNT(*) FROM history`); err != nil || total == 0 {
		return
	}
	del := total / 4
	if del < 1000 {
		del = 1000
	}
	if _, err := h.db.Exec(
		`DELETE FROM history WHERE id IN (SELECT id FROM history ORDER BY id ASC LIMIT ?)`, del,
	); err != nil {
		log.Printf("history trim: %v", err)
	}
}

// lastPathSegment returns the last "/" segment of a path, or the full string if no slash.
func lastPathSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func (h *HistoryLogger) writeBatch(batch []HistoryEvent) error {
	tx, err := h.db.Beginx()
	if err != nil {
		return err
	}
	stmt, err := tx.Preparex(`INSERT INTO history(mapping_id,signal_path,value,quality,timestamp) VALUES(?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, e := range batch {
		if _, err := stmt.Exec(e.MappingID, e.SignalPath, e.Value, e.Quality, e.Timestamp); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
