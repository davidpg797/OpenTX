package loadtest

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// Load Testing Framework for Payment Gateway
// Supports various load patterns and comprehensive metrics

// TestConfig holds load test configuration
type TestConfig struct {
	Name              string
	Duration          time.Duration
	VirtualUsers      int
	RampUpTime        time.Duration
	ThinkTime         time.Duration
	MaxRPS            int
	Pattern           LoadPattern
	Scenario          TestScenario
	WarmupDuration    time.Duration
	CooldownDuration  time.Duration
	ReportInterval    time.Duration
}

// LoadPattern defines traffic pattern
type LoadPattern string

const (
	PatternConstant   LoadPattern = "constant"   // Constant load
	PatternRampUp     LoadPattern = "ramp_up"    // Gradual increase
	PatternRampDown   LoadPattern = "ramp_down"  // Gradual decrease
	PatternSpike      LoadPattern = "spike"      // Sudden spike
	PatternWave       LoadPattern = "wave"       // Sine wave pattern
	PatternStairs     LoadPattern = "stairs"     // Step increases
)

// TestScenario represents a test scenario
type TestScenario interface {
	Setup(ctx context.Context) error
	Execute(ctx context.Context, userID int) (*Result, error)
	Teardown(ctx context.Context) error
}

// Result represents test execution result
type Result struct {
	Success       bool
	Duration      time.Duration
	ResponseSize  int64
	StatusCode    int
	Error         error
	Timestamp     time.Time
	UserID        int
	OperationType string
}

// Metrics holds test metrics
type Metrics struct {
	TotalRequests     int64
	SuccessfulRequests int64
	FailedRequests    int64
	TotalDuration     time.Duration
	MinDuration       time.Duration
	MaxDuration       time.Duration
	AvgDuration       time.Duration
	P50Duration       time.Duration
	P90Duration       time.Duration
	P95Duration       time.Duration
	P99Duration       time.Duration
	TotalBytes        int64
	ErrorRate         float64
	RequestsPerSecond float64
	ConcurrentUsers   int
	StatusCodes       map[int]int64
	Errors            map[string]int64
	durations         []time.Duration
	mu                sync.RWMutex
}

// Runner executes load tests
type Runner struct {
	config  TestConfig
	metrics *Metrics
	results chan *Result
	done    chan struct{}
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started time.Time
}

// NewRunner creates a new load test runner
func NewRunner(config TestConfig) *Runner {
	return &Runner{
		config: config,
		metrics: &Metrics{
			MinDuration: time.Hour,
			StatusCodes: make(map[int]int64),
			Errors:      make(map[string]int64),
			durations:   make([]time.Duration, 0, 100000),
		},
		results: make(chan *Result, 1000),
		done:    make(chan struct{}),
	}
}

// Run executes the load test
func (r *Runner) Run(ctx context.Context) (*Metrics, error) {
	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	defer cancel()

	// Setup scenario
	if err := r.config.Scenario.Setup(ctx); err != nil {
		return nil, fmt.Errorf("scenario setup failed: %w", err)
	}
	defer r.config.Scenario.Teardown(ctx)

	r.started = time.Now()

	// Start result collector
	go r.collectResults()

	// Start metrics reporter
	if r.config.ReportInterval > 0 {
		go r.reportMetrics()
	}

	// Warmup phase
	if r.config.WarmupDuration > 0 {
		fmt.Printf("Warmup phase: %v\n", r.config.WarmupDuration)
		r.runPhase(ctx, r.config.WarmupDuration, r.config.VirtualUsers/10)
	}

	// Main test phase
	fmt.Printf("Starting load test: %s\n", r.config.Name)
	fmt.Printf("Virtual Users: %d, Duration: %v, Pattern: %s\n",
		r.config.VirtualUsers, r.config.Duration, r.config.Pattern)

	r.runLoadPattern(ctx)

	// Cooldown phase
	if r.config.CooldownDuration > 0 {
		fmt.Printf("Cooldown phase: %v\n", r.config.CooldownDuration)
		r.runPhase(ctx, r.config.CooldownDuration, r.config.VirtualUsers/10)
	}

	// Wait for all workers to complete
	r.wg.Wait()
	close(r.results)
	<-r.done

	// Calculate final metrics
	r.calculateMetrics()

	return r.metrics, nil
}

// runLoadPattern executes the configured load pattern
func (r *Runner) runLoadPattern(ctx context.Context) {
	switch r.config.Pattern {
	case PatternConstant:
		r.runConstantLoad(ctx)
	case PatternRampUp:
		r.runRampUp(ctx)
	case PatternRampDown:
		r.runRampDown(ctx)
	case PatternSpike:
		r.runSpike(ctx)
	case PatternWave:
		r.runWave(ctx)
	case PatternStairs:
		r.runStairs(ctx)
	default:
		r.runConstantLoad(ctx)
	}
}

// runConstantLoad runs constant load
func (r *Runner) runConstantLoad(ctx context.Context) {
	r.runPhase(ctx, r.config.Duration, r.config.VirtualUsers)
}

