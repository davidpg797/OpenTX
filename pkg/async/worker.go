package async

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	ErrWorkerPoolClosed = errors.New("worker pool closed")
	ErrJobTimeout       = errors.New("job execution timeout")
)

// Job represents an asynchronous job
type Job struct {
	ID        string
	Type      string
	Payload   interface{}
	Priority  int
	Timeout   time.Duration
	Retries   int
	CreatedAt time.Time
}

// Result represents job execution result
type Result struct {
	JobID     string
	Success   bool
	Output    interface{}
	Error     error
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	Attempt   int
}

// Handler processes jobs
type Handler func(context.Context, *Job) (interface{}, error)

// WorkerPool manages asynchronous job processing
type WorkerPool struct {
	workers    int
	queue      chan *Job
	results    chan *Result
	handlers   map[string]Handler
	logger     *zap.Logger
	
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	
	// Statistics
	stats      *PoolStats
}

// PoolStats holds worker pool statistics
type PoolStats struct {
	mu              sync.RWMutex
	TotalJobs       uint64
	CompletedJobs   uint64
	FailedJobs      uint64
	RetryCount      uint64
	AverageDuration time.Duration
}

// Config configures worker pool
type Config struct {
	Workers    int
	QueueSize  int
	ResultSize int
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(config Config, logger *zap.Logger) *WorkerPool {
	if config.Workers == 0 {
		config.Workers = 10
	}
	if config.QueueSize == 0 {
		config.QueueSize = 1000
	}
	if config.ResultSize == 0 {
		config.ResultSize = 100
	}

	ctx, cancel := context.WithCancel(context.Background())

	wp := &WorkerPool{
		workers:  config.Workers,
		queue:    make(chan *Job, config.QueueSize),
		results:  make(chan *Result, config.ResultSize),
		handlers: make(map[string]Handler),
		logger:   logger,
		ctx:      ctx,
		cancel:   cancel,
		stats:    &PoolStats{},
	}

	return wp
}

// Start starts worker pool
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
	wp.logger.Info("worker pool started", zap.Int("workers", wp.workers))
}

// Stop stops worker pool
func (wp *WorkerPool) Stop() {
	wp.cancel()
	close(wp.queue)
	wp.wg.Wait()
	close(wp.results)
	wp.logger.Info("worker pool stopped")
}

// RegisterHandler registers a job handler
func (wp *WorkerPool) RegisterHandler(jobType string, handler Handler) {
	wp.handlers[jobType] = handler
	wp.logger.Info("handler registered", zap.String("type", jobType))
}

// Submit submits a job
func (wp *WorkerPool) Submit(job *Job) error {
	select {
	case wp.queue <- job:
		wp.stats.incrementTotal()
		return nil
	case <-wp.ctx.Done():
		return ErrWorkerPoolClosed
	default:
		return errors.New("queue full")
	}
}

// Results returns result channel
func (wp *WorkerPool) Results() <-chan *Result {
	return wp.results
}

func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	wp.logger.Info("worker started", zap.Int("worker_id", id))

	for {
		select {
		case <-wp.ctx.Done():
			return
		case job, ok := <-wp.queue:
			if !ok {
				return
			}
			wp.processJob(job)
		}
	}
}

