package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Integration Test Framework
// Supports end-to-end testing with mocks and fixtures

// TestSuite represents a collection of integration tests
type TestSuite struct {
	Name        string
	Description string
	Tests       []Test
	Setup       SetupFunc
	Teardown    TeardownFunc
	Parallel    bool
}

// Test represents a single integration test
type Test struct {
	Name        string
	Description string
	Skip        bool
	Timeout     time.Duration
	Func        TestFunc
	Fixtures    []Fixture
	Assertions  []Assertion
}

// TestFunc is the test function signature
type TestFunc func(ctx context.Context, t *TestContext) error

// SetupFunc is called before test suite
type SetupFunc func(ctx context.Context) error

// TeardownFunc is called after test suite
type TeardownFunc func(ctx context.Context) error

// TestContext provides test utilities
type TestContext struct {
	T           *testing.T
	Name        string
	Data        map[string]interface{}
	HTTPClient  *http.Client
	HTTPServer  *httptest.Server
	Mocks       *MockRegistry
	Fixtures    *FixtureRegistry
	mu          sync.RWMutex
}

// Fixture represents test data
type Fixture struct {
	Name string
	Data interface{}
	Load LoadFunc
}

// LoadFunc loads fixture data
type LoadFunc func(ctx context.Context) (interface{}, error)

// Assertion validates test results
type Assertion struct {
	Name     string
	Validate ValidateFunc
}

// ValidateFunc validates test results
type ValidateFunc func(ctx context.Context, data interface{}) error

// MockRegistry manages test mocks
type MockRegistry struct {
	mocks map[string]*Mock
	mu    sync.RWMutex
}

// Mock represents a mock service
type Mock struct {
	Name      string
	Handler   http.Handler
	Server    *httptest.Server
	Calls     []MockCall
	Responses map[string]MockResponse
	mu        sync.RWMutex
}

// MockCall records a mock invocation
type MockCall struct {
	Method    string
	Path      string
	Headers   http.Header
	Body      []byte
	Timestamp time.Time
}

// MockResponse defines mock response
type MockResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       interface{}
	Delay      time.Duration
}

// FixtureRegistry manages test fixtures
type FixtureRegistry struct {
	fixtures map[string]interface{}
	mu       sync.RWMutex
}

// NewTestContext creates a new test context
func NewTestContext(t *testing.T, name string) *TestContext {
	return &TestContext{
		T:        t,
		Name:     name,
		Data:     make(map[string]interface{}),
		Mocks:    NewMockRegistry(),
		Fixtures: NewFixtureRegistry(),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Set stores a value in context
func (tc *TestContext) Set(key string, value interface{}) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.Data[key] = value
}

// Get retrieves a value from context
func (tc *TestContext) Get(key string) (interface{}, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	val, ok := tc.Data[key]
	return val, ok
}

// AssertEqual asserts two values are equal
func (tc *TestContext) AssertEqual(expected, actual interface{}, msg string) {
	if expected != actual {
		tc.T.Errorf("%s: expected %v, got %v", msg, expected, actual)
	}
}

// AssertNotEqual asserts two values are not equal
func (tc *TestContext) AssertNotEqual(expected, actual interface{}, msg string) {
	if expected == actual {
		tc.T.Errorf("%s: expected values to differ, both are %v", msg, expected)
	}
}

// AssertNil asserts value is nil
func (tc *TestContext) AssertNil(value interface{}, msg string) {
	if value != nil {
		tc.T.Errorf("%s: expected nil, got %v", msg, value)
	}
}

// AssertNotNil asserts value is not nil
func (tc *TestContext) AssertNotNil(value interface{}, msg string) {
	if value == nil {
		tc.T.Errorf("%s: expected non-nil value", msg)
	}
}

// AssertError asserts an error occurred
func (tc *TestContext) AssertError(err error, msg string) {
	if err == nil {
		tc.T.Errorf("%s: expected error, got nil", msg)
	}
}

// AssertNoError asserts no error occurred
func (tc *TestContext) AssertNoError(err error, msg string) {
	if err != nil {
		tc.T.Errorf("%s: unexpected error: %v", msg, err)
	}
}

// AssertTrue asserts condition is true
func (tc *TestContext) AssertTrue(condition bool, msg string) {
	if !condition {
		tc.T.Errorf("%s: condition is false", msg)
	}
}

// AssertFalse asserts condition is false
func (tc *TestContext) AssertFalse(condition bool, msg string) {
	if condition {
		tc.T.Errorf("%s: condition is true", msg)
	}
}

// NewMockRegistry creates a new mock registry
func NewMockRegistry() *MockRegistry {
	return &MockRegistry{
		mocks: make(map[string]*Mock),
	}
}

// Register registers a new mock
func (mr *MockRegistry) Register(name string, handler http.Handler) *Mock {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	mock := &Mock{
		Name:      name,
		Handler:   handler,
		Calls:     make([]MockCall, 0),
		Responses: make(map[string]MockResponse),
	}

	// Wrap handler to record calls
	wrappedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock.recordCall(r)
		handler.ServeHTTP(w, r)
	})

	mock.Server = httptest.NewServer(wrappedHandler)
	mr.mocks[name] = mock

	return mock
}

