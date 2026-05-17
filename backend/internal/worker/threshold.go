package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"

	"goGateway/internal/iec104"
)

// AlarmLevel orders the severity of a threshold breach.
type AlarmLevel string

const (
	LevelNormal AlarmLevel = "NORMAL"
	LevelL      AlarmLevel = "L"
	LevelLL     AlarmLevel = "LL"
	LevelH      AlarmLevel = "H"
	LevelHH     AlarmLevel = "HH"
)

// SignalThreshold is one resolved row from signal_thresholds.
type SignalThreshold struct {
	ID            int64
	MappingID     int64
	HHValue       *float64
	HValue        *float64
	LValue        *float64
	LLValue       *float64
	HHAlarmIOA    int
	HAlarmIOA     int
	LAlarmIOA     int
	LLAlarmIOA    int
	AlarmServerID int64
	Deadband      float64
}

type alarmNotify struct {
	threshold SignalThreshold
	level     AlarmLevel
	prev      AlarmLevel
	value     float64
	ts        time.Time
}

// ThresholdEngine evaluates per-signal Hi/Lo limits and dispatches IEC-104
// alarm points (M_SP_TB_1) when a level change occurs.  It also persists
// alarm_events rows asynchronously so the hot path is never blocked by a DB
// write.
//
// Hysteresis: a level is only re-evaluated when the new value crosses the
// threshold +/- the configured Deadband, preventing chattering near limits.
type ThresholdEngine struct {
	mu         sync.RWMutex
	thresholds map[int64][]SignalThreshold // mapping_id → []threshold configs
	state      map[int64]AlarmLevel        // mapping_id → last active level
	events     chan alarmNotify            // buffered; drained by writer goroutine
	iec        iec104.Server
	db         *sqlx.DB
}

func NewThresholdEngine(db *sqlx.DB, srv iec104.Server) *ThresholdEngine {
	return &ThresholdEngine{
		db:         db,
		iec:        srv,
		thresholds: make(map[int64][]SignalThreshold),
		state:      make(map[int64]AlarmLevel),
		events:     make(chan alarmNotify, 1024),
	}
}

// Reload re-reads signal_thresholds from SQLite.  Safe to call while running.
func (e *ThresholdEngine) Reload() error {
	rows, err := e.db.Queryx(
		`SELECT id, mapping_id,
		        hh_value, h_value, l_value, ll_value,
		        hh_alarm_ioa, h_alarm_ioa, l_alarm_ioa, ll_alarm_ioa,
		        alarm_server_id, deadband
		   FROM signal_thresholds WHERE enabled = 1`)
	if err != nil {
		return err
	}
	defer rows.Close()

	next := make(map[int64][]SignalThreshold)
	for rows.Next() {
		var t SignalThreshold
		if err := rows.Scan(
			&t.ID, &t.MappingID,
			&t.HHValue, &t.HValue, &t.LValue, &t.LLValue,
			&t.HHAlarmIOA, &t.HAlarmIOA, &t.LAlarmIOA, &t.LLAlarmIOA,
			&t.AlarmServerID, &t.Deadband,
		); err != nil {
			return err
		}
		next[t.MappingID] = append(next[t.MappingID], t)
	}

	e.mu.Lock()
	e.thresholds = next
	e.mu.Unlock()
	return nil
}

// Evaluate checks a new value against all configured thresholds for mappingID.
// Call this from the dispatcher after a successful deadband pass.
// Non-blocking: drops the DB write notification if the event channel is full.
func (e *ThresholdEngine) Evaluate(mappingID int64, val float64, ts time.Time) {
	e.mu.RLock()
	threshs := e.thresholds[mappingID]
	e.mu.RUnlock()

	for _, t := range threshs {
		e.check(t, val, ts)
	}
}