func (wp *WorkerPool) processJob(job *Job) {
	handler, exists := wp.handlers[job.Type]
	if !exists {
		wp.logger.Error("no handler for job type",
			zap.String("job_id", job.ID),
			zap.String("type", job.Type))
		
		result := &Result{
			JobID:   job.ID,
			Success: false,
			Error:   errors.New("no handler registered"),
		}
		wp.sendResult(result)
		wp.stats.incrementFailed()
		return
	}

	// Execute job with retry
	var lastErr error
	for attempt := 0; attempt <= job.Retries; attempt++ {
		if attempt > 0 {
			wp.logger.Info("retrying job",
				zap.String("job_id", job.ID),
				zap.Int("attempt", attempt))
			wp.stats.incrementRetry()
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		result := wp.executeJob(job, handler, attempt+1)
		
		if result.Success {
			wp.sendResult(result)
			wp.stats.incrementCompleted()
			wp.stats.updateDuration(result.Duration)
			return
		}

		lastErr = result.Error
	}

	// All attempts failed
	result := &Result{
		JobID:   job.ID,
		Success: false,
		Error:   lastErr,
		Attempt: job.Retries + 1,
	}
	wp.sendResult(result)
	wp.stats.incrementFailed()
}

func (wp *WorkerPool) executeJob(job *Job, handler Handler, attempt int) *Result {
	start := time.Now()

	// Create timeout context
	timeout := job.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(wp.ctx, timeout)
	defer cancel()

	// Execute handler
	output, err := handler(ctx, job)

	result := &Result{
		JobID:     job.ID,
		Success:   err == nil,
		Output:    output,
		Error:     err,
		StartTime: start,
		EndTime:   time.Now(),
		Duration:  time.Since(start),
		Attempt:   attempt,
	}

	if err != nil {
		wp.logger.Error("job failed",
			zap.String("job_id", job.ID),
			zap.Int("attempt", attempt),
			zap.Error(err))
	} else {
		wp.logger.Debug("job completed",
			zap.String("job_id", job.ID),
			zap.Duration("duration", result.Duration))
	}

	return result
}

func (wp *WorkerPool) sendResult(result *Result) {
	select {
	case wp.results <- result:
	default:
		wp.logger.Warn("result channel full, dropping result",
			zap.String("job_id", result.JobID))
	}
}

// Stats returns pool statistics
func (wp *WorkerPool) Stats() PoolStats {
	wp.stats.mu.RLock()
	defer wp.stats.mu.RUnlock()
	return *wp.stats
}

func (s *PoolStats) incrementTotal() {
	s.mu.Lock()
	s.TotalJobs++
	s.mu.Unlock()
}

func (s *PoolStats) incrementCompleted() {
	s.mu.Lock()
	s.CompletedJobs++
	s.mu.Unlock()
}

func (s *PoolStats) incrementFailed() {
	s.mu.Lock()
	s.FailedJobs++
	s.mu.Unlock()
}

func (s *PoolStats) incrementRetry() {
	s.mu.Lock()
	s.RetryCount++
	s.mu.Unlock()
}

func (s *PoolStats) updateDuration(duration time.Duration) {
	s.mu.Lock()
	if s.AverageDuration == 0 {
		s.AverageDuration = duration
	} else {
		// Simple moving average
		s.AverageDuration = (s.AverageDuration + duration) / 2
	}
	s.mu.Unlock()
}

// PriorityQueue implements priority job queue
type PriorityQueue struct {
	items []*Job
	mu    sync.Mutex
}

// NewPriorityQueue creates a priority queue
func NewPriorityQueue() *PriorityQueue {
	return &PriorityQueue{
		items: make([]*Job, 0),
	}
}

// Push adds job to queue
func (pq *PriorityQueue) Push(job *Job) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	pq.items = append(pq.items, job)
	pq.sort()
}

// Pop removes and returns highest priority job
func (pq *PriorityQueue) Pop() *Job {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) == 0 {
		return nil
	}

	job := pq.items[0]
	pq.items = pq.items[1:]
	return job
}

// Len returns queue length
func (pq *PriorityQueue) Len() int {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return len(pq.items)
}

func (pq *PriorityQueue) sort() {
	// Simple bubble sort by priority (higher first)
	for i := 0; i < len(pq.items); i++ {
		for j := i + 1; j < len(pq.items); j++ {
			if pq.items[j].Priority > pq.items[i].Priority {
				pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
			}
		}
	}
}

// ScheduledJob represents a scheduled job
type ScheduledJob struct {
	Job       *Job
	Schedule  time.Time
	Recurring time.Duration
}

// Scheduler manages scheduled jobs
type Scheduler struct {
	pool      *WorkerPool
	jobs      []*ScheduledJob
	mu        sync.Mutex
	logger    *zap.Logger
	
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewScheduler creates a new scheduler
func NewScheduler(pool *WorkerPool, logger *zap.Logger) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	return &Scheduler{
		pool:   pool,
		jobs:   make([]*ScheduledJob, 0),
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start starts the scheduler
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.run()
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
}

// Schedule schedules a job
func (s *Scheduler) Schedule(job *ScheduledJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.jobs = append(s.jobs, job)
	s.logger.Info("job scheduled",
		zap.String("job_id", job.Job.ID),
		zap.Time("schedule", job.Schedule))
}

func (s *Scheduler) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkJobs()
		}
	}
}

func (s *Scheduler) checkJobs() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var remaining []*ScheduledJob

	for _, sj := range s.jobs {
		if now.After(sj.Schedule) || now.Equal(sj.Schedule) {
			// Submit job
			if err := s.pool.Submit(sj.Job); err != nil {
				s.logger.Error("failed to submit scheduled job",
					zap.String("job_id", sj.Job.ID),
					zap.Error(err))
			}

			// Reschedule if recurring
			if sj.Recurring > 0 {
				sj.Schedule = now.Add(sj.Recurring)
				remaining = append(remaining, sj)
				s.logger.Info("recurring job rescheduled",
					zap.String("job_id", sj.Job.ID),
					zap.Time("next_run", sj.Schedule))
			}
		} else {
			remaining = append(remaining, sj)
		}
	}

	s.jobs = remaining
}

// GetScheduledJobs returns all scheduled jobs
func (s *Scheduler) GetScheduledJobs() []*ScheduledJob {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*ScheduledJob, len(s.jobs))
	copy(result, s.jobs)
	return result
}
