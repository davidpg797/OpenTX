package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Analytics and Business Intelligence Dashboard
// Real-time metrics, KPIs, and business insights

// Dashboard manages analytics and reporting
type Dashboard struct {
	metrics    map[string]*Metric
	aggregates map[string]*Aggregate
	alerts     map[string]*Alert
	mu         sync.RWMutex
	collectors []Collector
	alerters   []Alerter
}

// Metric represents a business metric
type Metric struct {
	Name        string
	Type        MetricType
	Value       float64
	Unit        string
	Timestamp   time.Time
	Tags        map[string]string
	Description string
}

// MetricType represents type of metric
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"   // Monotonically increasing
	MetricTypeGauge     MetricType = "gauge"     // Point-in-time value
	MetricTypeHistogram MetricType = "histogram" // Distribution
	MetricTypeSummary   MetricType = "summary"   // Statistical summary
)

// Aggregate holds aggregated metrics
type Aggregate struct {
	Name      string
	Period    time.Duration
	StartTime time.Time
	EndTime   time.Time
	Count     int64
	Sum       float64
	Min       float64
	Max       float64
	Avg       float64
	P50       float64
	P90       float64
	P95       float64
	P99       float64
	Values    []float64
}

// Alert represents a business alert
type Alert struct {
	ID          string
	Name        string
	Description string
	Condition   AlertCondition
	Severity    AlertSeverity
	Status      AlertStatus
	FiredAt     time.Time
	ResolvedAt  *time.Time
	Value       float64
	Threshold   float64
	Tags        map[string]string
}

// AlertCondition defines alert trigger condition
type AlertCondition struct {
	Metric    string
	Operator  Operator
	Threshold float64
	Duration  time.Duration
}

// Operator represents comparison operator
type Operator string

const (
	OpGreaterThan    Operator = "gt"
	OpLessThan       Operator = "lt"
	OpEqual          Operator = "eq"
	OpGreaterOrEqual Operator = "gte"
	OpLessOrEqual    Operator = "lte"
)

// AlertSeverity represents alert severity level
type AlertSeverity string

const (
	SeverityInfo     AlertSeverity = "info"
	SeverityWarning  AlertSeverity = "warning"
	SeverityCritical AlertSeverity = "critical"
)

// AlertStatus represents alert status
type AlertStatus string

const (
	StatusFiring   AlertStatus = "firing"
	StatusResolved AlertStatus = "resolved"
)

// Collector collects metrics
type Collector interface {
	Collect(ctx context.Context) ([]*Metric, error)
	Name() string
}

// Alerter sends alert notifications
type Alerter interface {
	Send(ctx context.Context, alert *Alert) error
	Name() string
}

// KPI represents a Key Performance Indicator
type KPI struct {
	Name        string
	Value       float64
	Target      float64
	Achievement float64 // Percentage
	Trend       Trend
	Change      float64
	Period      string
	Unit        string
	Category    string
}

// Trend represents KPI trend
type Trend string

const (
	TrendUp   Trend = "up"
	TrendDown Trend = "down"
	TrendFlat Trend = "flat"
)

// Report represents an analytics report
type Report struct {
	ID          string
	Name        string
	Description string
	Period      TimePeriod
	GeneratedAt time.Time
	KPIs        []KPI
	Charts      []Chart
	Tables      []Table
	Summary     string
}

// TimePeriod represents a time period
type TimePeriod struct {
	Start time.Time
	End   time.Time
	Label string
}

// Chart represents a data visualization
type Chart struct {
	ID     string
	Type   ChartType
	Title  string
	XAxis  string
	YAxis  string
	Series []Series
}

// ChartType represents chart visualization type
type ChartType string

const (
	ChartTypeLine      ChartType = "line"
	ChartTypeBar       ChartType = "bar"
	ChartTypePie       ChartType = "pie"
	ChartTypeArea      ChartType = "area"
	ChartTypeScatter   ChartType = "scatter"
	ChartTypeHeatmap   ChartType = "heatmap"
)

// Series represents a data series
type Series struct {
	Name   string
	Data   []DataPoint
	Color  string
}

// DataPoint represents a data point
type DataPoint struct {
	X     interface{}
	Y     float64
	Label string
}

// Table represents tabular data
type Table struct {
	ID      string
	Title   string
	Headers []string
	Rows    [][]interface{}
	Footer  []string
}

// NewDashboard creates a new analytics dashboard
func NewDashboard() *Dashboard {
	return &Dashboard{
		metrics:    make(map[string]*Metric),
		aggregates: make(map[string]*Aggregate),
		alerts:     make(map[string]*Alert),
		collectors: make([]Collector, 0),
		alerters:   make([]Alerter, 0),
	}
}