func (e *ThresholdEngine) check(t SignalThreshold, val float64, ts time.Time) {
	level := classify(t, val)

	e.mu.RLock()
	prev, hasPrev := e.state[t.MappingID]
	e.mu.RUnlock()

	if !hasPrev {
		prev = LevelNormal
	}

	// Apply hysteresis: suppress level change if value is within deadband of
	// the boundary that would cause the transition.
	if level != prev && t.Deadband > 0 {
		if !hysteresisCleared(t, val, prev, level) {
			return
		}
	} else if level == prev {
		return
	}

	e.mu.Lock()
	e.state[t.MappingID] = level
	e.mu.Unlock()

	// Dispatch alarm IOA points to the assigned IEC-104 slave.
	e.dispatchAlarmPoints(t, level, ts)

	// Async DB write — never blocks the hot path.
	select {
	case e.events <- alarmNotify{threshold: t, level: level, prev: prev, value: val, ts: ts}:
	default:
		log.Printf("threshold: event buffer full, alarm event dropped for mapping %d", t.MappingID)
	}
}

// dispatchAlarmPoints sends M_SP_TB_1 frames for each IOA configured in the
// threshold row.  Active = 1.0, inactive = 0.0.
func (e *ThresholdEngine) dispatchAlarmPoints(t SignalThreshold, level AlarmLevel, ts time.Time) {
	send := func(ioa int, active bool) {
		if ioa == 0 {
			return
		}
		v := 0.0
		if active {
			v = 1.0
		}
		e.iec.Dispatch(t.AlarmServerID, iec104.Point{
			IOA:       ioa,
			TypeID:    "M_SP_TB_1",
			Value:     v,
			Quality:   iec104.QualityGood,
			Timestamp: ts,
		})
	}
	send(t.HHAlarmIOA, level == LevelHH)
	send(t.HAlarmIOA, level == LevelH)
	send(t.LAlarmIOA, level == LevelL)
	send(t.LLAlarmIOA, level == LevelLL)
}

// Run drains the events channel and persists alarm_events rows until ctx is
// cancelled.  Start in a dedicated goroutine from main.
func (e *ThresholdEngine) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-e.events:
			e.writeEvent(ctx, ev)
		}
	}
}

func (e *ThresholdEngine) writeEvent(ctx context.Context, ev alarmNotify) {
	_, err := e.db.ExecContext(ctx,
		`INSERT INTO alarm_events (mapping_id, level, value, timestamp)
		 VALUES (?, ?, ?, ?)`,
		ev.threshold.MappingID, string(ev.level), ev.value, ev.ts,
	)
	if err != nil {
		log.Printf("threshold: insert alarm_event: %v", err)
	}
}

// classify returns the highest breach level for val given the threshold config.
func classify(t SignalThreshold, val float64) AlarmLevel {
	if t.HHValue != nil && val >= *t.HHValue {
		return LevelHH
	}
	if t.HValue != nil && val >= *t.HValue {
		return LevelH
	}
	if t.LLValue != nil && val <= *t.LLValue {
		return LevelLL
	}
	if t.LValue != nil && val <= *t.LValue {
		return LevelL
	}
	return LevelNormal
}

// hysteresisCleared reports whether val has moved far enough beyond the
// boundary for the target level transition to be considered genuine.
func hysteresisCleared(t SignalThreshold, val float64, from, to AlarmLevel) bool {
	db := t.Deadband
	switch {
	case to == LevelHH && t.HHValue != nil:
		return val >= *t.HHValue+db
	case from == LevelHH && t.HHValue != nil:
		return val < *t.HHValue-db
	case to == LevelH && t.HValue != nil:
		return val >= *t.HValue+db
	case from == LevelH && t.HValue != nil:
		return val < *t.HValue-db
	case to == LevelLL && t.LLValue != nil:
		return val <= *t.LLValue-db
	case from == LevelLL && t.LLValue != nil:
		return val > *t.LLValue+db
	case to == LevelL && t.LValue != nil:
		return val <= *t.LValue-db
	case from == LevelL && t.LValue != nil:
		return val > *t.LValue+db
	}
	return true
}
