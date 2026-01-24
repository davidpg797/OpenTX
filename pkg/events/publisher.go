package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// Publisher publishes transaction events
type Publisher interface {
	// Publish publishes an event
	Publish(ctx context.Context, event *TransactionEvent) error
	
	// PublishBatch publishes multiple events
	PublishBatch(ctx context.Context, events []*TransactionEvent) error
	
	// Close closes the publisher
	Close() error
}

// KafkaPublisher publishes events to Kafka
type KafkaPublisher struct {
	writer *kafka.Writer
	topic  string
	logger *zap.Logger
}

// NewKafkaPublisher creates a new Kafka publisher
func NewKafkaPublisher(brokers []string, topic string, logger *zap.Logger) *KafkaPublisher {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{}, // Use hash balancing for ordering by key
		Compression:  kafka.Snappy,
		BatchSize:    100,
		BatchTimeout: 10 * time.Millisecond,
		RequiredAcks: kafka.RequireAll,
		MaxAttempts:  3,
		ErrorLogger:  kafka.LoggerFunc(logger.Sugar().Errorf),
	}
	
	return &KafkaPublisher{
		writer: writer,
		topic:  topic,
		logger: logger,
	}
}

// Publish publishes a single event
func (p *KafkaPublisher) Publish(ctx context.Context, event *TransactionEvent) error {
	// Serialize event
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	
	// Create Kafka message
	msg := kafka.Message{
		Key:   []byte(event.Identifiers.IdempotencyKey), // Use idempotency key for ordering
		Value: data,
		Time:  time.Now(),
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte(event.EventType)},
			{Key: "correlation_id", Value: []byte(event.CorrelationId)},
			{Key: "network", Value: []byte(event.Identifiers.Network)},
		},
	}
	
	// Write message
	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		p.logger.Error("Failed to publish event to Kafka",
			zap.String("event_id", event.EventId),
			zap.String("event_type", event.EventType),
			zap.Error(err),
		)
		return fmt.Errorf("failed to write to Kafka: %w", err)
	}
	
	p.logger.Debug("Event published to Kafka",
		zap.String("event_id", event.EventId),
		zap.String("event_type", event.EventType),
		zap.String("correlation_id", event.CorrelationId),
	)
	
	return nil
}

// PublishBatch publishes multiple events
func (p *KafkaPublisher) PublishBatch(ctx context.Context, events []*TransactionEvent) error {
	if len(events) == 0 {
		return nil
	}
	
	messages := make([]kafka.Message, 0, len(events))
	
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			p.logger.Error("Failed to marshal event",
				zap.String("event_id", event.EventId),
				zap.Error(err),
			)
			continue
		}
		
		msg := kafka.Message{
			Key:   []byte(event.Identifiers.IdempotencyKey),
			Value: data,
			Time:  time.Now(),
			Headers: []kafka.Header{
				{Key: "event_type", Value: []byte(event.EventType)},
				{Key: "correlation_id", Value: []byte(event.CorrelationId)},
				{Key: "network", Value: []byte(event.Identifiers.Network)},
			},
		}
		
		messages = append(messages, msg)
	}
	
	if err := p.writer.WriteMessages(ctx, messages...); err != nil {
		p.logger.Error("Failed to publish batch to Kafka",
			zap.Int("batch_size", len(messages)),
			zap.Error(err),
		)
		return fmt.Errorf("failed to write batch to Kafka: %w", err)
	}
	
	p.logger.Debug("Event batch published to Kafka",
		zap.Int("batch_size", len(messages)),
	)
	
	return nil
}

// Close closes the publisher
func (p *KafkaPublisher) Close() error {
	return p.writer.Close()
}

// EventBuilder helps build transaction events
type EventBuilder struct {
	correlationID string
	network       string
}

// NewEventBuilder creates a new event builder
func NewEventBuilder(correlationID, network string) *EventBuilder {
	return &EventBuilder{
		correlationID: correlationID,
		network:       network,
	}
}

