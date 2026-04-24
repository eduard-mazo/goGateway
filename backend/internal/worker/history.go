package worker

import (
	"context"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
)

// HistoryEvent = one row to persist.
type HistoryEvent struct {
	MappingID int64
	SignalKey string
	Value     float64
	Quality   int
	Timestamp time.Time
}

// HistoryLogger = async batched writer. Drops on full buffer (never blocks hot path).
type HistoryLogger struct {
	db    *sqlx.DB
	ch    chan HistoryEvent
	batch int
	flush time.Duration
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

// Log = non-blocking enqueue. Returns false if buffer full.
func (h *HistoryLogger) Log(e HistoryEvent) bool {
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
		}
	}
}

func (h *HistoryLogger) writeBatch(batch []HistoryEvent) error {
	tx, err := h.db.Beginx()
	if err != nil {
		return err
	}
	stmt, err := tx.Preparex(`INSERT INTO history(mapping_id,signal_key,value,quality,timestamp) VALUES(?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, e := range batch {
		if _, err := stmt.Exec(e.MappingID, e.SignalKey, e.Value, e.Quality, e.Timestamp); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
