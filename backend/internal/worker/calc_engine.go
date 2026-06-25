package worker

import (
	"context"
	"log"
	"math"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"

	"goGateway/internal/iec104"
)

// CalcExpression is one row from calculated_signals, resolved into a
// form ready for runtime evaluation.
type CalcExpression struct {
	ID          int64
	Name        string
	Operator    string  // '+', '-', '*', '/', 'abs'
	AKey        uint64  // packKey(operand_a_server, operand_a_ioa)
	BKey        uint64  // packKey(operand_b_server, operand_b_ioa); zero for unary
	Scale       float64
	OutServerID int64
	OutIOA      int
	OutType     string
}

// CalcEngine maintains a live value registry and re-evaluates virtual signals
// whenever an operand changes.  Evaluation and IEC-104 dispatch happen in a
// single dedicated goroutine so the hot path (Update) is a non-blocking
// channel send — it never stalls MQTT message handling.
type CalcEngine struct {
	mu     sync.RWMutex
	vals   map[uint64]float64  // (serverID, IOA) → latest dispatched value
	exprs  []CalcExpression    // loaded from DB; reloaded via Reload()
	notify chan uint64          // buffered; carries the key that just changed
	iec    iec104.Server
	db     *sqlx.DB
}

func NewCalcEngine(db *sqlx.DB, srv iec104.Server) *CalcEngine {
	return &CalcEngine{
		db:     db,
		iec:    srv,
		vals:   make(map[uint64]float64),
		notify: make(chan uint64, 512),
	}
}

// Reload re-reads calculated_signals from SQLite.  Safe to call while running.
func (e *CalcEngine) Reload() error {
	rows, err := e.db.Queryx(
		`SELECT id, name, operator,
		        operand_a_server, operand_a_ioa,
		        COALESCE(operand_b_server, 0), COALESCE(operand_b_ioa, 0),
		        scale, out_server_id, out_ioa, out_type
		   FROM calculated_signals WHERE enabled = 1`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var exprs []CalcExpression
	for rows.Next() {
		var (
			ex                                      CalcExpression
			aServer, bServer                        int64
			aIOA, bIOA                              int
		)
		if err := rows.Scan(
			&ex.ID, &ex.Name, &ex.Operator,
			&aServer, &aIOA,
			&bServer, &bIOA,
			&ex.Scale, &ex.OutServerID, &ex.OutIOA, &ex.OutType,
		); err != nil {
			return err
		}
		ex.AKey = packKey(aServer, aIOA)
		if bServer != 0 {
			ex.BKey = packKey(bServer, bIOA)
		}
		exprs = append(exprs, ex)
	}

	e.mu.Lock()
	e.exprs = exprs
	e.mu.Unlock()
	return nil
}

// Update records a newly dispatched value and signals the evaluation goroutine
// to re-check any expressions that depend on this (serverID, ioa).
// Non-blocking: drops the notification if the channel is full (the next Update
// will trigger another evaluation pass).
func (e *CalcEngine) Update(serverID int64, ioa int, val float64) {
	key := packKey(serverID, ioa)
	e.mu.Lock()
	e.vals[key] = val
	e.mu.Unlock()
	select {
	case e.notify <- key:
	default:
	}
}

// Run processes the notification channel until ctx is cancelled.
// Start it in a dedicated goroutine from main.
func (e *CalcEngine) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case key := <-e.notify:
			e.evaluate(key)
		}
	}
}

func (e *CalcEngine) evaluate(changedKey uint64) {
	e.mu.RLock()
	exprs := e.exprs
	vals := e.vals
	e.mu.RUnlock()

	for _, ex := range exprs {
		if ex.AKey != changedKey && ex.BKey != changedKey {
			continue
		}
		a, okA := vals[ex.AKey]
		if !okA {
			continue
		}
		b, okB := vals[ex.BKey]

		var result float64
		switch ex.Operator {
		case "+":
			if !okB {
				continue
			}
			result = a + b
		case "-":
			if !okB {
				continue
			}
			result = a - b
		case "*":
			if !okB {
				continue
			}
			result = a * b
		case "/":
			if !okB || b == 0 {
				continue
			}
			result = a / b
		case "abs":
			result = math.Abs(a)
		default:
			log.Printf("calc: unknown operator %q for signal %q", ex.Operator, ex.Name)
			continue
		}

		result *= ex.Scale
		e.iec.Dispatch(ex.OutServerID, iec104.Point{
			IOA:       ex.OutIOA,
			TypeID:    ex.OutType,
			Value:     result,
			Quality:   iec104.QualityGood,
			Timestamp: time.Now(),
		})
	}
}
