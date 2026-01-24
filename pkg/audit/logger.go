package audit

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Event represents an audit event
type Event struct {
	ID            string                 `json:"id"`
	Timestamp     time.Time              `json:"timestamp"`
	EventType     string                 `json:"event_type"`
	Actor         Actor                  `json:"actor"`
	Resource      Resource               `json:"resource"`
	Action        string                 `json:"action"`
	Result        string                 `json:"result"` // success, failure, partial
	Details       map[string]interface{} `json:"details"`
	IPAddress     string                 `json:"ip_address"`
	UserAgent     string                 `json:"user_agent"`
	SessionID     string                 `json:"session_id"`
	CorrelationID string                 `json:"correlation_id"`
	Severity      string                 `json:"severity"` // info, warning, critical
	Metadata      map[string]interface{} `json:"metadata"`
}

// Actor represents who performed the action
type Actor struct {
	ID   string `json:"id"`
	Type string `json:"type"` // user, system, service, api_key
	Name string `json:"name"`
}

// Resource represents what was affected
type Resource struct {
	ID   string `json:"id"`
	Type string `json:"type"` // transaction, user, configuration, etc.
	Name string `json:"name"`
}

// Logger provides audit logging
type Logger struct {
	storage  Storage
	async    bool
	queue    chan *Event
	logger   *zap.Logger
	
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	
	// Filters
	filters  []Filter
}

// Storage interface for audit event storage
type Storage interface {
	Store(ctx context.Context, event *Event) error
	Query(ctx context.Context, query *Query) ([]*Event, error)
	Count(ctx context.Context, query *Query) (int64, error)
}

// Filter filters audit events
type Filter interface {
	ShouldLog(event *Event) bool
}

// Query represents audit query
type Query struct {
	EventTypes    []string
	ActorIDs      []string
	ResourceTypes []string
	Actions       []string
	StartTime     time.Time
	EndTime       time.Time
	Limit         int
	Offset        int
}

// Config configures audit logger
type Config struct {
	Storage Storage
	Async   bool
	Workers int
}

// NewLogger creates a new audit logger
func NewLogger(config Config, logger *zap.Logger) *Logger {
	ctx, cancel := context.WithCancel(context.Background())

	al := &Logger{
		storage: config.Storage,
		async:   config.Async,
		logger:  logger,
		ctx:     ctx,
		cancel:  cancel,
		filters: make([]Filter, 0),
	}

	if config.Async {
		if config.Workers == 0 {
			config.Workers = 5
		}
		al.queue = make(chan *Event, 1000)
		
		// Start workers
		for i := 0; i < config.Workers; i++ {
			al.wg.Add(1)
			go al.worker(i)
		}
	}

	return al
}

// Log logs an audit event
func (al *Logger) Log(ctx context.Context, event *Event) error {
	// Apply filters
	for _, filter := range al.filters {
		if !filter.ShouldLog(event) {
			return nil
		}
	}

	// Set defaults
	if event.ID == "" {
		event.ID = generateEventID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Severity == "" {
		event.Severity = "info"
	}

	if al.async {
		select {
		case al.queue <- event:
			return nil
		default:
			al.logger.Warn("audit queue full, dropping event",
				zap.String("event_id", event.ID))
			return nil
		}
	}

	return al.storage.Store(ctx, event)
}

// LogTransaction logs a transaction audit event
func (al *Logger) LogTransaction(ctx context.Context, txnID string, action string, actor Actor, result string, details map[string]interface{}) error {
	event := &Event{
		EventType: "transaction",
		Actor:     actor,
		Resource: Resource{
			ID:   txnID,
			Type: "transaction",
		},
		Action:  action,
		Result:  result,
		Details: details,
	}

	return al.Log(ctx, event)
}

// LogConfigChange logs configuration change
func (al *Logger) LogConfigChange(ctx context.Context, configKey string, actor Actor, oldValue, newValue interface{}) error {
	event := &Event{
		EventType: "configuration",
		Actor:     actor,
		Resource: Resource{
			ID:   configKey,
			Type: "configuration",
			Name: configKey,
		},
		Action: "update",
		Result: "success",
		Details: map[string]interface{}{
			"old_value": oldValue,
			"new_value": newValue,
		},
		Severity: "warning",
	}

	return al.Log(ctx, event)
}

// LogAuthentication logs authentication events
func (al *Logger) LogAuthentication(ctx context.Context, userID string, result string, details map[string]interface{}) error {
	event := &Event{
		EventType: "authentication",
		Actor: Actor{
			ID:   userID,
			Type: "user",
		},
		Resource: Resource{
			ID:   userID,
			Type: "user",
		},
		Action:   "login",
		Result:   result,
		Details:  details,
		Severity: getSeverityForAuthResult(result),
	}

	return al.Log(ctx, event)
}

// LogSecurityEvent logs security-related events
func (al *Logger) LogSecurityEvent(ctx context.Context, eventType string, actor Actor, details map[string]interface{}) error {
	event := &Event{
		EventType: "security",
		Actor:     actor,
		Action:    eventType,
		Details:   details,
		Severity:  "critical",
	}

	return al.Log(ctx, event)
}

// Query queries audit events
func (al *Logger) Query(ctx context.Context, query *Query) ([]*Event, error) {
	return al.storage.Query(ctx, query)
}

// Count counts matching audit events
func (al *Logger) Count(ctx context.Context, query *Query) (int64, error) {
	return al.storage.Count(ctx, query)
}

// AddFilter adds an event filter
func (al *Logger) AddFilter(filter Filter) {
	al.filters = append(al.filters, filter)
}

// Stop stops the audit logger
func (al *Logger) Stop() {
	if al.async {
		al.cancel()
		close(al.queue)
		al.wg.Wait()
	}
}

func (al *Logger) worker(id int) {
	defer al.wg.Done()

	al.logger.Info("audit worker started", zap.Int("worker_id", id))

	for {
		select {
		case <-al.ctx.Done():
			return
		case event, ok := <-al.queue:
			if !ok {
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := al.storage.Store(ctx, event); err != nil {
				al.logger.Error("failed to store audit event",
					zap.String("event_id", event.ID),
					zap.Error(err))
			}
			cancel()
		}
	}
}

// MemoryStorage implements in-memory audit storage
type MemoryStorage struct {
	mu     sync.RWMutex
	events []*Event
}

// NewMemoryStorage creates in-memory storage
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		events: make([]*Event, 0),
	}
}