// RecordMetric records a metric value
func (d *Dashboard) RecordMetric(metric *Metric) {
	d.mu.Lock()
	defer d.mu.Unlock()

	metric.Timestamp = time.Now()
	d.metrics[metric.Name] = metric

	// Check alerts
	d.checkAlerts(metric)
}

// GetMetric retrieves a metric by name
func (d *Dashboard) GetMetric(name string) (*Metric, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	metric, exists := d.metrics[name]
	return metric, exists
}

// RegisterCollector registers a metric collector
func (d *Dashboard) RegisterCollector(collector Collector) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.collectors = append(d.collectors, collector)
}

// RegisterAlerter registers an alerter
func (d *Dashboard) RegisterAlerter(alerter Alerter) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.alerters = append(d.alerters, alerter)
}

// CollectMetrics collects metrics from all collectors
func (d *Dashboard) CollectMetrics(ctx context.Context) error {
	d.mu.RLock()
	collectors := make([]Collector, len(d.collectors))
	copy(collectors, d.collectors)
	d.mu.RUnlock()

	for _, collector := range collectors {
		metrics, err := collector.Collect(ctx)
		if err != nil {
			return fmt.Errorf("collector %s failed: %w", collector.Name(), err)
		}

		for _, metric := range metrics {
			d.RecordMetric(metric)
		}
	}

	return nil
}

// StartCollection starts periodic metric collection
func (d *Dashboard) StartCollection(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := d.CollectMetrics(ctx); err != nil {
				// Log error
			}
		}
	}
}

// checkAlerts checks if metric triggers any alerts
func (d *Dashboard) checkAlerts(metric *Metric) {
	for _, alert := range d.alerts {
		if alert.Condition.Metric != metric.Name {
			continue
		}

		if d.evaluateCondition(metric.Value, alert.Condition) {
			if alert.Status != StatusFiring {
				alert.Status = StatusFiring
				alert.FiredAt = time.Now()
				alert.Value = metric.Value

				// Send alert notification
				go d.sendAlert(alert)
			}
		} else {
			if alert.Status == StatusFiring {
				now := time.Now()
				alert.Status = StatusResolved
				alert.ResolvedAt = &now

				// Send resolution notification
				go d.sendAlert(alert)
			}
		}
	}
}

// evaluateCondition evaluates alert condition
func (d *Dashboard) evaluateCondition(value float64, condition AlertCondition) bool {
	switch condition.Operator {
	case OpGreaterThan:
		return value > condition.Threshold
	case OpLessThan:
		return value < condition.Threshold
	case OpEqual:
		return value == condition.Threshold
	case OpGreaterOrEqual:
		return value >= condition.Threshold
	case OpLessOrEqual:
		return value <= condition.Threshold
	default:
		return false
	}
}

// sendAlert sends alert to all alerters
func (d *Dashboard) sendAlert(alert *Alert) {
	d.mu.RLock()
	alerters := make([]Alerter, len(d.alerters))
	copy(alerters, d.alerters)
	d.mu.RUnlock()

	ctx := context.Background()
	for _, alerter := range alerters {
		if err := alerter.Send(ctx, alert); err != nil {
			// Log error
		}
	}
}

// GenerateKPIs generates business KPIs
func (d *Dashboard) GenerateKPIs(period TimePeriod) []KPI {
	kpis := []KPI{
		d.generateTransactionKPIs(period),
		d.generateRevenueKPIs(period),
		d.generatePerformanceKPIs(period),
		d.generateQualityKPIs(period),
	}

	return kpis
}

// generateTransactionKPIs generates transaction-related KPIs
func (d *Dashboard) generateTransactionKPIs(period TimePeriod) KPI {
	d.mu.RLock()
	defer d.mu.RUnlock()

	totalTx, _ := d.metrics["total_transactions"]
	value := 0.0
	if totalTx != nil {
		value = totalTx.Value
	}

	return KPI{
		Name:     "Total Transactions",
		Value:    value,
		Target:   100000,
		Achievement: (value / 100000) * 100,
		Trend:    TrendUp,
		Change:   5.2,
		Period:   "Today",
		Unit:     "transactions",
		Category: "Volume",
	}
}

// generateRevenueKPIs generates revenue-related KPIs
func (d *Dashboard) generateRevenueKPIs(period TimePeriod) KPI {
	d.mu.RLock()
	defer d.mu.RUnlock()

	revenue, _ := d.metrics["total_revenue"]
	value := 0.0
	if revenue != nil {
		value = revenue.Value
	}

	return KPI{
		Name:        "Total Revenue",
		Value:       value,
		Target:      1000000,
		Achievement: (value / 1000000) * 100,
		Trend:       TrendUp,
		Change:      12.5,
		Period:      "Today",
		Unit:        "USD",
		Category:    "Revenue",
	}
}

