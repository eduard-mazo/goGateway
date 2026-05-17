package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type activeAlarm struct {
	id    int64
	start time.Time
}

// AlarmManager tracks open SSFV alarms in ssfv.tbl_alarmas.
// It keeps an in-memory map of open alarm IDs to avoid duplicate inserts.
type AlarmManager struct {
	pool   *pgxpool.Pool
	active map[int64]*activeAlarm // equisenal_id → open alarm
	mu     sync.Mutex
}

func NewAlarmManager(pool *pgxpool.Pool) *AlarmManager {
	return &AlarmManager{
		pool:   pool,
		active: make(map[int64]*activeAlarm),
	}
}

// Process opens or closes an alarm based on value (0 = clear, != 0 = active).
func (m *AlarmManager) Process(equisenalID int64, value float64, ts time.Time, tipoAlarma string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	isActive := value != 0
	existing := m.active[equisenalID]

	if isActive && existing == nil {
		id := m.insertAlarma(equisenalID, ts, tipoAlarma, severidadForTipo(tipoAlarma))
		if id > 0 {
			m.active[equisenalID] = &activeAlarm{id: id, start: ts}
		}
	} else if !isActive && existing != nil {
		m.closeAlarma(existing.id, ts)
		delete(m.active, equisenalID)
	}
}

// DrainOnShutdown closes all open alarms with ts = now.
// Call before closing the DB connection.
func (m *AlarmManager) DrainOnShutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for equiID, alarm := range m.active {
		m.closeAlarma(alarm.id, now)
		delete(m.active, equiID)
	}
}

// ActiveCount returns the number of currently open alarms (for status reporting).
func (m *AlarmManager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.active)
}

func (m *AlarmManager) insertAlarma(equisenalID int64, ts time.Time, tipoAlarma, severidad string) int64 {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var id int64
	err := m.pool.QueryRow(ctx, `
		INSERT INTO ssfv.tbl_alarmas
		    (equisenal_id, ts_inicio, tipo_alarma, severidad, activa)
		VALUES ($1,$2,$3,$4,TRUE)
		RETURNING alarma_id`,
		equisenalID, ts, tipoAlarma, severidad).Scan(&id)
	if err != nil {
		log.Printf("alarm: insert equisenal=%d: %v", equisenalID, err)
		return 0
	}
	return id
}

func (m *AlarmManager) closeAlarma(alarmaID int64, ts time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := m.pool.Exec(ctx, `
		UPDATE ssfv.tbl_alarmas
		SET ts_fin=$1, activa=FALSE
		WHERE alarma_id=$2`,
		ts, alarmaID)
	if err != nil {
		log.Printf("alarm: close id=%d: %v", alarmaID, err)
	}
}

func severidadForTipo(tipoAlarma string) string {
	switch tipoAlarma {
	case "Comunicacion":
		return "Alta"
	case "Dispositivo":
		return "Media"
	case "Fabricante":
		return "Baja"
	default:
		return "Media"
	}
}
