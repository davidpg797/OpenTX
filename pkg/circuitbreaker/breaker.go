package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyRequests = errors.New("too many requests")
)

// State represents circuit breaker state
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	name            string
	maxRequests     uint32
	interval        time.Duration
	timeout         time.Duration
	failureThreshold uint32
	
	mu              sync.RWMutex
	state           State
	generation      uint64
	counts          *Counts
	expiry          time.Time
	
	logger          *zap.Logger
	
	onStateChange   func(name string, from State, to State)
}

// Counts holds statistics
type Counts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

// Settings configures the circuit breaker
type Settings struct {
	Name             string
	MaxRequests      uint32        // Max requests allowed in HalfOpen state
	Interval         time.Duration // Period to clear internal counts
	Timeout          time.Duration // Period of open state before transitioning to half-open
	FailureThreshold uint32        // Number of consecutive failures to open circuit
	OnStateChange    func(name string, from State, to State)
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(settings Settings, logger *zap.Logger) *CircuitBreaker {
	cb := &CircuitBreaker{
		name:             settings.Name,
		maxRequests:      settings.MaxRequests,
		interval:         settings.Interval,
		timeout:          settings.Timeout,
		failureThreshold: settings.FailureThreshold,
		state:            StateClosed,
		counts:           &Counts{},
		logger:           logger,
		onStateChange:    settings.OnStateChange,
	}

	if cb.maxRequests == 0 {
		cb.maxRequests = 1
	}
	if cb.interval == 0 {
		cb.interval = 60 * time.Second
	}
	if cb.timeout == 0 {
		cb.timeout = 60 * time.Second
	}
	if cb.failureThreshold == 0 {
		cb.failureThreshold = 5
	}

	return cb
}

// Execute runs the given function if circuit allows
func (cb *CircuitBreaker) Execute(fn func() error) error {
	generation, err := cb.beforeRequest()
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			cb.afterRequest(generation, false)
			panic(r)
		}
	}()

	err = fn()
	cb.afterRequest(generation, err == nil)
	return err
}

// ExecuteContext runs function with context
func (cb *CircuitBreaker) ExecuteContext(ctx context.Context, fn func(context.Context) error) error {
	generation, err := cb.beforeRequest()
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			cb.afterRequest(generation, false)
			panic(r)
		}
	}()

	err = fn(ctx)
	cb.afterRequest(generation, err == nil)
	return err
}

// State returns current state
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	now := time.Now()
	state, _ := cb.currentState(now)
	return state
}

// Counts returns current statistics
func (cb *CircuitBreaker) Counts() Counts {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	return *cb.counts
}

func (cb *CircuitBreaker) beforeRequest() (uint64, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)

	if state == StateOpen {
		return generation, ErrCircuitOpen
	} else if state == StateHalfOpen && cb.counts.Requests >= cb.maxRequests {
		return generation, ErrTooManyRequests
	}

	cb.counts.Requests++
	return generation, nil
}

func (cb *CircuitBreaker) afterRequest(before uint64, success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)
	
	if generation != before {
		return
	}

	if success {
		cb.onSuccess(state, now)
	} else {
		cb.onFailure(state, now)
	}
}

func (cb *CircuitBreaker) onSuccess(state State, now time.Time) {
	switch state {
	case StateClosed:
		cb.counts.TotalSuccesses++
		cb.counts.ConsecutiveSuccesses++
		cb.counts.ConsecutiveFailures = 0
		
	case StateHalfOpen:
		cb.counts.TotalSuccesses++
		cb.counts.ConsecutiveSuccesses++
		cb.counts.ConsecutiveFailures = 0
		
		if cb.counts.ConsecutiveSuccesses >= cb.maxRequests {
			cb.setState(StateClosed, now)
		}
	}
}

func (cb *CircuitBreaker) onFailure(state State, now time.Time) {
	switch state {
	case StateClosed:
		cb.counts.TotalFailures++
		cb.counts.ConsecutiveFailures++
		cb.counts.ConsecutiveSuccesses = 0
		
		if cb.counts.ConsecutiveFailures >= cb.failureThreshold {
			cb.setState(StateOpen, now)
		}
		
	case StateHalfOpen:
		cb.setState(StateOpen, now)
	}
}

func (cb *CircuitBreaker) currentState(now time.Time) (State, uint64) {
	switch cb.state {
	case StateClosed:
		if !cb.expiry.IsZero() && cb.expiry.Before(now) {
			cb.toNewGeneration(now)
		}
	case StateOpen:
		if cb.expiry.Before(now) {
			cb.setState(StateHalfOpen, now)
		}
	}
	return cb.state, cb.generation
}

func (cb *CircuitBreaker) setState(state State, now time.Time) {
	if cb.state == state {
		return
	}

	prev := cb.state
	cb.state = state

	cb.toNewGeneration(now)

	if state == StateOpen {
		cb.expiry = now.Add(cb.timeout)
	}

	if cb.onStateChange != nil {
		cb.onStateChange(cb.name, prev, state)
	}

	cb.logger.Info("circuit breaker state changed",
		zap.String("name", cb.name),
		zap.String("from", prev.String()),
		zap.String("to", state.String()))
}

func (cb *CircuitBreaker) toNewGeneration(now time.Time) {
	cb.generation++
	cb.counts = &Counts{}

	if cb.state == StateClosed {
		cb.expiry = now.Add(cb.interval)
	} else {
		cb.expiry = time.Time{}
	}
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.toNewGeneration(time.Now())
	cb.state = StateClosed
	cb.expiry = time.Time{}
}

// CircuitBreakerGroup manages multiple circuit breakers
type CircuitBreakerGroup struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	logger   *zap.Logger
}

// NewCircuitBreakerGroup creates a new group
func NewCircuitBreakerGroup(logger *zap.Logger) *CircuitBreakerGroup {
	return &CircuitBreakerGroup{
		breakers: make(map[string]*CircuitBreaker),
		logger:   logger,
	}
}

// Get returns or creates a circuit breaker
func (g *CircuitBreakerGroup) Get(name string, settings Settings) *CircuitBreaker {
	g.mu.RLock()
	cb, exists := g.breakers[name]
	g.mu.RUnlock()

	if exists {
		return cb
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Double-check
	if cb, exists := g.breakers[name]; exists {
		return cb
	}

	settings.Name = name
	cb = NewCircuitBreaker(settings, g.logger)
	g.breakers[name] = cb

	return cb
}

// Remove removes a circuit breaker
func (g *CircuitBreakerGroup) Remove(name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.breakers, name)
}

// GetAll returns all circuit breakers
func (g *CircuitBreakerGroup) GetAll() map[string]*CircuitBreaker {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make(map[string]*CircuitBreaker, len(g.breakers))
	for k, v := range g.breakers {
		result[k] = v
	}
	return result
}