// BuildAuthRequestedEvent builds an authorization requested event
func (b *EventBuilder) BuildAuthRequestedEvent(
	messageID, stan, rrn string,
	amount int64, currency string,
	merchantID, merchantName, terminalID string,
	cardLast4 string,
	metadata map[string]string,
) *TransactionEvent {
	return &TransactionEvent{
		EventId:       uuid.New().String(),
		EventType:     "auth.requested",
		EventTimestamp: timestampNow(),
		CorrelationId: b.correlationID,
		Identifiers: &TransactionIdentifiers{
			MessageId:      messageID,
			Stan:           stan,
			Rrn:            rrn,
			Network:        b.network,
		},
		Payload: &TransactionEvent_AuthRequested{
			AuthRequested: &AuthorizationRequestedEvent{
				Amount: &Money{
					Amount:       amount,
					CurrencyCode: currency,
					DecimalPlaces: 2,
					Sign:         Sign_SIGN_POSITIVE,
				},
				MerchantId:       merchantID,
				MerchantName:     merchantName,
				TerminalId:       terminalID,
				CardLast4:        cardLast4,
				TransactionDatetime: timestampNow(),
				Metadata:         metadata,
			},
		},
		Context: &EventContext{
			SourceService: "canonical-gateway",
			TraceId:       b.correlationID,
		},
	}
}

// BuildAuthApprovedEvent builds an authorization approved event
func (b *EventBuilder) BuildAuthApprovedEvent(
	messageID, stan, rrn, authID string,
	approvedAmount int64,
	responseCode string,
) *TransactionEvent {
	return &TransactionEvent{
		EventId:       uuid.New().String(),
		EventType:     "auth.approved",
		EventTimestamp: timestampNow(),
		CorrelationId: b.correlationID,
		Identifiers: &TransactionIdentifiers{
			MessageId: messageID,
			Stan:      stan,
			Rrn:       rrn,
			AuthId:    authID,
			Network:   b.network,
		},
		Payload: &TransactionEvent_AuthApproved{
			AuthApproved: &AuthorizationApprovedEvent{
				ApprovedAmount: &Money{
					Amount:       approvedAmount,
					CurrencyCode: "USD", // TODO: Get from context
					DecimalPlaces: 2,
					Sign:         Sign_SIGN_POSITIVE,
				},
				AuthId:          authID,
				ResponseCode:    responseCode,
				ApprovalDatetime: timestampNow(),
			},
		},
		Context: &EventContext{
			SourceService: "canonical-gateway",
			TraceId:       b.correlationID,
		},
	}
}

// BuildAuthDeclinedEvent builds an authorization declined event
func (b *EventBuilder) BuildAuthDeclinedEvent(
	messageID, stan, rrn string,
	responseCode, declineReason string,
) *TransactionEvent {
	return &TransactionEvent{
		EventId:       uuid.New().String(),
		EventType:     "auth.declined",
		EventTimestamp: timestampNow(),
		CorrelationId: b.correlationID,
		Identifiers: &TransactionIdentifiers{
			MessageId: messageID,
			Stan:      stan,
			Rrn:       rrn,
			Network:   b.network,
		},
		Payload: &TransactionEvent_AuthDeclined{
			AuthDeclined: &AuthorizationDeclinedEvent{
				ResponseCode:    responseCode,
				DeclineReason:   declineReason,
				DeclineDatetime: timestampNow(),
			},
		},
		Context: &EventContext{
			SourceService: "canonical-gateway",
			TraceId:       b.correlationID,
		},
	}
}

// BuildReversalSentEvent builds a reversal sent event
func (b *EventBuilder) BuildReversalSentEvent(
	messageID, stan, rrn string,
	originalStan, originalRRN string,
	amount int64,
	reason string,
) *TransactionEvent {
	return &TransactionEvent{
		EventId:       uuid.New().String(),
		EventType:     "reversal.sent",
		EventTimestamp: timestampNow(),
		CorrelationId: b.correlationID,
		Identifiers: &TransactionIdentifiers{
			MessageId: messageID,
			Stan:      stan,
			Rrn:       rrn,
			Network:   b.network,
		},
		Payload: &TransactionEvent_ReversalSent{
			ReversalSent: &ReversalSentEvent{
				ReversalAmount: &Money{
					Amount:       amount,
					CurrencyCode: "USD",
					DecimalPlaces: 2,
					Sign:         Sign_SIGN_POSITIVE,
				},
				OriginalStan:     originalStan,
				OriginalRrn:      originalRRN,
				ReversalDatetime: timestampNow(),
			},
		},
		Context: &EventContext{
			SourceService: "canonical-gateway",
			TraceId:       b.correlationID,
		},
	}
}