// Get retrieves a mock by name
func (mr *MockRegistry) Get(name string) (*Mock, error) {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	mock, exists := mr.mocks[name]
	if !exists {
		return nil, fmt.Errorf("mock %s not found", name)
	}

	return mock, nil
}

// Close closes all mock servers
func (mr *MockRegistry) Close() {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	for _, mock := range mr.mocks {
		if mock.Server != nil {
			mock.Server.Close()
		}
	}
}

// recordCall records a mock call
func (m *Mock) recordCall(r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	body := make([]byte, 0)
	if r.Body != nil {
		// Read body (simplified for demo)
		// In production, use io.ReadAll
	}

	m.Calls = append(m.Calls, MockCall{
		Method:    r.Method,
		Path:      r.URL.Path,
		Headers:   r.Header.Clone(),
		Body:      body,
		Timestamp: time.Now(),
	})
}

// GetCalls returns all recorded calls
func (m *Mock) GetCalls() []MockCall {
	m.mu.RLock()
	defer m.mu.RUnlock()

	calls := make([]MockCall, len(m.Calls))
	copy(calls, m.Calls)
	return calls
}

// CallCount returns number of calls
func (m *Mock) CallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.Calls)
}

// Reset resets mock call history
func (m *Mock) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = make([]MockCall, 0)
}

// SetResponse sets a mock response for a path
func (m *Mock) SetResponse(path string, response MockResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Responses[path] = response
}

// NewFixtureRegistry creates a new fixture registry
func NewFixtureRegistry() *FixtureRegistry {
	return &FixtureRegistry{
		fixtures: make(map[string]interface{}),
	}
}

// Load loads a fixture
func (fr *FixtureRegistry) Load(ctx context.Context, fixture Fixture) error {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	if _, exists := fr.fixtures[fixture.Name]; exists {
		return nil // Already loaded
	}

	data, err := fixture.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load fixture %s: %w", fixture.Name, err)
	}

	fr.fixtures[fixture.Name] = data
	return nil
}

// Get retrieves a fixture
func (fr *FixtureRegistry) Get(name string) (interface{}, error) {
	fr.mu.RLock()
	defer fr.mu.RUnlock()

	data, exists := fr.fixtures[name]
	if !exists {
		return nil, fmt.Errorf("fixture %s not found", name)
	}

	return data, nil
}

// Clear clears all fixtures
func (fr *FixtureRegistry) Clear() {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.fixtures = make(map[string]interface{})
}

// Runner executes test suites
type Runner struct {
	suites []*TestSuite
	config RunnerConfig
}

// RunnerConfig holds runner configuration
type RunnerConfig struct {
	Parallel       bool
	Timeout        time.Duration
	FailFast       bool
	Verbose        bool
	ReportFormat   string
}

// NewRunner creates a new test runner
func NewRunner(config RunnerConfig) *Runner {
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Minute
	}

	return &Runner{
		suites: make([]*TestSuite, 0),
		config: config,
	}
}

// AddSuite adds a test suite
func (r *Runner) AddSuite(suite *TestSuite) {
	r.suites = append(r.suites, suite)
}

// Run executes all test suites
func (r *Runner) Run(t *testing.T) *TestReport {
	report := &TestReport{
		StartTime: time.Now(),
		Suites:    make([]*SuiteReport, 0),
	}

	for _, suite := range r.suites {
		suiteReport := r.runSuite(t, suite)
		report.Suites = append(report.Suites, suiteReport)

		if r.config.FailFast && suiteReport.Failed > 0 {
			break
		}
	}

	report.EndTime = time.Now()
	report.Duration = report.EndTime.Sub(report.StartTime)
	report.calculateTotals()

	return report
}

// runSuite executes a test suite
func (r *Runner) runSuite(t *testing.T, suite *TestSuite) *SuiteReport {
	report := &SuiteReport{
		Name:      suite.Name,
		StartTime: time.Now(),
		Tests:     make([]*TestReport, 0),
	}

	ctx := context.Background()

	// Setup
	if suite.Setup != nil {
		if err := suite.Setup(ctx); err != nil {
			report.SetupError = err
			return report
		}
	}

	// Run tests
	if suite.Parallel {
		report.Tests = r.runTestsParallel(t, suite.Tests)
	} else {
		report.Tests = r.runTestsSequential(t, suite.Tests)
	}

	// Teardown
	if suite.Teardown != nil {
		if err := suite.Teardown(ctx); err != nil {
			report.TeardownError = err
		}
	}

	report.EndTime = time.Now()
	report.Duration = report.EndTime.Sub(report.StartTime)
	report.calculateTotals()

	return report
}

