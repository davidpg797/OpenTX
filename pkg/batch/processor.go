package batch

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

var (
	ErrBatchTimeout = errors.New("batch processing timeout")
	ErrBatchFailed  = errors.New("batch processing failed")
)

// BatchProcessor processes items in batches
type BatchProcessor struct {
	batchSize    int
	workers      int
	flushTimeout time.Duration
	maxRetries   int
	
	mu           sync.Mutex
	buffer       []interface{}
	timer        *time.Timer
	
	processFn    ProcessFunc
	errorHandler ErrorHandler
	
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	
	logger       *zap.Logger
	
	// Metrics
	stats        *BatchStats
}

// ProcessFunc processes a batch of items
type ProcessFunc func(ctx context.Context, items []interface{}) error

// ErrorHandler handles processing errors
type ErrorHandler func(items []interface{}, err error)

// BatchStats holds processing statistics
type BatchStats struct {
	mu                sync.RWMutex
	TotalBatches      uint64
	SuccessfulBatches uint64
	FailedBatches     uint64
	TotalItems        uint64
	ProcessedItems    uint64
	FailedItems       uint64
}

// Config configures the batch processor
type Config struct {
	BatchSize    int           // Number of items per batch
	Workers      int           // Number of concurrent workers
	FlushTimeout time.Duration // Max time to wait before flushing partial batch
	MaxRetries   int           // Max retry attempts for failed batches
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(config Config, processFn ProcessFunc, logger *zap.Logger) *BatchProcessor {
	if config.BatchSize == 0 {
		config.BatchSize = 100
	}
	if config.Workers == 0 {
		config.Workers = 5
	}
	if config.FlushTimeout == 0 {
		config.FlushTimeout = 5 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	ctx, cancel := context.WithCancel(context.Background())

	bp := &BatchProcessor{
		batchSize:    config.BatchSize,
		workers:      config.Workers,
		flushTimeout: config.FlushTimeout,
		maxRetries:   config.MaxRetries,
		buffer:       make([]interface{}, 0, config.BatchSize),
		processFn:    processFn,
		ctx:          ctx,
		cancel:       cancel,
		logger:       logger,
		stats:        &BatchStats{},
	}

	bp.timer = time.NewTimer(bp.flushTimeout)
	bp.timer.Stop()

	return bp
}

// Add adds an item to the batch
func (bp *BatchProcessor) Add(item interface{}) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.ctx.Err() != nil {
		return bp.ctx.Err()
	}

	bp.buffer = append(bp.buffer, item)
	bp.stats.increment(&bp.stats.TotalItems, 1)

	// Start timer if this is the first item
	if len(bp.buffer) == 1 {
		bp.timer.Reset(bp.flushTimeout)
	}

	// Flush if batch is full
	if len(bp.buffer) >= bp.batchSize {
		return bp.flushLocked()
	}

	return nil
}

// Flush forces processing of buffered items
func (bp *BatchProcessor) Flush() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return bp.flushLocked()
}

func (bp *BatchProcessor) flushLocked() error {
	if len(bp.buffer) == 0 {
		return nil
	}

	bp.timer.Stop()

	batch := bp.buffer
	bp.buffer = make([]interface{}, 0, bp.batchSize)

	// Process asynchronously
	bp.wg.Add(1)
	go func() {
		defer bp.wg.Done()
		bp.processBatch(batch)
	}()

	return nil
}