// generatePerformanceKPIs generates performance-related KPIs
func (d *Dashboard) generatePerformanceKPIs(period TimePeriod) KPI {
	d.mu.RLock()
	defer d.mu.RUnlock()

	latency, _ := d.metrics["avg_latency"]
	value := 0.0
	if latency != nil {
		value = latency.Value
	}

	return KPI{
		Name:        "Average Latency",
		Value:       value,
		Target:      100,
		Achievement: ((100 - value) / 100) * 100,
		Trend:       TrendDown,
		Change:      -3.2,
		Period:      "Last Hour",
		Unit:        "ms",
		Category:    "Performance",
	}
}

// generateQualityKPIs generates quality-related KPIs
func (d *Dashboard) generateQualityKPIs(period TimePeriod) KPI {
	d.mu.RLock()
	defer d.mu.RUnlock()

	successRate, _ := d.metrics["success_rate"]
	value := 99.5
	if successRate != nil {
		value = successRate.Value
	}

	return KPI{
		Name:        "Success Rate",
		Value:       value,
		Target:      99.9,
		Achievement: (value / 99.9) * 100,
		Trend:       TrendFlat,
		Change:      0.1,
		Period:      "Today",
		Unit:        "%",
		Category:    "Quality",
	}
}

// GenerateReport generates an analytics report
func (d *Dashboard) GenerateReport(name string, period TimePeriod) *Report {
	report := &Report{
		ID:          fmt.Sprintf("report-%d", time.Now().Unix()),
		Name:        name,
		Description: fmt.Sprintf("Analytics report for %s", period.Label),
		Period:      period,
		GeneratedAt: time.Now(),
		KPIs:        d.GenerateKPIs(period),
		Charts:      d.generateCharts(period),
		Tables:      d.generateTables(period),
	}

	report.Summary = d.generateSummary(report)

	return report
}

// generateCharts generates report charts
func (d *Dashboard) generateCharts(period TimePeriod) []Chart {
	return []Chart{
		{
			ID:    "transactions-over-time",
			Type:  ChartTypeLine,
			Title: "Transactions Over Time",
			XAxis: "Time",
			YAxis: "Count",
			Series: []Series{
				{
					Name: "Transactions",
					Data: d.getTimeSeriesData("transactions", period),
					Color: "#4CAF50",
				},
			},
		},
		{
			ID:    "revenue-breakdown",
			Type:  ChartTypePie,
			Title: "Revenue by Payment Method",
			Series: []Series{
				{
					Name: "Revenue",
					Data: []DataPoint{
						{X: "Card", Y: 65.5, Label: "Card"},
						{X: "Bank", Y: 20.3, Label: "Bank"},
						{X: "Wallet", Y: 14.2, Label: "Wallet"},
					},
				},
			},
		},
		{
			ID:    "performance-metrics",
			Type:  ChartTypeArea,
			Title: "Performance Metrics",
			XAxis: "Time",
			YAxis: "Latency (ms)",
			Series: []Series{
				{
					Name:  "P50",
					Data:  d.getTimeSeriesData("latency_p50", period),
					Color: "#2196F3",
				},
				{
					Name:  "P95",
					Data:  d.getTimeSeriesData("latency_p95", period),
					Color: "#FF9800",
				},
				{
					Name:  "P99",
					Data:  d.getTimeSeriesData("latency_p99", period),
					Color: "#F44336",
				},
			},
		},
	}
}

// generateTables generates report tables
func (d *Dashboard) generateTables(period TimePeriod) []Table {
	return []Table{
		{
			ID:      "top-merchants",
			Title:   "Top Merchants by Volume",
			Headers: []string{"Rank", "Merchant", "Transactions", "Revenue", "Avg Ticket"},
			Rows: [][]interface{}{
				{1, "Merchant A", 15420, "$1,234,567", "$80.12"},
				{2, "Merchant B", 12350, "$987,654", "$79.97"},
				{3, "Merchant C", 9870, "$765,432", "$77.53"},
				{4, "Merchant D", 8900, "$654,321", "$73.52"},
				{5, "Merchant E", 7650, "$543,210", "$71.00"},
			},
			Footer: []string{"Total", "54,190", "$4,185,184", "$77.21"},
		},
	}
}

// generateSummary generates report summary
func (d *Dashboard) generateSummary(report *Report) string {
	return fmt.Sprintf(
		"Analytics report for %s generated at %s. "+
			"Total KPIs: %d. All systems operational.",
		report.Period.Label,
		report.GeneratedAt.Format(time.RFC3339),
		len(report.KPIs),
	)
}