// runRampUp gradually increases load
func (r *Runner) runRampUp(ctx context.Context) {
	steps := 10
	stepDuration := r.config.Duration / time.Duration(steps)
	stepUsers := r.config.VirtualUsers / steps

	for i := 1; i <= steps; i++ {
		select {
		case <-ctx.Done():
			return
		default:
			users := stepUsers * i
			r.runPhase(ctx, stepDuration, users)
		}
	}
}

// runRampDown gradually decreases load
func (r *Runner) runRampDown(ctx context.Context) {
	steps := 10
	stepDuration := r.config.Duration / time.Duration(steps)
	stepUsers := r.config.VirtualUsers / steps

	for i := steps; i >= 1; i-- {
		select {
		case <-ctx.Done():
			return
		default:
			users := stepUsers * i
			r.runPhase(ctx, stepDuration, users)
		}
	}
}

// runSpike creates traffic spike
func (r *Runner) runSpike(ctx context.Context) {
	// Normal load for 40%
	normalDuration := r.config.Duration * 4 / 10
	r.runPhase(ctx, normalDuration, r.config.VirtualUsers/2)

	// Spike for 20%
	spikeDuration := r.config.Duration * 2 / 10
	r.runPhase(ctx, spikeDuration, r.config.VirtualUsers*2)

	// Back to normal for 40%
	r.runPhase(ctx, normalDuration, r.config.VirtualUsers/2)
}

// runWave creates sine wave pattern
func (r *Runner) runWave(ctx context.Context) {
	steps := 20
	stepDuration := r.config.Duration / time.Duration(steps)

	for i := 0; i < steps; i++ {
		select {
		case <-ctx.Done():
			return
		default:
			// Calculate users using sine wave
			angle := float64(i) * 2 * math.Pi / float64(steps)
			ratio := (math.Sin(angle) + 1) / 2 // Normalize to 0-1
			users := int(float64(r.config.VirtualUsers) * ratio)
			if users < 1 {
				users = 1
			}
			r.runPhase(ctx, stepDuration, users)
		}
	}
}

// runStairs creates step increases
func (r *Runner) runStairs(ctx context.Context) {
	steps := 5
	stepDuration := r.config.Duration / time.Duration(steps)
	stepUsers := r.config.VirtualUsers / steps

	for i := 1; i <= steps; i++ {
		select {
		case <-ctx.Done():
			return
		default:
			users := stepUsers * i
			r.runPhase(ctx, stepDuration, users)
		}
	}
}

// runPhase executes a test phase
func (r *Runner) runPhase(ctx context.Context, duration time.Duration, users int) {
	phaseCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	r.metrics.mu.Lock()
	r.metrics.ConcurrentUsers = users
	r.metrics.mu.Unlock()

	// Start virtual users
	for i := 0; i < users; i++ {
		r.wg.Add(1)
		go r.runUser(phaseCtx, i)

		// Ramp up users gradually if configured
		if r.config.RampUpTime > 0 && users > 1 {
			time.Sleep(r.config.RampUpTime / time.Duration(users))
		}
	}

	// Wait for phase to complete
	<-phaseCtx.Done()
}

// runUser simulates a virtual user
func (r *Runner) runUser(ctx context.Context, userID int) {
	defer r.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Execute scenario
			result, err := r.config.Scenario.Execute(ctx, userID)
			if err != nil {
				result = &Result{
					Success:   false,
					Error:     err,
					Timestamp: time.Now(),
					UserID:    userID,
				}
			}

			// Send result
			select {
			case r.results <- result:
			case <-ctx.Done():
				return
			}

			// Think time between requests
			if r.config.ThinkTime > 0 {
				select {
				case <-time.After(r.config.ThinkTime):
				case <-ctx.Done():
					return
				}
			}

			// Rate limiting
			if r.config.MaxRPS > 0 {
				delay := time.Second / time.Duration(r.config.MaxRPS)
				time.Sleep(delay)
			}
		}
	}
}

// collectResults collects test results
func (r *Runner) collectResults() {
	for result := range r.results {
		r.recordResult(result)
	}
	close(r.done)
}

// recordResult records a single result
func (r *Runner) recordResult(result *Result) {
	r.metrics.mu.Lock()
	defer r.metrics.mu.Unlock()

	atomic.AddInt64(&r.metrics.TotalRequests, 1)

	if result.Success {
		atomic.AddInt64(&r.metrics.SuccessfulRequests, 1)
		r.metrics.durations = append(r.metrics.durations, result.Duration)

		// Update min/max
		if result.Duration < r.metrics.MinDuration {
			r.metrics.MinDuration = result.Duration
		}
		if result.Duration > r.metrics.MaxDuration {
			r.metrics.MaxDuration = result.Duration
		}

		r.metrics.TotalBytes += result.ResponseSize
	} else {
		atomic.AddInt64(&r.metrics.FailedRequests, 1)
		if result.Error != nil {
			r.metrics.Errors[result.Error.Error()]++
		}
	}

	if result.StatusCode > 0 {
		r.metrics.StatusCodes[result.StatusCode]++
	}

	r.metrics.TotalDuration += result.Duration
}

