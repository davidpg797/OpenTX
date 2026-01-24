package monitoring

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// Metrics holds all application metrics
type Metrics struct {
	// Transaction metrics
	TransactionTotal         *prometheus.CounterVec
	TransactionDuration      *prometheus.HistogramVec
	TransactionSize          *prometheus.HistogramVec
	TransactionErrorTotal    *prometheus.CounterVec
	
	// Network metrics
	NetworkRequestTotal      *prometheus.CounterVec
	NetworkRequestDuration   *prometheus.HistogramVec
	NetworkErrorTotal        *prometheus.CounterVec
	NetworkConnectionPool    *prometheus.GaugeVec
	
	// Cache metrics
	CacheHitTotal            *prometheus.CounterVec
	CacheMissTotal           *prometheus.CounterVec
	CacheOperationDuration   *prometheus.HistogramVec
	
	// Queue metrics
	QueueSize                *prometheus.GaugeVec
	QueueProcessingDuration  *prometheus.HistogramVec
	QueueErrorTotal          *prometheus.CounterVec
	
	// Business metrics
	AuthorizationTotal       *prometheus.CounterVec
	DeclineTotal             *prometheus.CounterVec
	ApprovalRate             *prometheus.GaugeVec
	AverageTicketSize        *prometheus.GaugeVec
	
	// System metrics
	GoroutineCount           prometheus.Gauge
	MemoryUsage              prometheus.Gauge
	CPUUsage                 prometheus.Gauge
	
	// Custom metrics
	customMetrics            sync.Map
}

// NewMetrics creates and registers Prometheus metrics
func NewMetrics(namespace string) *Metrics {
	m := &Metrics{
		// Transaction metrics
		TransactionTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "transactions_total",
				Help:      "Total number of transactions processed",
			},
			[]string{"type", "status", "network"},
		),
		TransactionDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "transaction_duration_seconds",
				Help:      "Transaction processing duration in seconds",
				Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"type", "network"},
		),
		TransactionSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "transaction_size_bytes",
				Help:      "Transaction message size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 2, 10),
			},
			[]string{"type", "direction"},
		),
		TransactionErrorTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "transaction_errors_total",
				Help:      "Total number of transaction errors",
			},
			[]string{"type", "error_type"},
		),
		
		// Network metrics
		NetworkRequestTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "network_requests_total",
				Help:      "Total number of network requests",
			},
			[]string{"network", "operation"},
		),
		NetworkRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "network_request_duration_seconds",
				Help:      "Network request duration in seconds",
				Buckets:   []float64{.01, .025, .05, .1, .25, .5, 1, 2, 5},
			},
			[]string{"network", "operation"},
		),
		NetworkErrorTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "network_errors_total",
				Help:      "Total number of network errors",
			},
			[]string{"network", "error_type"},
		),
		NetworkConnectionPool: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "network_connection_pool",
				Help:      "Number of connections in pool",
			},
			[]string{"network", "state"},
		),
		
		// Cache metrics
		CacheHitTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_hits_total",
				Help:      "Total number of cache hits",
			},
			[]string{"cache_name"},
		),
		CacheMissTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_misses_total",
				Help:      "Total number of cache misses",
			},
			[]string{"cache_name"},
		),
		CacheOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "cache_operation_duration_seconds",
				Help:      "Cache operation duration in seconds",
				Buckets:   []float64{.0001, .0005, .001, .005, .01, .025, .05, .1},
			},
			[]string{"cache_name", "operation"},
		),
		
		// Queue metrics
		QueueSize: promauto.NewGaugeVec(
			prometheus.GaugeVec{
				Namespace: namespace,
				Name:      "queue_size",
				Help:      "Current size of processing queue",
			},
			[]string{"queue_name"},
		),
		QueueProcessingDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "queue_processing_duration_seconds",
				Help:      "Queue item processing duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"queue_name"},
		),
		QueueErrorTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "queue_errors_total",
				Help:      "Total number of queue processing errors",
			},
			[]string{"queue_name", "error_type"},
		),
		
		// Business metrics
		AuthorizationTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "authorizations_total",
				Help:      "Total number of authorization requests",
			},
			[]string{"network", "response_code"},
		),
		DeclineTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "declines_total",
				Help:      "Total number of declined transactions",
			},
			[]string{"network", "reason"},
		),
		ApprovalRate: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "approval_rate",
				Help:      "Transaction approval rate percentage",
			},
			[]string{"network"},
		),
		AverageTicketSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "average_ticket_size",
				Help:      "Average transaction amount",
			},
			[]string{"network", "currency"},
		),
		
		// System metrics
		GoroutineCount: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "goroutines",
				Help:      "Number of goroutines",
			},
		),
		MemoryUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "memory_usage_bytes",
				Help:      "Memory usage in bytes",
			},
		),
		CPUUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "cpu_usage_percent",
				Help:      "CPU usage percentage",
			},
		),
	}

	return m
}

// RecordTransaction records transaction metrics
func (m *Metrics) RecordTransaction(txnType, status, network string, duration time.Duration, size int) {
	m.TransactionTotal.WithLabelValues(txnType, status, network).Inc()
	m.TransactionDuration.WithLabelValues(txnType, network).Observe(duration.Seconds())
	m.TransactionSize.WithLabelValues(txnType, "outbound").Observe(float64(size))
}

// RecordNetworkRequest records network request metrics
func (m *Metrics) RecordNetworkRequest(network, operation string, duration time.Duration, err error) {
	m.NetworkRequestTotal.WithLabelValues(network, operation).Inc()
	m.NetworkRequestDuration.WithLabelValues(network, operation).Observe(duration.Seconds())
	
	if err != nil {
		m.NetworkErrorTotal.WithLabelValues(network, "request_failed").Inc()
	}
}

