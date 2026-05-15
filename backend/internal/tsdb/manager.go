package tsdb

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"goGateway/internal/models"
)

// Manager owns the pipeline lifecycle: it can start, stop, and hot-reload
// a WritePipeline when the UI saves new TSDB configuration.
type Manager struct {
	mu          sync.Mutex
	pipe        *WritePipeline
	cancel      context.CancelFunc
	done        chan struct{}
	parentCtx   context.Context
	setHist     func(*WritePipeline)
	ssfvAdapter *SSFVAdapter // non-nil when ssfv schema is connected
}

// SSFVAdapter returns the active SSFVAdapter or nil.
// Used by the ssfv catalog API handlers to obtain the PostgreSQL pool.
func (m *Manager) SSFVAdapter() *SSFVAdapter {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ssfvAdapter
}

// NewManager creates a Manager. setHist is called whenever the active pipeline
// changes so the history logger keeps a current reference.
func NewManager(ctx context.Context, setHist func(*WritePipeline)) *Manager {
	return &Manager{parentCtx: ctx, setHist: setHist}
}

// Pipeline returns the currently active WritePipeline, or nil if not running.
func (m *Manager) Pipeline() *WritePipeline {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pipe
}

// Reload stops the old pipeline (if any) and starts a new one from cfg.
// If cfg.Enabled is false or cfg.Backend is "none"/empty, the pipeline is
// stopped and not restarted.
func (m *Manager) Reload(cfg models.TSDBConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()

	if !cfg.Enabled || cfg.Backend == "none" || cfg.Backend == "" {
		if m.setHist != nil {
			m.setHist(nil)
		}
		return
	}

	var backends []TSDBWriter

	if cfg.Backend == "victoriametrics" || cfg.Backend == "both" {
		if cfg.VMUrl != "" {
			backends = append(backends, NewVMAdapter(VMConfig{
				URL:      cfg.VMUrl,
				Username: cfg.VMUsername,
				Password: cfg.VMPassword,
			}))
			log.Printf("tsdb: VictoriaMetrics → %s", cfg.VMUrl)
		}
	}
	if cfg.Backend == "timescaledb" || cfg.Backend == "both" {
		if cfg.TsDSN != "" {
			// Only the SSFV adapter writes to TimescaleDB. The generic TimescaleAdapter
			// (public.signals table) is not created: all data goes to ssfv.tbl_valores
			// via ON CONFLICT DO NOTHING. This eliminates the dual-write path.
			ssfvCtx, ssfvCancel := context.WithTimeout(m.parentCtx, 30*time.Second)
			defer ssfvCancel()
			sa, ssfvErr := NewSSFVAdapter(ssfvCtx, cfg.TsDSN)
			if ssfvErr != nil {
				log.Printf("tsdb: ssfv adapter init: %v (apply migration 007 to create ssfv schema)", ssfvErr)
			} else {
				backends = append(backends, sa)
				m.ssfvAdapter = sa
				log.Printf("tsdb: SSFV adapter connected → ssfv.tbl_valores (ON CONFLICT DO NOTHING)")
			}
		}
	}

	if len(backends) == 0 {
		if m.setHist != nil {
			m.setHist(nil)
		}
		return
	}

	walPath := cfg.WALPath
	if walPath == "" {
		walPath = "data/wal.bolt"
	}
	dlqPath := cfg.DLQPath
	if dlqPath == "" {
		dlqPath = "data/dlq.bolt"
	}
	batchSize := cfg.BatchSize
	if batchSize == 0 {
		batchSize = 2000
	}
	flushMs := cfg.FlushMs
	if flushMs == 0 {
		flushMs = 100
	}

	if err := os.MkdirAll("data", 0755); err != nil {
		log.Printf("tsdb: mkdir data: %v", err)
	}
	pipe, err := NewWritePipeline(PipelineConfig{
		Backends:      backends,
		WALPath:       walPath,
		DLQPath:       dlqPath,
		BatchSize:     batchSize,
		FlushInterval: time.Duration(flushMs) * time.Millisecond,
	})
	if err != nil {
		log.Printf("tsdb: pipeline init: %v", err)
		if m.setHist != nil {
			m.setHist(nil)
		}
		return
	}

	pipeCtx, cancel := context.WithCancel(m.parentCtx)
	done := make(chan struct{})
	go func() {
		pipe.Run(pipeCtx)
		close(done)
	}()
	m.pipe = pipe
	m.cancel = cancel
	m.done = done
	if m.setHist != nil {
		m.setHist(pipe)
	}
	log.Printf("tsdb: pipeline started (%d backends)", len(backends))
}

// Stop shuts down the pipeline gracefully.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
	m.ssfvAdapter = nil
	if m.setHist != nil {
		m.setHist(nil)
	}
}

// StatusSummary returns a simple running flag for the UI config panel.
func (m *Manager) StatusSummary() map[string]any {
	m.mu.Lock()
	p := m.pipe
	m.mu.Unlock()
	if p == nil {
		return map[string]any{"running": false}
	}
	return map[string]any{"running": true}
}

// stopLocked cancels and drains the current pipeline. Must be called with m.mu held.
func (m *Manager) stopLocked() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if m.done != nil {
		select {
		case <-m.done:
		case <-time.After(10 * time.Second):
			log.Printf("tsdb: pipeline shutdown timeout")
		}
		m.done = nil
	}
	m.pipe = nil
}