// BuildTransactionFailedEvent builds a transaction failed event
func (b *EventBuilder) BuildTransactionFailedEvent(
	messageID, stan, rrn string,
	errorCode, errorMessage, reason string,
) *TransactionEvent {
	return &TransactionEvent{
		EventId:       uuid.New().String(),
		EventType:     "transaction.failed",
		EventTimestamp: timestampNow(),
		CorrelationId: b.correlationID,
		Identifiers: &TransactionIdentifiers{
			MessageId: messageID,
			Stan:      stan,
			Rrn:       rrn,
			Network:   b.network,
		},
		Payload: &TransactionEvent_TransactionFailed{
			TransactionFailed: &TransactionFailedEvent{
				ErrorCode:       errorCode,
				ErrorMessage:    errorMessage,
				FailureDatetime: timestampNow(),
			},
		},
		Context: &EventContext{
			SourceService: "canonical-gateway",
			TraceId:       b.correlationID,
		},
	}
}

func timestampNow() *Timestamp {
	now := time.Now()
	return &Timestamp{
		Seconds: now.Unix(),
		Nanos:   int32(now.Nanosecond()),
	}
}

// Mock types for compilation (these should be generated from proto)
type TransactionEvent struct {
	EventId        string
	EventType      string
	EventTimestamp *Timestamp
	CorrelationId  string
	Identifiers    *TransactionIdentifiers
	Payload        interface{}
	Context        *EventContext
}

type TransactionIdentifiers struct {
	MessageId      string
	Stan           string
	Rrn            string
	AuthId         string
	IdempotencyKey string
	Network        string
}

type EventContext struct {
	SourceService string
	TraceId       string
}

type AuthorizationRequestedEvent struct {
	Amount              *Money
	MerchantId          string
	MerchantName        string
	TerminalId          string
	CardLast4           string
	TransactionDatetime *Timestamp
	Metadata            map[string]string
}

type AuthorizationApprovedEvent struct {
	ApprovedAmount   *Money
	AuthId           string
	ResponseCode     string
	ApprovalDatetime *Timestamp
}

type AuthorizationDeclinedEvent struct {
	ResponseCode    string
	DeclineReason   string
	DeclineDatetime *Timestamp
}

type ReversalSentEvent struct {
	ReversalAmount   *Money
	OriginalStan     string
	OriginalRrn      string
	ReversalDatetime *Timestamp
}

type TransactionFailedEvent struct {
	ErrorCode       string
	ErrorMessage    string
	FailureDatetime *Timestamp
}

type Money struct {
	Amount        int64
	CurrencyCode  string
	DecimalPlaces int32
	Sign          Sign
}

type Sign int32

const (
	Sign_SIGN_UNKNOWN  Sign = 0
	Sign_SIGN_POSITIVE Sign = 1
	Sign_SIGN_NEGATIVE Sign = 2
)

type Timestamp struct {
	Seconds int64
	Nanos   int32
}

type TransactionEvent_AuthRequested struct {
	AuthRequested *AuthorizationRequestedEvent
}

type TransactionEvent_AuthApproved struct {
	AuthApproved *AuthorizationApprovedEvent
}

type TransactionEvent_AuthDeclined struct {
	AuthDeclined *AuthorizationDeclinedEvent
}

type TransactionEvent_ReversalSent struct {
	ReversalSent *ReversalSentEvent
}

type TransactionEvent_TransactionFailed struct {
	TransactionFailed *TransactionFailedEvent
}
