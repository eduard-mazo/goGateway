package tsdb

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
)

// PipelineConfig controls all tunable parameters.
type PipelineConfig struct {
	WorkerCount   int           // default runtime.NumCPU()*2
	InputBufSize  int           // input channel depth, default 100_000
	BatchSize     int           // max points per batch, default 2000
	FlushInterval time.Duration // max time between flushes, default 100ms
	MaxRetries    int           // retry attempts before DLQ, default 5
	RetryBufSize  int           // per-backend retry channel depth, default 20_000
	WALPath       string        // default "data/wal.bolt"
	DLQPath       string        // default "data/dlq.bolt"
	Backends      []TSDBWriter
}

func (c *PipelineConfig) applyDefaults() {
	if c.WorkerCount == 0 {
		c.WorkerCount = runtime.NumCPU() * 2
	}
	if c.InputBufSize == 0 {
		c.InputBufSize = 100_000
	}
	if c.BatchSize == 0 {
		c.BatchSize = 2000
	}
	if c.FlushInterval == 0 {
		c.FlushInterval = 100 * time.Millisecond
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = 5
	}
	if c.RetryBufSize == 0 {
		c.RetryBufSize = 20_000
	}
	if c.WALPath == "" {
		c.WALPath = "data/wal.bolt"
	}
	if c.DLQPath == "" {
		c.DLQPath = "data/dlq.bolt"
	}
}

type retryItem struct {
	walID      uint64
	batch      []DataPoint
	backend    TSDBWriter
	attempt    int
	ackTracker *ackTracker
	isReplay   bool // use WriteBatchSafe (idempotent) on WAL replay
}

type batchWork struct {
	walID      uint64
	batch      []DataPoint
	ackTracker *ackTracker
	isReplay   bool
}