// runTestsSequential runs tests sequentially
func (r *Runner) runTestsSequential(t *testing.T, tests []Test) []*TestReport {
	reports := make([]*TestReport, 0, len(tests))

	for _, test := range tests {
		if test.Skip {
			continue
		}

		report := r.runTest(t, test)
		reports = append(reports, report)

		if r.config.FailFast && !report.Passed {
			break
		}
	}

	return reports
}

// runTestsParallel runs tests in parallel
func (r *Runner) runTestsParallel(t *testing.T, tests []Test) []*TestReport {
	var wg sync.WaitGroup
	reportChan := make(chan *TestReport, len(tests))

	for _, test := range tests {
		if test.Skip {
			continue
		}

		wg.Add(1)
		go func(tst Test) {
			defer wg.Done()
			report := r.runTest(t, tst)
			reportChan <- report
		}(test)
	}

	go func() {
		wg.Wait()
		close(reportChan)
	}()

	reports := make([]*TestReport, 0)
	for report := range reportChan {
		reports = append(reports, report)
	}

	return reports
}

// runTest executes a single test
func (r *Runner) runTest(t *testing.T, test Test) *TestReport {
	report := &TestReport{
		Name:      test.Name,
		StartTime: time.Now(),
	}

	timeout := test.Timeout
	if timeout == 0 {
		timeout = r.config.Timeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	testCtx := NewTestContext(t, test.Name)
	defer testCtx.Mocks.Close()

	// Load fixtures
	for _, fixture := range test.Fixtures {
		if err := testCtx.Fixtures.Load(ctx, fixture); err != nil {
			report.Error = err
			report.Passed = false
			return report
		}
	}

	// Run test
	if err := test.Func(ctx, testCtx); err != nil {
		report.Error = err
		report.Passed = false
	} else {
		report.Passed = true
	}

	// Run assertions
	for _, assertion := range test.Assertions {
		if err := assertion.Validate(ctx, testCtx.Data); err != nil {
			report.Error = err
			report.Passed = false
			break
		}
	}

	report.EndTime = time.Now()
	report.Duration = report.EndTime.Sub(report.StartTime)

	return report
}

// TestReport represents test execution report
type TestReport struct {
	Name      string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	Passed    bool
	Error     error
	Total     int
	Failed    int
	Suites    []*SuiteReport
}

// SuiteReport represents suite execution report
type SuiteReport struct {
	Name           string
	StartTime      time.Time
	EndTime        time.Time
	Duration       time.Duration
	Total          int
	Passed         int
	Failed         int
	Skipped        int
	Tests          []*TestReport
	SetupError     error
	TeardownError  error
}

// calculateTotals calculates report totals
func (r *TestReport) calculateTotals() {
	r.Total = len(r.Suites)
	for _, suite := range r.Suites {
		if suite.Failed > 0 {
			r.Failed++
		}
	}
}

// calculateTotals calculates suite totals
func (sr *SuiteReport) calculateTotals() {
	sr.Total = len(sr.Tests)
	for _, test := range sr.Tests {
		if test.Passed {
			sr.Passed++
		} else {
			sr.Failed++
		}
	}
}

// PrintReport prints test report
func PrintReport(report *TestReport) {
	fmt.Println("\n" + "="*60)
	fmt.Println("INTEGRATION TEST REPORT")
	fmt.Println("="*60)
	fmt.Printf("Duration: %v\n", report.Duration)
	fmt.Printf("Total Suites: %d\n", report.Total)
	fmt.Printf("Failed: %d\n", report.Failed)
	fmt.Println()

	for _, suite := range report.Suites {
		fmt.Printf("Suite: %s\n", suite.Name)
		fmt.Printf("  Duration: %v\n", suite.Duration)
		fmt.Printf("  Tests: %d passed, %d failed\n", suite.Passed, suite.Failed)

		for _, test := range suite.Tests {
			status := "✓"
			if !test.Passed {
				status = "✗"
			}
			fmt.Printf("    %s %s (%v)\n", status, test.Name, test.Duration)
			if test.Error != nil {
				fmt.Printf("      Error: %v\n", test.Error)
			}
		}
		fmt.Println()
	}

	fmt.Println("="*60)
}

// Common test utilities

// WaitFor waits for a condition with timeout
func WaitFor(ctx context.Context, condition func() bool, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return errors.New("timeout waiting for condition")
		case <-ticker.C:
			if condition() {
				return nil
			}
		}
	}
}

// RetryUntilSuccess retries operation until success
func RetryUntilSuccess(ctx context.Context, fn func() error, maxRetries int, delay time.Duration) error {
	for i := 0; i < maxRetries; i++ {
		if err := fn(); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return errors.New("max retries exceeded")
}

// JSONEqual compares two JSON objects
func JSONEqual(expected, actual interface{}) (bool, error) {
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		return false, err
	}

	actualJSON, err := json.Marshal(actual)
	if err != nil {
		return false, err
	}

	return string(expectedJSON) == string(actualJSON), nil
}
