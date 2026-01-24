package retry

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"

	"go.uber.org/zap"
)

var (
	ErrMaxRetriesExceeded = errors.New("maximum retries exceeded")
)

// Strategy defines retry behavior
type Strategy interface {
	NextDelay(attempt int) time.Duration
	ShouldRetry(attempt int, err error) bool
}

// Policy defines retry policy
type Policy struct {
	MaxAttempts int
	MaxDuration time.Duration
	Strategy    Strategy
	OnRetry     func(attempt int, err error, delay time.Duration)
	Logger      *zap.Logger
}

// Execute executes function with retry logic
func (p *Policy) Execute(ctx context.Context, fn func(context.Context) error) error {
	return p.ExecuteWithValue(ctx, func(ctx context.Context) (interface{}, error) {
		return nil, fn(ctx)
	})
}

// ExecuteWithValue executes function returning value with retry logic
func (p *Policy) ExecuteWithValue(ctx context.Context, fn func(context.Context) (interface{}, error)) (interface{}, error) {
	startTime := time.Now()
	var lastErr error

	for attempt := 0; attempt < p.MaxAttempts; attempt++ {
		// Check timeout
		if p.MaxDuration > 0 && time.Since(startTime) > p.MaxDuration {
			return nil, ErrMaxRetriesExceeded
		}

		// Try execution
		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if should retry
		if !p.Strategy.ShouldRetry(attempt, err) {
			return nil, err
		}

		// Last attempt, no delay needed
		if attempt == p.MaxAttempts-1 {
			break
		}

		// Calculate delay
		delay := p.Strategy.NextDelay(attempt)

		// Call retry callback
		if p.OnRetry != nil {
			p.OnRetry(attempt, err, delay)
		}

		if p.Logger != nil {
			p.Logger.Warn("retrying after error",
				zap.Int("attempt", attempt+1),
				zap.Int("max_attempts", p.MaxAttempts),
				zap.Duration("delay", delay),
				zap.Error(err))
		}

		// Wait for delay or context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}

	return nil, lastErr
}

// FixedDelayStrategy uses fixed delay between retries
type FixedDelayStrategy struct {
	Delay time.Duration
}

func (s *FixedDelayStrategy) NextDelay(attempt int) time.Duration {
	return s.Delay
}

func (s *FixedDelayStrategy) ShouldRetry(attempt int, err error) bool {
	return !isNonRetryableError(err)
}

// ExponentialBackoffStrategy uses exponential backoff
type ExponentialBackoffStrategy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	Jitter       bool
}

func (s *ExponentialBackoffStrategy) NextDelay(attempt int) time.Duration {
	if s.InitialDelay == 0 {
		s.InitialDelay = 100 * time.Millisecond
	}
	if s.MaxDelay == 0 {
		s.MaxDelay = 30 * time.Second
	}
	if s.Multiplier == 0 {
		s.Multiplier = 2.0
	}

	delay := float64(s.InitialDelay) * math.Pow(s.Multiplier, float64(attempt))
	
	if delay > float64(s.MaxDelay) {
		delay = float64(s.MaxDelay)
	}

	// Add jitter
	if s.Jitter {
		jitter := rand.Float64() * delay * 0.1 // 10% jitter
		delay += jitter
	}

	return time.Duration(delay)
}

func (s *ExponentialBackoffStrategy) ShouldRetry(attempt int, err error) bool {
	return !isNonRetryableError(err)
}

// LinearBackoffStrategy uses linear backoff
type LinearBackoffStrategy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Increment    time.Duration
}

func (s *LinearBackoffStrategy) NextDelay(attempt int) time.Duration {
	delay := s.InitialDelay + time.Duration(attempt)*s.Increment
	
	if s.MaxDelay > 0 && delay > s.MaxDelay {
		delay = s.MaxDelay
	}

	return delay
}

func (s *LinearBackoffStrategy) ShouldRetry(attempt int, err error) bool {
	return !isNonRetryableError(err)
}

// DecorrelatedJitterStrategy uses AWS decorrelated jitter
type DecorrelatedJitterStrategy struct {
	BaseDelay time.Duration
	MaxDelay  time.Duration
	prevDelay time.Duration
}

func (s *DecorrelatedJitterStrategy) NextDelay(attempt int) time.Duration {
	if s.BaseDelay == 0 {
		s.BaseDelay = 100 * time.Millisecond
	}
	if s.MaxDelay == 0 {
		s.MaxDelay = 30 * time.Second
	}

	if s.prevDelay == 0 {
		s.prevDelay = s.BaseDelay
	}

	// temp = min(max_delay, random_between(base_delay, prev_delay * 3))
	temp := s.BaseDelay + time.Duration(rand.Int63n(int64(s.prevDelay*3-s.BaseDelay)))
	
	if temp > s.MaxDelay {
		temp = s.MaxDelay
	}

	s.prevDelay = temp
	return temp
}