// calculateMetrics calculates final metrics
func (r *Runner) calculateMetrics() {
	r.metrics.mu.Lock()
	defer r.metrics.mu.Unlock()

	totalReqs := r.metrics.TotalRequests
	if totalReqs == 0 {
		return
	}

	// Calculate error rate
	r.metrics.ErrorRate = float64(r.metrics.FailedRequests) / float64(totalReqs) * 100

	// Calculate average duration
	if r.metrics.SuccessfulRequests > 0 {
		r.metrics.AvgDuration = r.metrics.TotalDuration / time.Duration(r.metrics.SuccessfulRequests)
	}

	// Calculate RPS
	elapsed := time.Since(r.started)
	r.metrics.RequestsPerSecond = float64(totalReqs) / elapsed.Seconds()

	// Calculate percentiles
	r.calculatePercentiles()
}

// calculatePercentiles calculates duration percentiles
func (r *Runner) calculatePercentiles() {
	if len(r.metrics.durations) == 0 {
		return
	}

	// Sort durations
	durations := make([]time.Duration, len(r.metrics.durations))
	copy(durations, r.metrics.durations)

	// Simple bubble sort for demonstration (use quicksort for production)
	for i := 0; i < len(durations); i++ {
		for j := i + 1; j < len(durations); j++ {
			if durations[i] > durations[j] {
				durations[i], durations[j] = durations[j], durations[i]
			}
		}
	}

	r.metrics.P50Duration = durations[len(durations)*50/100]
	r.metrics.P90Duration = durations[len(durations)*90/100]
	r.metrics.P95Duration = durations[len(durations)*95/100]
	r.metrics.P99Duration = durations[len(durations)*99/100]
}

// reportMetrics prints periodic metrics
func (r *Runner) reportMetrics() {
	ticker := time.NewTicker(r.config.ReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.printMetrics()
		case <-r.done:
			return
		}
	}
}

// printMetrics prints current metrics
func (r *Runner) printMetrics() {
	r.metrics.mu.RLock()
	defer r.metrics.mu.RUnlock()

	elapsed := time.Since(r.started)
	fmt.Printf("\n--- Metrics (Elapsed: %v) ---\n", elapsed.Round(time.Second))
	fmt.Printf("Total Requests: %d\n", r.metrics.TotalRequests)
	fmt.Printf("Successful: %d, Failed: %d\n", r.metrics.SuccessfulRequests, r.metrics.FailedRequests)
	fmt.Printf("Error Rate: %.2f%%\n", r.metrics.ErrorRate)
	fmt.Printf("RPS: %.2f\n", r.metrics.RequestsPerSecond)
	fmt.Printf("Avg Duration: %v\n", r.metrics.AvgDuration)
	fmt.Printf("Concurrent Users: %d\n", r.metrics.ConcurrentUsers)
	fmt.Println("---")
}

// Stop stops the load test
func (r *Runner) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
}

// PrintReport prints final test report
func PrintReport(metrics *Metrics) {
	fmt.Println("\n" + "="*60)
	fmt.Println("LOAD TEST REPORT")
	fmt.Println("="*60)
	fmt.Printf("Total Requests:       %d\n", metrics.TotalRequests)
	fmt.Printf("Successful Requests:  %d\n", metrics.SuccessfulRequests)
	fmt.Printf("Failed Requests:      %d\n", metrics.FailedRequests)
	fmt.Printf("Error Rate:           %.2f%%\n", metrics.ErrorRate)
	fmt.Printf("Requests/Second:      %.2f\n", metrics.RequestsPerSecond)
	fmt.Println()
	fmt.Println("Response Times:")
	fmt.Printf("  Min:     %v\n", metrics.MinDuration)
	fmt.Printf("  Max:     %v\n", metrics.MaxDuration)
	fmt.Printf("  Average: %v\n", metrics.AvgDuration)
	fmt.Printf("  P50:     %v\n", metrics.P50Duration)
	fmt.Printf("  P90:     %v\n", metrics.P90Duration)
	fmt.Printf("  P95:     %v\n", metrics.P95Duration)
	fmt.Printf("  P99:     %v\n", metrics.P99Duration)
	fmt.Println()
	fmt.Printf("Total Data Transfer:  %d bytes (%.2f MB)\n",
		metrics.TotalBytes, float64(metrics.TotalBytes)/(1024*1024))
	
	if len(metrics.StatusCodes) > 0 {
		fmt.Println()
		fmt.Println("Status Codes:")
		for code, count := range metrics.StatusCodes {
			fmt.Printf("  %d: %d\n", code, count)
		}
	}

	if len(metrics.Errors) > 0 {
		fmt.Println()
		fmt.Println("Top Errors:")
		count := 0
		for err, cnt := range metrics.Errors {
			if count >= 5 {
				break
			}
			fmt.Printf("  %s: %d\n", err, cnt)
			count++
		}
	}
	fmt.Println("="*60)
}
