package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	ErrReconciliationFailed = errors.New("reconciliation failed")
	ErrNoDataAvailable      = errors.New("no data available for reconciliation")
)

// Record represents a transaction record
type Record struct {
	ID              string
	TransactionID   string
	Amount          int64
	Currency        string
	Timestamp       time.Time
	Source          string
	Status          string
	Metadata        map[string]string
}

// MatchResult represents reconciliation match result
type MatchResult struct {
	SourceRecord      *Record
	DestinationRecord *Record
	Status            MatchStatus
	Discrepancies     []Discrepancy
	MatchedAt         time.Time
}

// MatchStatus indicates match status
type MatchStatus string

const (
	MatchStatusMatched     MatchStatus = "MATCHED"
	MatchStatusMismatched  MatchStatus = "MISMATCHED"
	MatchStatusMissing     MatchStatus = "MISSING"
	MatchStatusDuplicate   MatchStatus = "DUPLICATE"
)

// Discrepancy represents a field mismatch
type Discrepancy struct {
	Field    string
	Expected interface{}
	Actual   interface{}
}

// ReconciliationReport contains reconciliation results
type ReconciliationReport struct {
	StartTime         time.Time
	EndTime           time.Time
	Duration          time.Duration
	TotalSource       int
	TotalDestination  int
	Matched           int
	Mismatched        int
	MissingInSource   int
	MissingInDest     int
	Duplicates        int
	Results           []*MatchResult
	Summary           map[string]interface{}
}

// DataSource provides records for reconciliation
type DataSource interface {
	FetchRecords(ctx context.Context, start, end time.Time) ([]*Record, error)
	GetName() string
}

// MatchStrategy defines matching logic
type MatchStrategy interface {
	Match(source, destination *Record) (bool, []Discrepancy)
}

// Reconciler performs reconciliation
type Reconciler struct {
	source        DataSource
	destination   DataSource
	strategy      MatchStrategy
	logger        *zap.Logger
	batchSize     int
	workers       int
}

// Config configures the reconciler
type Config struct {
	Source      DataSource
	Destination DataSource
	Strategy    MatchStrategy
	BatchSize   int
	Workers     int
}

// NewReconciler creates a new reconciler
func NewReconciler(config Config, logger *zap.Logger) *Reconciler {
	if config.BatchSize == 0 {
		config.BatchSize = 1000
	}
	if config.Workers == 0 {
		config.Workers = 5
	}

	return &Reconciler{
		source:      config.Source,
		destination: config.Destination,
		strategy:    config.Strategy,
		logger:      logger,
		batchSize:   config.BatchSize,
		workers:     config.Workers,
	}
}

// Reconcile performs reconciliation for date range
func (r *Reconciler) Reconcile(ctx context.Context, start, end time.Time) (*ReconciliationReport, error) {
	r.logger.Info("starting reconciliation",
		zap.Time("start", start),
		zap.Time("end", end))

	report := &ReconciliationReport{
		StartTime: time.Now(),
		Summary:   make(map[string]interface{}),
	}

	// Fetch records from both sources
	sourceRecords, err := r.source.FetchRecords(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch source records: %w", err)
	}

	destRecords, err := r.destination.FetchRecords(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch destination records: %w", err)
	}

	report.TotalSource = len(sourceRecords)
	report.TotalDestination = len(destRecords)

	r.logger.Info("records fetched",
		zap.String("source", r.source.GetName()),
		zap.Int("source_count", len(sourceRecords)),
		zap.String("destination", r.destination.GetName()),
		zap.Int("dest_count", len(destRecords)))

	// Create lookup maps
	sourceMap := make(map[string]*Record)
	destMap := make(map[string]*Record)

	for _, rec := range sourceRecords {
		sourceMap[rec.TransactionID] = rec
	}

	for _, rec := range destRecords {
		if _, exists := destMap[rec.TransactionID]; exists {
			// Duplicate in destination
			report.Duplicates++
		}
		destMap[rec.TransactionID] = rec
	}

	// Match records
	var results []*MatchResult
	var mu sync.Mutex

	// Check source records against destination
	for txnID, sourceRec := range sourceMap {
		destRec, exists := destMap[txnID]

		if !exists {
			// Missing in destination
			result := &MatchResult{
				SourceRecord: sourceRec,
				Status:       MatchStatusMissing,
				MatchedAt:    time.Now(),
			}
			mu.Lock()
			results = append(results, result)
			report.MissingInDest++
			mu.Unlock()
			continue
		}

		// Match records
		matched, discrepancies := r.strategy.Match(sourceRec, destRec)

		result := &MatchResult{
			SourceRecord:      sourceRec,
			DestinationRecord: destRec,
			Discrepancies:     discrepancies,
			MatchedAt:         time.Now(),
		}

		if matched {
			result.Status = MatchStatusMatched
			mu.Lock()
			report.Matched++
			mu.Unlock()
		} else {
			result.Status = MatchStatusMismatched
			mu.Lock()
			report.Mismatched++
			mu.Unlock()
		}

		mu.Lock()
		results = append(results, result)
		mu.Unlock()
	}

	// Check for records in destination but not in source
	for txnID, destRec := range destMap {
		if _, exists := sourceMap[txnID]; !exists {
			result := &MatchResult{
				DestinationRecord: destRec,
				Status:            MatchStatusMissing,
				MatchedAt:         time.Now(),
			}
			mu.Lock()
			results = append(results, result)
			report.MissingInSource++
			mu.Unlock()
		}
	}

	report.Results = results
	report.EndTime = time.Now()
	report.Duration = report.EndTime.Sub(report.StartTime)

	// Calculate summary
	report.Summary["match_rate"] = float64(report.Matched) / float64(report.TotalSource) * 100
	report.Summary["total_exceptions"] = report.Mismatched + report.MissingInSource + report.MissingInDest

	r.logger.Info("reconciliation completed",
		zap.Duration("duration", report.Duration),
		zap.Int("matched", report.Matched),
		zap.Int("mismatched", report.Mismatched),
		zap.Int("missing_in_dest", report.MissingInDest),
		zap.Int("missing_in_source", report.MissingInSource))

	return report, nil
}