func (s *DecorrelatedJitterStrategy) ShouldRetry(attempt int, err error) bool {
	return !isNonRetryableError(err)
}

// ConditionalStrategy retries based on error type
type ConditionalStrategy struct {
	BaseStrategy Strategy
	RetryableErrors []error
	NonRetryableErrors []error
}

func (s *ConditionalStrategy) NextDelay(attempt int) time.Duration {
	return s.BaseStrategy.NextDelay(attempt)
}

func (s *ConditionalStrategy) ShouldRetry(attempt int, err error) bool {
	// Check non-retryable errors first
	for _, nonRetryable := range s.NonRetryableErrors {
		if errors.Is(err, nonRetryable) {
			return false
		}
	}

	// Check retryable errors
	if len(s.RetryableErrors) > 0 {
		for _, retryable := range s.RetryableErrors {
			if errors.Is(err, retryable) {
				return s.BaseStrategy.ShouldRetry(attempt, err)
			}
		}
		return false
	}

	return s.BaseStrategy.ShouldRetry(attempt, err)
}

// Helper function to check if error is non-retryable
func isNonRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Add common non-retryable errors
	nonRetryable := []error{
		context.Canceled,
		context.DeadlineExceeded,
	}

	for _, nr := range nonRetryable {
		if errors.Is(err, nr) {
			return true
		}
	}

	return false
}

// RetryExecutor manages multiple retry policies
type RetryExecutor struct {
	policies map[string]*Policy
	logger   *zap.Logger
}

// NewRetryExecutor creates a new retry executor
func NewRetryExecutor(logger *zap.Logger) *RetryExecutor {
	return &RetryExecutor{
		policies: make(map[string]*Policy),
		logger:   logger,
	}
}

// RegisterPolicy registers a named policy
func (re *RetryExecutor) RegisterPolicy(name string, policy *Policy) {
	policy.Logger = re.logger
	re.policies[name] = policy
}

// Execute executes with named policy
func (re *RetryExecutor) Execute(ctx context.Context, policyName string, fn func(context.Context) error) error {
	policy, exists := re.policies[policyName]
	if !exists {
		return errors.New("policy not found: " + policyName)
	}

	return policy.Execute(ctx, fn)
}

// DefaultPolicies returns common retry policies
func DefaultPolicies(logger *zap.Logger) map[string]*Policy {
	return map[string]*Policy{
		"fast": {
			MaxAttempts: 3,
			MaxDuration: 5 * time.Second,
			Strategy: &ExponentialBackoffStrategy{
				InitialDelay: 100 * time.Millisecond,
				MaxDelay:     1 * time.Second,
				Multiplier:   2.0,
				Jitter:       true,
			},
			Logger: logger,
		},
		"standard": {
			MaxAttempts: 5,
			MaxDuration: 30 * time.Second,
			Strategy: &ExponentialBackoffStrategy{
				InitialDelay: 500 * time.Millisecond,
				MaxDelay:     10 * time.Second,
				Multiplier:   2.0,
				Jitter:       true,
			},
			Logger: logger,
		},
		"aggressive": {
			MaxAttempts: 10,
			MaxDuration: 2 * time.Minute,
			Strategy: &ExponentialBackoffStrategy{
				InitialDelay: 1 * time.Second,
				MaxDelay:     30 * time.Second,
				Multiplier:   2.0,
				Jitter:       true,
			},
			Logger: logger,
		},
		"linear": {
			MaxAttempts: 5,
			MaxDuration: 30 * time.Second,
			Strategy: &LinearBackoffStrategy{
				InitialDelay: 1 * time.Second,
				MaxDelay:     10 * time.Second,
				Increment:    2 * time.Second,
			},
			Logger: logger,
		},
	}
}

// Retry is a convenience function for quick retries
func Retry(ctx context.Context, attempts int, delay time.Duration, fn func() error) error {
	policy := &Policy{
		MaxAttempts: attempts,
		Strategy: &FixedDelayStrategy{
			Delay: delay,
		},
	}

	return policy.Execute(ctx, func(ctx context.Context) error {
		return fn()
	})
}

// RetryWithBackoff is a convenience function with exponential backoff
func RetryWithBackoff(ctx context.Context, attempts int, fn func() error) error {
	policy := &Policy{
		MaxAttempts: attempts,
		Strategy: &ExponentialBackoffStrategy{
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     10 * time.Second,
			Multiplier:   2.0,
			Jitter:       true,
		},
	}

	return policy.Execute(ctx, func(ctx context.Context) error {
		return fn()
	})
}