// RecordCacheOperation records cache metrics
func (m *Metrics) RecordCacheOperation(cacheName, operation string, hit bool, duration time.Duration) {
	if hit {
		m.CacheHitTotal.WithLabelValues(cacheName).Inc()
	} else {
		m.CacheMissTotal.WithLabelValues(cacheName).Inc()
	}
	m.CacheOperationDuration.WithLabelValues(cacheName, operation).Observe(duration.Seconds())
}

// UpdateQueueSize updates queue size gauge
func (m *Metrics) UpdateQueueSize(queueName string, size int) {
	m.QueueSize.WithLabelValues(queueName).Set(float64(size))
}

// RecordQueueProcessing records queue processing metrics
func (m *Metrics) RecordQueueProcessing(queueName string, duration time.Duration, err error) {
	m.QueueProcessingDuration.WithLabelValues(queueName).Observe(duration.Seconds())
	
	if err != nil {
		m.QueueErrorTotal.WithLabelValues(queueName, "processing_failed").Inc()
	}
}

// HealthCheck represents system health status
type HealthCheck struct {
	Status      string                 `json:"status"`
	Timestamp   time.Time              `json:"timestamp"`
	Checks      map[string]CheckResult `json:"checks"`
	Version     string                 `json:"version"`
	Uptime      time.Duration          `json:"uptime"`
}

// CheckResult represents individual health check result
type CheckResult struct {
	Status    string        `json:"status"`
	Message   string        `json:"message,omitempty"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time     `json:"timestamp"`
}

// HealthChecker performs health checks
type HealthChecker struct {
	checks    map[string]HealthCheckFunc
	mu        sync.RWMutex
	startTime time.Time
	version   string
	logger    *zap.Logger
}

// HealthCheckFunc is a health check function
type HealthCheckFunc func(context.Context) CheckResult

// NewHealthChecker creates a new health checker
func NewHealthChecker(version string, logger *zap.Logger) *HealthChecker {
	return &HealthChecker{
		checks:    make(map[string]HealthCheckFunc),
		startTime: time.Now(),
		version:   version,
		logger:    logger,
	}
}

// Register registers a health check
func (hc *HealthChecker) Register(name string, fn HealthCheckFunc) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.checks[name] = fn
}

// Check performs all health checks
func (hc *HealthChecker) Check(ctx context.Context) HealthCheck {
	hc.mu.RLock()
	checks := make(map[string]HealthCheckFunc, len(hc.checks))
	for k, v := range hc.checks {
		checks[k] = v
	}
	hc.mu.RUnlock()

	results := make(map[string]CheckResult)
	overallStatus := "healthy"

	for name, fn := range checks {
		start := time.Now()
		result := fn(ctx)
		result.Duration = time.Since(start)
		result.Timestamp = time.Now()
		
		results[name] = result

		if result.Status != "healthy" {
			overallStatus = "unhealthy"
		}
	}

	return HealthCheck{
		Status:    overallStatus,
		Timestamp: time.Now(),
		Checks:    results,
		Version:   hc.version,
		Uptime:    time.Since(hc.startTime),
	}
}

// Readiness checks if service is ready to accept traffic
func (hc *HealthChecker) Readiness(ctx context.Context) bool {
	check := hc.Check(ctx)
	return check.Status == "healthy"
}

// Liveness checks if service is alive
func (hc *HealthChecker) Liveness(ctx context.Context) bool {
	// Simple liveness check - just respond
	return true
}

// AlertManager manages alerts
type AlertManager struct {
	mu       sync.RWMutex
	alerts   map[string]*Alert
	handlers []AlertHandler
	logger   *zap.Logger
}

// Alert represents an alert
type Alert struct {
	ID          string
	Name        string
	Severity    string
	Message     string
	Timestamp   time.Time
	Labels      map[string]string
	Resolved    bool
	ResolvedAt  time.Time
}

// AlertHandler handles alerts
type AlertHandler interface {
	Handle(alert *Alert) error
}

// NewAlertManager creates a new alert manager
func NewAlertManager(logger *zap.Logger) *AlertManager {
	return &AlertManager{
		alerts:   make(map[string]*Alert),
		handlers: make([]AlertHandler, 0),
		logger:   logger,
	}
}

// RegisterHandler registers an alert handler
func (am *AlertManager) RegisterHandler(handler AlertHandler) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.handlers = append(am.handlers, handler)
}

// Fire fires an alert
func (am *AlertManager) Fire(alert *Alert) {
	am.mu.Lock()
	alert.Timestamp = time.Now()
	am.alerts[alert.ID] = alert
	handlers := make([]AlertHandler, len(am.handlers))
	copy(handlers, am.handlers)
	am.mu.Unlock()

	am.logger.Warn("alert fired",
		zap.String("id", alert.ID),
		zap.String("name", alert.Name),
		zap.String("severity", alert.Severity),
		zap.String("message", alert.Message))

	// Call handlers
	for _, handler := range handlers {
		go func(h AlertHandler) {
			if err := h.Handle(alert); err != nil {
				am.logger.Error("alert handler failed", zap.Error(err))
			}
		}(handler)
	}
}

// Resolve resolves an alert
func (am *AlertManager) Resolve(alertID string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if alert, exists := am.alerts[alertID]; exists {
		alert.Resolved = true
		alert.ResolvedAt = time.Now()
		am.logger.Info("alert resolved", zap.String("id", alertID))
	}
}

// GetActiveAlerts returns active alerts
func (am *AlertManager) GetActiveAlerts() []*Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var active []*Alert
	for _, alert := range am.alerts {
		if !alert.Resolved {
			active = append(active, alert)
		}
	}

	return active
}