// WritePipeline is the central coordinator:
//
//	inputCh → [workers] → accumCh → [accumulator] → WAL → batchCh
//	→ [fan-out] → backends (parallel)
//	     └─ on fail → per-backend retryQ → [retryWorker] → DLQ (last resort)
type WritePipeline struct {
	cfg         PipelineConfig
	inputCh     chan DataPoint
	accumCh     chan DataPoint
	batchCh     chan batchWork
	retryQueues map[string]chan retryItem
	backends    []TSDBWriter
	breakers    map[string]*CircuitBreaker
	wal         *WAL
	dlq         *DLQ
	store       *PointStore
	inputRate   *rateTracker
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewWritePipeline constructs and initialises the pipeline.
func NewWritePipeline(cfg PipelineConfig) (*WritePipeline, error) {
	cfg.applyDefaults()

	wal, err := NewWAL(cfg.WALPath)
	if err != nil {
		return nil, fmt.Errorf("pipeline wal: %w", err)
	}
	dlq, err := NewDLQ(cfg.DLQPath)
	if err != nil {
		return nil, fmt.Errorf("pipeline dlq: %w", err)
	}

	breakers := make(map[string]*CircuitBreaker, len(cfg.Backends))
	retryQueues := make(map[string]chan retryItem, len(cfg.Backends))
	for _, b := range cfg.Backends {
		breakers[b.Name()] = NewCircuitBreaker()
		retryQueues[b.Name()] = make(chan retryItem, cfg.RetryBufSize)
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &WritePipeline{
		cfg:         cfg,
		inputCh:     make(chan DataPoint, cfg.InputBufSize),
		accumCh:     make(chan DataPoint, cfg.InputBufSize/2),
		batchCh:     make(chan batchWork, 128),
		retryQueues: retryQueues,
		backends:    cfg.Backends,
		breakers:    breakers,
		wal:         wal,
		dlq:         dlq,
		store:       NewPointStore(),
		inputRate:   newRateTracker(10 * time.Second),
		ctx:         ctx,
		cancel:      cancel,
	}, nil
}

// Push enqueues one DataPoint. Non-blocking; returns error if input buffer is full.
func (p *WritePipeline) Push(pt DataPoint) error {
	select {
	case p.inputCh <- pt:
		p.inputRate.record(1)
		return nil
	default:
		return fmt.Errorf("tsdb input buffer full (%d)", p.cfg.InputBufSize)
	}
}

// Run starts all goroutines and blocks until ctx is cancelled.
func (p *WritePipeline) Run(ctx context.Context) {
	for range p.cfg.WorkerCount {
		p.wg.Add(1)
		go p.worker()
	}
	p.wg.Add(1)
	go p.accumulator()
	p.wg.Add(1)
	go p.fanOut()
	for _, b := range p.backends {
		p.wg.Add(1)
		go p.retryWorker(b)
	}
	p.wg.Add(1)
	go p.healthChecker()

	p.replayWAL()

	<-ctx.Done()
	p.Shutdown()
}

// worker reads from inputCh, updates the PointStore, forwards to accumCh.
func (p *WritePipeline) worker() {
	defer p.wg.Done()
	for {
		select {
		case pt, ok := <-p.inputCh:
			if !ok {
				return
			}
			if ioa := pt.Tags["ioa"]; ioa != "" {
				p.store.Set(ioa, StoredPoint{
					Value:     primaryValue(pt.Fields),
					Timestamp: pt.Timestamp,
					Tags:      pt.Tags,
				})
			}
			select {
			case p.accumCh <- pt:
			case <-p.ctx.Done():
				return
			}
		case <-p.ctx.Done():
			return
		}
	}
}

// accumulator collects individual points into batches (size or timer trigger).
func (p *WritePipeline) accumulator() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.cfg.FlushInterval)
	defer ticker.Stop()
	batch := make([]DataPoint, 0, p.cfg.BatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		toSend := batch
		batch = make([]DataPoint, 0, p.cfg.BatchSize)

		walID, err := p.wal.Append(toSend)
		if err != nil {
			// WAL write failure is critical; park in DLQ immediately.
			for _, b := range p.backends {
				p.dlq.Push(b.Name(), toSend, "wal write failed: "+err.Error(), 0) //nolint:errcheck
			}
			return
		}

		tracker := newAckTracker(p.wal)
		tracker.Register(walID, len(p.backends))

		select {
		case p.batchCh <- batchWork{walID: walID, batch: toSend, ackTracker: tracker}:
		case <-p.ctx.Done():
		}
	}

	for {
		select {
		case pt, ok := <-p.accumCh:
			if !ok {
				flush()
				return
			}
			batch = append(batch, pt)
			if len(batch) >= p.cfg.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-p.ctx.Done():
			for {
				select {
				case pt := <-p.accumCh:
					batch = append(batch, pt)
					if len(batch) >= p.cfg.BatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// fanOut dispatches each batch to all backends in parallel goroutines.
func (p *WritePipeline) fanOut() {
	defer p.wg.Done()
	for {
		select {
		case work, ok := <-p.batchCh:
			if !ok {
				return
			}
			var wg sync.WaitGroup
			for _, b := range p.backends {
				wg.Add(1)
				go func(backend TSDBWriter) {
					defer wg.Done()
					cb := p.breakers[backend.Name()]
					if !cb.Allow() {
						p.scheduleRetry(work, backend, 1)
						return
					}
					writeCtx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
					defer cancel()
					var err error
					if work.isReplay {
						if ts, ok := backend.(*TimescaleAdapter); ok {
							err = ts.WriteBatchSafe(writeCtx, work.batch)
						} else {
							err = backend.WriteBatch(writeCtx, work.batch)
						}
					} else {
						err = backend.WriteBatch(writeCtx, work.batch)
					}
					if err != nil {
						cb.RecordFailure()
						p.scheduleRetry(work, backend, 1)
						return
					}
					cb.RecordSuccess()
					work.ackTracker.Ack(work.walID)
				}(b)
			}
			wg.Wait()
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *WritePipeline) scheduleRetry(work batchWork, backend TSDBWriter, attempt int) {
	item := retryItem{
		walID:      work.walID,
		batch:      work.batch,
		backend:    backend,
		attempt:    attempt,
		ackTracker: work.ackTracker,
		isReplay:   work.isReplay,
	}
	q := p.retryQueues[backend.Name()]
	select {
	case q <- item:
	default:
		p.dlq.Push(backend.Name(), work.batch, "retry queue full", attempt) //nolint:errcheck
		work.ackTracker.Ack(work.walID)
	}
}

// retryWorker handles one backend's retry queue with exponential backoff.
func (p *WritePipeline) retryWorker(target TSDBWriter) {
	defer p.wg.Done()
	q := p.retryQueues[target.Name()]
	for {
		select {
		case item, ok := <-q:
			if !ok {
				return
			}
			if item.attempt > p.cfg.MaxRetries {
				p.dlq.Push(item.backend.Name(), item.batch, //nolint:errcheck
					fmt.Sprintf("max retries %d exceeded", p.cfg.MaxRetries), item.attempt)
				item.ackTracker.Ack(item.walID)
				continue
			}

			// Exponential backoff: 500ms, 2s, 4.5s, 8s, 12.5s
			backoff := time.Duration(item.attempt*item.attempt) * 500 * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-p.ctx.Done():
				p.dlq.Push(item.backend.Name(), item.batch, "shutdown during retry", item.attempt) //nolint:errcheck
				item.ackTracker.Ack(item.walID)
				return
			}

			cb := p.breakers[item.backend.Name()]
			if !cb.Allow() {
				item.attempt++
				select {
				case q <- item:
				default:
					p.dlq.Push(item.backend.Name(), item.batch, "circuit open, queue full", item.attempt) //nolint:errcheck
					item.ackTracker.Ack(item.walID)
				}
				continue
			}

			writeCtx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
			var err error
			if item.isReplay {
				if ts, ok := item.backend.(*TimescaleAdapter); ok {
					err = ts.WriteBatchSafe(writeCtx, item.batch)
				} else {
					err = item.backend.WriteBatch(writeCtx, item.batch)
				}
			} else {
				err = item.backend.WriteBatch(writeCtx, item.batch)
			}
			cancel()

			if err != nil {
				cb.RecordFailure()
				item.attempt++
				select {
				case q <- item:
				default:
					p.dlq.Push(item.backend.Name(), item.batch, err.Error(), item.attempt) //nolint:errcheck
					item.ackTracker.Ack(item.walID)
				}
				continue
			}
			cb.RecordSuccess()
			item.ackTracker.Ack(item.walID)

		case <-p.ctx.Done():
			return
		}
	}
}

// healthChecker probes all backends every 15 seconds.
func (p *WritePipeline) healthChecker() {
	defer p.wg.Done()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			for _, b := range p.backends {
				go func(backend TSDBWriter) {
					ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
					defer cancel()
					cb := p.breakers[backend.Name()]
					if err := backend.HealthCheck(ctx); err != nil {
						cb.RecordFailure()
					} else {
						cb.RecordSuccess()
					}
				}(b)
			}
		case <-p.ctx.Done():
			return
		}
	}
}

// replayWAL re-feeds unacked WAL batches on startup after a crash.
func (p *WritePipeline) replayWAL() {
	replayed := 0
	p.wal.Replay(func(e WALEntry) bool { //nolint:errcheck
		tracker := newAckTracker(p.wal)
		tracker.Register(e.ID, len(p.backends))
		select {
		case p.batchCh <- batchWork{
			walID:      e.ID,
			batch:      e.Batch,
			ackTracker: tracker,
			isReplay:   true,
		}:
			replayed++
		default:
		}
		return false // fan-out will ack
	})
	if replayed > 0 {
		fmt.Printf("tsdb: WAL replay %d batches\n", replayed)
	}
}

// Shutdown drains the pipeline and closes all resources.
func (p *WritePipeline) Shutdown() {
	p.cancel()
	close(p.inputCh)
	p.wg.Wait()
	for _, b := range p.backends {
		b.Close()
	}
	p.wal.Close()
	p.dlq.Close()
}

// Accessors used by the HTTP handler.
func (p *WritePipeline) Store() *PointStore            { return p.store }
func (p *WritePipeline) WAL() *WAL                    { return p.wal }
func (p *WritePipeline) DLQ() *DLQ                    { return p.dlq }
func (p *WritePipeline) Backends() []TSDBWriter        { return p.backends }
func (p *WritePipeline) InputRate() float64            { return p.inputRate.rate() }
func (p *WritePipeline) InputQueueDepth() int          { return len(p.inputCh) }
func (p *WritePipeline) RetryQueueDepth(name string) int {
	if q, ok := p.retryQueues[name]; ok {
		return len(q)
	}
	return 0
}
func (p *WritePipeline) CircuitState(name string) string {
	if cb, ok := p.breakers[name]; ok {
		return cb.State()
	}
	return "unknown"
}