func (bp *BatchProcessor) processBatch(items []interface{}) {
	batchSize := uint64(len(items))
	bp.stats.increment(&bp.stats.TotalBatches, 1)

	var err error
	for attempt := 0; attempt <= bp.maxRetries; attempt++ {
		if attempt > 0 {
			bp.logger.Warn("retrying batch",
				zap.Int("attempt", attempt),
				zap.Int("batch_size", len(items)))
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		ctx, cancel := context.WithTimeout(bp.ctx, 30*time.Second)
		err = bp.processFn(ctx, items)
		cancel()

		if err == nil {
			bp.stats.increment(&bp.stats.SuccessfulBatches, 1)
			bp.stats.increment(&bp.stats.ProcessedItems, batchSize)
			bp.logger.Debug("batch processed successfully",
				zap.Int("size", len(items)))
			return
		}

		bp.logger.Error("batch processing failed",
			zap.Error(err),
			zap.Int("attempt", attempt),
			zap.Int("batch_size", len(items)))
	}

	// All retries failed
	bp.stats.increment(&bp.stats.FailedBatches, 1)
	bp.stats.increment(&bp.stats.FailedItems, batchSize)

	if bp.errorHandler != nil {
		bp.errorHandler(items, err)
	}
}

// Start starts the flush timer monitoring
func (bp *BatchProcessor) Start() {
	bp.wg.Add(1)
	go bp.timerLoop()
}

func (bp *BatchProcessor) timerLoop() {
	defer bp.wg.Done()

	for {
		select {
		case <-bp.ctx.Done():
			return
		case <-bp.timer.C:
			bp.Flush()
		}
	}
}

// Stop stops the processor and waits for pending batches
func (bp *BatchProcessor) Stop(ctx context.Context) error {
	bp.cancel()

	// Flush remaining items
	if err := bp.Flush(); err != nil {
		bp.logger.Error("failed to flush on stop", zap.Error(err))
	}

	// Wait for pending batches with timeout
	done := make(chan struct{})
	go func() {
		bp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stats returns processing statistics
func (bp *BatchProcessor) Stats() BatchStats {
	bp.stats.mu.RLock()
	defer bp.stats.mu.RUnlock()
	return *bp.stats
}

func (s *BatchStats) increment(counter *uint64, delta uint64) {
	s.mu.Lock()
	*counter += delta
	s.mu.Unlock()
}

// ParallelBatchProcessor processes batches in parallel
type ParallelBatchProcessor struct {
	workers    int
	workQueue  chan []interface{}
	processFn  ProcessFunc
	logger     *zap.Logger
	
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewParallelBatchProcessor creates a parallel batch processor
func NewParallelBatchProcessor(workers int, processFn ProcessFunc, logger *zap.Logger) *ParallelBatchProcessor {
	if workers == 0 {
		workers = 5
	}

	ctx, cancel := context.WithCancel(context.Background())

	pbp := &ParallelBatchProcessor{
		workers:   workers,
		workQueue: make(chan []interface{}, workers*2),
		processFn: processFn,
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
	}

	return pbp
}

// Start starts worker goroutines
func (pbp *ParallelBatchProcessor) Start() {
	for i := 0; i < pbp.workers; i++ {
		pbp.wg.Add(1)
		go pbp.worker(i)
	}
}

func (pbp *ParallelBatchProcessor) worker(id int) {
	defer pbp.wg.Done()

	pbp.logger.Info("worker started", zap.Int("worker_id", id))

	for {
		select {
		case <-pbp.ctx.Done():
			return
		case batch, ok := <-pbp.workQueue:
			if !ok {
				return
			}

			ctx, cancel := context.WithTimeout(pbp.ctx, 30*time.Second)
			err := pbp.processFn(ctx, batch)
			cancel()

			if err != nil {
				pbp.logger.Error("batch processing failed",
					zap.Int("worker_id", id),
					zap.Int("batch_size", len(batch)),
					zap.Error(err))
			}
		}
	}
}

// Submit submits a batch for processing
func (pbp *ParallelBatchProcessor) Submit(items []interface{}) error {
	select {
	case pbp.workQueue <- items:
		return nil
	case <-pbp.ctx.Done():
		return pbp.ctx.Err()
	}
}

// Stop stops all workers
func (pbp *ParallelBatchProcessor) Stop() {
	pbp.cancel()
	close(pbp.workQueue)
	pbp.wg.Wait()
}

// ChunkedProcessor splits large datasets into chunks and processes them
type ChunkedProcessor struct {
	chunkSize int
	workers   int
	logger    *zap.Logger
}

// NewChunkedProcessor creates a chunked processor
func NewChunkedProcessor(chunkSize, workers int, logger *zap.Logger) *ChunkedProcessor {
	return &ChunkedProcessor{
		chunkSize: chunkSize,
		workers:   workers,
		logger:    logger,
	}
}

// Process processes items in chunks
func (cp *ChunkedProcessor) Process(ctx context.Context, items []interface{}, processFn ProcessFunc) error {
	if len(items) == 0 {
		return nil
	}

	chunks := cp.createChunks(items)
	
	g, ctx := errgroup.WithContext(ctx)
	semaphore := make(chan struct{}, cp.workers)

	for i, chunk := range chunks {
		chunk := chunk
		chunkID := i

		g.Go(func() error {
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return ctx.Err()
			}

			cp.logger.Debug("processing chunk",
				zap.Int("chunk_id", chunkID),
				zap.Int("size", len(chunk)))

			return processFn(ctx, chunk)
		})
	}

	return g.Wait()
}

func (cp *ChunkedProcessor) createChunks(items []interface{}) [][]interface{} {
	var chunks [][]interface{}

	for i := 0; i < len(items); i += cp.chunkSize {
		end := i + cp.chunkSize
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[i:end])
	}

	return chunks
}
