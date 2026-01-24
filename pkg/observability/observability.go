package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Tracer provides distributed tracing capabilities
type Tracer struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
}

// Logger provides structured logging
type Logger struct {
	*zap.Logger
}

// NewTracer initializes OpenTelemetry tracing
func NewTracer(serviceName, otlpEndpoint string) (*Tracer, error) {
	ctx := context.Background()
	
	// Create OTLP exporter
	exporter, err := otlptrace.New(
		ctx,
		otlptracegrpc.NewClient(
			otlptracegrpc.WithEndpoint(otlpEndpoint),
			otlptracegrpc.WithInsecure(), // Use TLS in production
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}
	
	// Create resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}
	
	// Create tracer provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	
	// Set global provider
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	
	tracer := provider.Tracer(serviceName)
	
	return &Tracer{
		provider: provider,
		tracer:   tracer,
	}, nil
}

// StartSpan starts a new span
func (t *Tracer) StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

// Shutdown gracefully shuts down the tracer
func (t *Tracer) Shutdown(ctx context.Context) error {
	return t.provider.Shutdown(ctx)
}

// NewLogger creates a new structured logger
func NewLogger(env string) (*Logger, error) {
	var config zap.Config
	
	if env == "production" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	
	logger, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	
	return &Logger{logger}, nil
}

// WithCorrelationID adds correlation ID to logger
func (l *Logger) WithCorrelationID(correlationID string) *Logger {
	return &Logger{l.With(zap.String("correlation_id", correlationID))}
}

// WithMessageID adds message ID to logger
func (l *Logger) WithMessageID(messageID string) *Logger {
	return &Logger{l.With(zap.String("message_id", messageID))}
}

// WithNetwork adds network to logger
func (l *Logger) WithNetwork(network string) *Logger {
	return &Logger{l.With(zap.String("network", network))}
}

// LogTransaction logs transaction details
func (l *Logger) LogTransaction(
	level zapcore.Level,
	msg string,
	mti string,
	stan string,
	rrn string,
	amount int64,
	currency string,
	responseCode string,
) {
	l.Log(level, msg,
		zap.String("mti", mti),
		zap.String("stan", stan),
		zap.String("rrn", rrn),
		zap.Int64("amount", amount),
		zap.String("currency", currency),
		zap.String("response_code", responseCode),
	)
}

// LogISO8583Message logs ISO 8583 message details
func (l *Logger) LogISO8583Message(level zapcore.Level, msg string, isoMsg map[string]interface{}) {
	fields := make([]zap.Field, 0, len(isoMsg))
	for k, v := range isoMsg {
		fields = append(fields, zap.Any(k, v))
	}
	l.Log(level, msg, fields...)
}

// TransactionLogger provides transaction-specific logging
type TransactionLogger struct {
	*Logger
	correlationID string
	messageID     string
	network       string
}

// NewTransactionLogger creates a logger for a specific transaction
func NewTransactionLogger(baseLogger *Logger, correlationID, messageID, network string) *TransactionLogger {
	return &TransactionLogger{
		Logger:        baseLogger.WithCorrelationID(correlationID).WithMessageID(messageID).WithNetwork(network),
		correlationID: correlationID,
		messageID:     messageID,
		network:       network,
	}
}

// SpanAttributes provides common span attributes for transactions
type SpanAttributes struct {
	Network      string
	MTI          string
	STAN         string
	RRN          string
	TerminalID   string
	MerchantID   string
	Amount       int64
	Currency     string
	ResponseCode string
}

// ToAttributes converts to OpenTelemetry attributes
func (a *SpanAttributes) ToAttributes() []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("payment.network", a.Network),
		attribute.String("payment.mti", a.MTI),
		attribute.String("payment.stan", a.STAN),
		attribute.String("payment.rrn", a.RRN),
		attribute.String("payment.terminal_id", a.TerminalID),
		attribute.String("payment.merchant_id", a.MerchantID),
		attribute.Int64("payment.amount", a.Amount),
		attribute.String("payment.currency", a.Currency),
	}
	
	if a.ResponseCode != "" {
		attrs = append(attrs, attribute.String("payment.response_code", a.ResponseCode))
	}
	
	return attrs
}

// Metrics provides Prometheus-compatible metrics
type Metrics struct {
	// TODO: Implement Prometheus metrics
	// - Transaction counter (by network, MTI, response code)
	// - Latency histogram (by hop: ingress, parse, map, send, response)
	// - Error counter (by error type)
	// - Reversal counter
	// - Timeout counter
}

// MetricsLabels provides common labels for metrics
type MetricsLabels struct {
	Network      string
	MTI          string
	ResponseCode string
	ErrorType    string
	Stage        string
}