func (ms *MemoryStorage) Store(ctx context.Context, event *Event) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	ms.events = append(ms.events, event)
	return nil
}

func (ms *MemoryStorage) Query(ctx context.Context, query *Query) ([]*Event, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var results []*Event

	for _, event := range ms.events {
		if ms.matchesQuery(event, query) {
			results = append(results, event)
		}
	}

	// Apply pagination
	if query.Offset > 0 {
		if query.Offset >= len(results) {
			return []*Event{}, nil
		}
		results = results[query.Offset:]
	}

	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}

	return results, nil
}

func (ms *MemoryStorage) Count(ctx context.Context, query *Query) (int64, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var count int64
	for _, event := range ms.events {
		if ms.matchesQuery(event, query) {
			count++
		}
	}

	return count, nil
}

func (ms *MemoryStorage) matchesQuery(event *Event, query *Query) bool {
	// Event type filter
	if len(query.EventTypes) > 0 {
		matched := false
		for _, et := range query.EventTypes {
			if event.EventType == et {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Actor ID filter
	if len(query.ActorIDs) > 0 {
		matched := false
		for _, aid := range query.ActorIDs {
			if event.Actor.ID == aid {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Resource type filter
	if len(query.ResourceTypes) > 0 {
		matched := false
		for _, rt := range query.ResourceTypes {
			if event.Resource.Type == rt {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Action filter
	if len(query.Actions) > 0 {
		matched := false
		for _, action := range query.Actions {
			if event.Action == action {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Time range filter
	if !query.StartTime.IsZero() && event.Timestamp.Before(query.StartTime) {
		return false
	}
	if !query.EndTime.IsZero() && event.Timestamp.After(query.EndTime) {
		return false
	}

	return true
}

// SeverityFilter filters by severity
type SeverityFilter struct {
	MinSeverity string
}

func (sf *SeverityFilter) ShouldLog(event *Event) bool {
	severityLevels := map[string]int{
		"info":     1,
		"warning":  2,
		"critical": 3,
	}

	minLevel := severityLevels[sf.MinSeverity]
	eventLevel := severityLevels[event.Severity]

	return eventLevel >= minLevel
}

// EventTypeFilter filters by event type
type EventTypeFilter struct {
	AllowedTypes []string
}

func (etf *EventTypeFilter) ShouldLog(event *Event) bool {
	if len(etf.AllowedTypes) == 0 {
		return true
	}

	for _, allowed := range etf.AllowedTypes {
		if event.EventType == allowed {
			return true
		}
	}

	return false
}

// Helpers

func generateEventID() string {
	return time.Now().Format("20060102150405.000000")
}

func getSeverityForAuthResult(result string) string {
	if result == "success" {
		return "info"
	}
	return "warning"
}

// Export exports audit events
func Export(events []*Event, format string) ([]byte, error) {
	switch format {
	case "json":
		return json.MarshalIndent(events, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// ComplianceReporter generates compliance reports
type ComplianceReporter struct {
	logger  *Logger
	storage Storage
}

// NewComplianceReporter creates compliance reporter
func NewComplianceReporter(logger *Logger, storage Storage) *ComplianceReporter {
	return &ComplianceReporter{
		logger:  logger,
		storage: storage,
	}
}

// GenerateReport generates compliance report
func (cr *ComplianceReporter) GenerateReport(ctx context.Context, startTime, endTime time.Time) (*ComplianceReport, error) {
	query := &Query{
		StartTime: startTime,
		EndTime:   endTime,
	}

	events, err := cr.storage.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	report := &ComplianceReport{
		StartTime:     startTime,
		EndTime:       endTime,
		GeneratedAt:   time.Now(),
		TotalEvents:   len(events),
		EventsByType:  make(map[string]int),
		EventsByActor: make(map[string]int),
	}

	for _, event := range events {
		report.EventsByType[event.EventType]++
		report.EventsByActor[event.Actor.Type]++

		if event.Severity == "critical" {
			report.CriticalEvents++
		}
	}

	return report, nil
}

// ComplianceReport represents compliance report
type ComplianceReport struct {
	StartTime      time.Time      `json:"start_time"`
	EndTime        time.Time      `json:"end_time"`
	GeneratedAt    time.Time      `json:"generated_at"`
	TotalEvents    int            `json:"total_events"`
	CriticalEvents int            `json:"critical_events"`
	EventsByType   map[string]int `json:"events_by_type"`
	EventsByActor  map[string]int `json:"events_by_actor"`
}