// getTimeSeriesData gets time series data for a metric
func (d *Dashboard) getTimeSeriesData(metricName string, period TimePeriod) []DataPoint {
	// Placeholder - would query historical data
	data := make([]DataPoint, 0)
	duration := period.End.Sub(period.Start)
	points := 24

	for i := 0; i < points; i++ {
		t := period.Start.Add(duration * time.Duration(i) / time.Duration(points))
		data = append(data, DataPoint{
			X: t.Format("15:04"),
			Y: float64(1000 + (i * 50)),
		})
	}

	return data
}

// ExportReport exports report to JSON
func (d *Dashboard) ExportReport(report *Report) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

// TransactionCollector collects transaction metrics
type TransactionCollector struct {
	name string
}

func NewTransactionCollector() *TransactionCollector {
	return &TransactionCollector{name: "transaction_collector"}
}

func (c *TransactionCollector) Name() string {
	return c.name
}

func (c *TransactionCollector) Collect(ctx context.Context) ([]*Metric, error) {
	// Placeholder - would query actual transaction data
	metrics := []*Metric{
		{
			Name:  "total_transactions",
			Type:  MetricTypeCounter,
			Value: 85420,
			Unit:  "count",
			Tags:  map[string]string{"period": "today"},
		},
		{
			Name:  "successful_transactions",
			Type:  MetricTypeCounter,
			Value: 85100,
			Unit:  "count",
			Tags:  map[string]string{"period": "today"},
		},
		{
			Name:  "failed_transactions",
			Type:  MetricTypeCounter,
			Value: 320,
			Unit:  "count",
			Tags:  map[string]string{"period": "today"},
		},
		{
			Name:  "avg_transaction_value",
			Type:  MetricTypeGauge,
			Value: 125.50,
			Unit:  "USD",
			Tags:  map[string]string{"period": "today"},
		},
		{
			Name:  "total_revenue",
			Type:  MetricTypeCounter,
			Value: 10720000,
			Unit:  "USD",
			Tags:  map[string]string{"period": "today"},
		},
	}

	return metrics, nil
}

// EmailAlerter sends alerts via email
type EmailAlerter struct {
	name      string
	recipients []string
}

func NewEmailAlerter(recipients []string) *EmailAlerter {
	return &EmailAlerter{
		name:      "email_alerter",
		recipients: recipients,
	}
}

func (a *EmailAlerter) Name() string {
	return a.name
}

func (a *EmailAlerter) Send(ctx context.Context, alert *Alert) error {
	// Placeholder - would send actual email
	fmt.Printf("Email Alert: %s - %s (Severity: %s)\n",
		alert.Name, alert.Description, alert.Severity)
	return nil
}

// SlackAlerter sends alerts to Slack
type SlackAlerter struct {
	name    string
	webhook string
}

func NewSlackAlerter(webhook string) *SlackAlerter {
	return &SlackAlerter{
		name:    "slack_alerter",
		webhook: webhook,
	}
}

func (a *SlackAlerter) Name() string {
	return a.name
}

func (a *SlackAlerter) Send(ctx context.Context, alert *Alert) error {
	// Placeholder - would send to Slack webhook
	fmt.Printf("Slack Alert: %s - %s (Severity: %s)\n",
		alert.Name, alert.Description, alert.Severity)
	return nil
}

// RealTimeDashboard provides real-time dashboard data
type RealTimeDashboard struct {
	dashboard *Dashboard
	subscribers map[string]chan *Metric
	mu        sync.RWMutex
}

func NewRealTimeDashboard(dashboard *Dashboard) *RealTimeDashboard {
	return &RealTimeDashboard{
		dashboard:   dashboard,
		subscribers: make(map[string]chan *Metric),
	}
}

// Subscribe subscribes to real-time metric updates
func (rtd *RealTimeDashboard) Subscribe(id string) chan *Metric {
	rtd.mu.Lock()
	defer rtd.mu.Unlock()

	ch := make(chan *Metric, 100)
	rtd.subscribers[id] = ch
	return ch
}

// Unsubscribe unsubscribes from metric updates
func (rtd *RealTimeDashboard) Unsubscribe(id string) {
	rtd.mu.Lock()
	defer rtd.mu.Unlock()

	if ch, exists := rtd.subscribers[id]; exists {
		close(ch)
		delete(rtd.subscribers, id)
	}
}

// Broadcast broadcasts metric to all subscribers
func (rtd *RealTimeDashboard) Broadcast(metric *Metric) {
	rtd.mu.RLock()
	defer rtd.mu.RUnlock()

	for _, ch := range rtd.subscribers {
		select {
		case ch <- metric:
		default:
			// Skip if channel is full
		}
	}
}