// ExactMatchStrategy matches records exactly
type ExactMatchStrategy struct {
	Fields []string // Fields to compare
}

func (s *ExactMatchStrategy) Match(source, destination *Record) (bool, []Discrepancy) {
	var discrepancies []Discrepancy

	// Amount must match
	if source.Amount != destination.Amount {
		discrepancies = append(discrepancies, Discrepancy{
			Field:    "amount",
			Expected: source.Amount,
			Actual:   destination.Amount,
		})
	}

	// Currency must match
	if source.Currency != destination.Currency {
		discrepancies = append(discrepancies, Discrepancy{
			Field:    "currency",
			Expected: source.Currency,
			Actual:   destination.Currency,
		})
	}

	// Status must match
	if source.Status != destination.Status {
		discrepancies = append(discrepancies, Discrepancy{
			Field:    "status",
			Expected: source.Status,
			Actual:   destination.Status,
		})
	}

	return len(discrepancies) == 0, discrepancies
}

// ToleranceMatchStrategy allows tolerance in amount
type ToleranceMatchStrategy struct {
	AmountTolerance int64 // Tolerance in cents
}

func (s *ToleranceMatchStrategy) Match(source, destination *Record) (bool, []Discrepancy) {
	var discrepancies []Discrepancy

	// Amount tolerance check
	diff := source.Amount - destination.Amount
	if diff < 0 {
		diff = -diff
	}

	if diff > s.AmountTolerance {
		discrepancies = append(discrepancies, Discrepancy{
			Field:    "amount",
			Expected: source.Amount,
			Actual:   destination.Amount,
		})
	}

	// Currency must match exactly
	if source.Currency != destination.Currency {
		discrepancies = append(discrepancies, Discrepancy{
			Field:    "currency",
			Expected: source.Currency,
			Actual:   destination.Currency,
		})
	}

	return len(discrepancies) == 0, discrepancies
}

// AutoReconciler performs automatic scheduled reconciliation
type AutoReconciler struct {
	reconciler *Reconciler
	schedule   time.Duration
	logger     *zap.Logger
	
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	
	onComplete func(*ReconciliationReport)
}

// NewAutoReconciler creates auto reconciler
func NewAutoReconciler(reconciler *Reconciler, schedule time.Duration, logger *zap.Logger) *AutoReconciler {
	ctx, cancel := context.WithCancel(context.Background())

	return &AutoReconciler{
		reconciler: reconciler,
		schedule:   schedule,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start starts automatic reconciliation
func (ar *AutoReconciler) Start() {
	ar.wg.Add(1)
	go ar.run()
}

func (ar *AutoReconciler) run() {
	defer ar.wg.Done()

	ticker := time.NewTicker(ar.schedule)
	defer ticker.Stop()

	for {
		select {
		case <-ar.ctx.Done():
			return
		case <-ticker.C:
			ar.runReconciliation()
		}
	}
}

func (ar *AutoReconciler) runReconciliation() {
	// Reconcile previous day
	end := time.Now().Truncate(24 * time.Hour)
	start := end.Add(-24 * time.Hour)

	ar.logger.Info("starting scheduled reconciliation",
		zap.Time("start", start),
		zap.Time("end", end))

	report, err := ar.reconciler.Reconcile(ar.ctx, start, end)
	if err != nil {
		ar.logger.Error("reconciliation failed", zap.Error(err))
		return
	}

	if ar.onComplete != nil {
		ar.onComplete(report)
	}
}

// Stop stops auto reconciliation
func (ar *AutoReconciler) Stop() {
	ar.cancel()
	ar.wg.Wait()
}

// SetOnComplete sets completion callback
func (ar *AutoReconciler) SetOnComplete(fn func(*ReconciliationReport)) {
	ar.onComplete = fn
}
