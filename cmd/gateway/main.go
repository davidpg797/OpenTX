package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/krish567366/OpenTX/pkg/idempotency"
	"github.com/krish567366/OpenTX/pkg/iso8583"
	"github.com/krish567366/OpenTX/pkg/iso8583/packager"
	"github.com/krish567366/OpenTX/pkg/mapper"
	"github.com/krish567366/OpenTX/pkg/observability"
	"github.com/krish567366/OpenTX/pkg/security"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var (
	configPath = flag.String("config", "configs/gateway.yaml", "Path to configuration file")
	env        = flag.String("env", "development", "Environment (development/production)")
)

// Gateway is the main service that processes transactions
type Gateway struct {
	logger           *observability.Logger
	tracer           *observability.Tracer
	packagers        map[string]iso8583.Packager
	mapper           mapper.Mapper
	idempotency      idempotency.Store
	security         security.SecurityProvider
	stateMachine     *idempotency.StateMachine
}

func main() {
	flag.Parse()
	
	// Initialize logger
	logger, err := observability.NewLogger(*env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()
	
	logger.Info("Starting Canonical Transaction Gateway",
		zap.String("version", "1.0.0"),
		zap.String("env", *env),
	)
	
	// Initialize tracer
	tracer, err := observability.NewTracer("canonical-gateway", "localhost:4317")
	if err != nil {
		logger.Error("Failed to initialize tracer", zap.Error(err))
		// Continue without tracing
	}
	if tracer != nil {
		defer tracer.Shutdown(context.Background())
	}
	
	// Initialize idempotency store
	redisURL := getEnvOrDefault("REDIS_URL", "redis://localhost:6379/0")
	idempotencyStore, err := idempotency.NewRedisStore(redisURL, 24*time.Hour)
	if err != nil {
		logger.Fatal("Failed to initialize idempotency store", zap.Error(err))
	}
	defer idempotencyStore.Close()
	
	// Initialize security provider
	securityProvider := security.NewDefaultSecurityProvider(5 * time.Minute)
	
	// Load encryption keys (in production, load from HSM or KMS)
	// For demo purposes, we'll skip key loading
	
	// Initialize packagers
	packagers := map[string]iso8583.Packager{
		"VISA":       packager.NewVisaPackager(),
		"MASTERCARD": packager.NewMastercardPackager(),
		"NPCI":       packager.NewNPCIPackager(),
	}
	
	// Initialize mapper
	canonicalMapper := mapper.NewGenericMapper("GENERIC")
	
	// Initialize state machine
	stateMachine := idempotency.NewStateMachine(idempotencyStore)
	
	// Create gateway
	gateway := &Gateway{
		logger:       logger,
		tracer:       tracer,
		packagers:    packagers,
		mapper:       canonicalMapper,
		idempotency:  idempotencyStore,
		security:     securityProvider,
		stateMachine: stateMachine,
	}
	
	logger.Info("Gateway initialized successfully")
	
	// Start services
	// TODO: Start TCP listener for ISO 8583
	// TODO: Start gRPC server for canonical API
	// TODO: Start HTTP/REST API
	// TODO: Start Kafka consumer/producer
	
	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	<-sigChan
	logger.Info("Shutdown signal received, shutting down gracefully...")
	
	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	gateway.Shutdown(ctx)
	
	logger.Info("Gateway stopped")
}

// ProcessTransaction is the main transaction processing pipeline
func (g *Gateway) ProcessTransaction(ctx context.Context, rawISO8583 []byte, variant string) error {
	// Start tracing span
	var span trace.Span
	if g.tracer != nil {
		ctx, span = g.tracer.StartSpan(ctx, "process_transaction")
		defer span.End()
	}
	
	// Step 1: Parse ISO 8583 message
	ctx, parseSpan := g.tracer.StartSpan(ctx, "iso8583.parse")
	packager, exists := g.packagers[variant]
	if !exists {
		return fmt.Errorf("unsupported variant: %s", variant)
	}
	
	isoMsg, err := packager.Unpack(rawISO8583)
	if err != nil {
		g.logger.Error("Failed to parse ISO 8583 message", zap.Error(err))
		parseSpan.End()
		return err
	}
	parseSpan.End()
	
	g.logger.Info("ISO 8583 message parsed",
		zap.String("mti", isoMsg.MTI),
		zap.String("variant", variant),
		zap.String("stan", isoMsg.GetFieldAsString(11)),
	)
	
	// Step 2: Map to canonical format
	ctx, mapSpan := g.tracer.StartSpan(ctx, "canonical.map")
	canonical, err := g.mapper.ToCanonical(isoMsg)
	if err != nil {
		g.logger.Error("Failed to map to canonical", zap.Error(err))
		mapSpan.End()
		return err
	}
	mapSpan.End()
	
	// Step 3: Idempotency check
	ctx, idempotencySpan := g.tracer.StartSpan(ctx, "idempotency.check")
	txn := &idempotency.Transaction{
		IdempotencyKey: canonical.IdempotencyKey,
		MessageID:      canonical.MessageID,
		Network:        canonical.Metadata.Network,
		STAN:           canonical.Metadata.Stan,
		RRN:            canonical.Metadata.Rrn,
		State:          idempotency.StatusInit,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	
	isDuplicate, existingTxn, err := g.idempotency.CheckAndStore(ctx, canonical.IdempotencyKey, txn)
	if err != nil {
		g.logger.Error("Idempotency check failed", zap.Error(err))
		idempotencySpan.End()
		return err
	}
	idempotencySpan.End()
	
	if isDuplicate {
		g.logger.Warn("Duplicate transaction detected",
			zap.String("idempotency_key", canonical.IdempotencyKey),
			zap.String("existing_state", string(existingTxn.State)),
		)
		// Return cached response if available
		return nil
	}
	
	// Step 4: Validate and process
	// TODO: Add business logic validation
	// TODO: Route to appropriate network
	// TODO: Handle response
	// TODO: Update state machine
	// TODO: Emit events
	
	g.logger.Info("Transaction processed successfully",
		zap.String("message_id", canonical.MessageID),
		zap.String("correlation_id", canonical.CorrelationID),
	)
	
	return nil
}

// Shutdown gracefully shuts down the gateway
func (g *Gateway) Shutdown(ctx context.Context) error {
	g.logger.Info("Shutting down gateway...")
	
	// TODO: Close all connections
	// TODO: Drain in-flight transactions
	// TODO: Close stores
	
	return nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
