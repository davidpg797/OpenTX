package idempotency

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Store provides idempotency and deduplication
type Store interface {
	// CheckAndStore checks if a transaction exists, stores if not
	CheckAndStore(ctx context.Context, key string, txn *Transaction) (bool, *Transaction, error)
	
	// Get retrieves a transaction by key
	Get(ctx context.Context, key string) (*Transaction, error)
	
	// UpdateState updates transaction state
	UpdateState(ctx context.Context, key string, state TransactionStatus) error
	
	// IsDuplicate checks if transaction is a duplicate
	IsDuplicate(ctx context.Context, dedup *DeduplicationKey) (bool, error)
}

// RedisStore implements Store using Redis
type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
}

// Transaction represents stored transaction data
type Transaction struct {
	IdempotencyKey string            `json:"idempotency_key"`
	MessageID      string            `json:"message_id"`
	Network        string            `json:"network"`
	STAN           string            `json:"stan"`
	RRN            string            `json:"rrn"`
	Amount         int64             `json:"amount"`
	TerminalID     string            `json:"terminal_id"`
	MerchantID     string            `json:"merchant_id"`
	State          TransactionStatus `json:"state"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	Response       []byte            `json:"response,omitempty"`
}

// DeduplicationKey contains fields for duplicate detection
type DeduplicationKey struct {
	Network    string
	STAN       string
	RRN        string
	Amount     int64
	TerminalID string
	MerchantID string
	Timestamp  time.Time
}

// TransactionStatus represents transaction lifecycle states
type TransactionStatus string

const (
	StatusInit     TransactionStatus = "INIT"
	StatusSent     TransactionStatus = "SENT"
	StatusAcked    TransactionStatus = "ACKED"
	StatusApproved TransactionStatus = "APPROVED"
	StatusDeclined TransactionStatus = "DECLINED"
	StatusReversed TransactionStatus = "REVERSED"
	StatusSettled  TransactionStatus = "SETTLED"
	StatusFailed   TransactionStatus = "FAILED"
)

// NewRedisStore creates a new Redis-backed store
func NewRedisStore(redisURL string, ttl time.Duration) (*RedisStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}
	
	client := redis.NewClient(opts)
	
	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	
	return &RedisStore{
		client: client,
		ttl:    ttl,
	}, nil
}

// CheckAndStore atomically checks and stores a transaction
func (s *RedisStore) CheckAndStore(ctx context.Context, key string, txn *Transaction) (bool, *Transaction, error) {
	// Serialize transaction
	data, err := json.Marshal(txn)
	if err != nil {
		return false, nil, fmt.Errorf("failed to marshal transaction: %w", err)
	}
	
	// Try to set key with NX (only if not exists)
	set, err := s.client.SetNX(ctx, s.idempotencyKey(key), data, s.ttl).Result()
	if err != nil {
		return false, nil, fmt.Errorf("failed to check/store transaction: %w", err)
	}
	
	if !set {
		// Key already exists, retrieve existing transaction
		existing, err := s.Get(ctx, key)
		if err != nil {
			return false, nil, err
		}
		return true, existing, nil
	}
	
	// Successfully stored new transaction
	return false, txn, nil
}

// Get retrieves a transaction
func (s *RedisStore) Get(ctx context.Context, key string) (*Transaction, error) {
	data, err := s.client.Get(ctx, s.idempotencyKey(key)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}
	
	var txn Transaction
	if err := json.Unmarshal(data, &txn); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction: %w", err)
	}
	
	return &txn, nil
}

// UpdateState updates transaction state
func (s *RedisStore) UpdateState(ctx context.Context, key string, state TransactionStatus) error {
	txn, err := s.Get(ctx, key)
	if err != nil {
		return err
	}
	if txn == nil {
		return fmt.Errorf("transaction not found: %s", key)
	}
	
	txn.State = state
	txn.UpdatedAt = time.Now()
	
	data, err := json.Marshal(txn)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction: %w", err)
	}
	
	return s.client.Set(ctx, s.idempotencyKey(key), data, s.ttl).Err()
}

// IsDuplicate checks for duplicates using deduplication key
func (s *RedisStore) IsDuplicate(ctx context.Context, dedup *DeduplicationKey) (bool, error) {
	dedupKey := s.deduplicationKey(dedup)
	
	// Check if deduplication key exists
	exists, err := s.client.Exists(ctx, dedupKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check duplicate: %w", err)
	}
	
	if exists > 0 {
		return true, nil
	}
	
	// Store deduplication key
	// Use longer TTL for deduplication (e.g., 24 hours)
	dedupTTL := 24 * time.Hour
	if err := s.client.Set(ctx, dedupKey, "1", dedupTTL).Err(); err != nil {
		return false, fmt.Errorf("failed to store deduplication key: %w", err)
	}
	
	return false, nil
}

// StateMachine handles transaction state transitions
type StateMachine struct {
	store Store
}

// NewStateMachine creates a new state machine
func NewStateMachine(store Store) *StateMachine {
	return &StateMachine{store: store}
}

// Transition validates and performs a state transition
func (sm *StateMachine) Transition(ctx context.Context, key string, fromState, toState TransactionStatus) error {
	if !sm.isValidTransition(fromState, toState) {
		return fmt.Errorf("invalid state transition: %s -> %s", fromState, toState)
	}
	
	return sm.store.UpdateState(ctx, key, toState)
}

// isValidTransition checks if a state transition is valid
func (sm *StateMachine) isValidTransition(from, to TransactionStatus) bool {
	validTransitions := map[TransactionStatus][]TransactionStatus{
		StatusInit: {StatusSent, StatusFailed},
		StatusSent: {StatusAcked, StatusFailed},
		StatusAcked: {StatusApproved, StatusDeclined, StatusFailed},
		StatusApproved: {StatusReversed, StatusSettled, StatusFailed},
		StatusDeclined: {StatusFailed},
		StatusReversed: {StatusSettled},
	}
	
	allowed, exists := validTransitions[from]
	if !exists {
		return false
	}
	
	for _, state := range allowed {
		if state == to {
			return true
		}
	}
	
	return false
}

// Helper functions

func (s *RedisStore) idempotencyKey(key string) string {
	return fmt.Sprintf("idempotency:%s", key)
}

func (s *RedisStore) deduplicationKey(dedup *DeduplicationKey) string {
	return fmt.Sprintf("dedup:%s:%s:%s:%d:%s:%s",
		dedup.Network,
		dedup.STAN,
		dedup.RRN,
		dedup.Amount,
		dedup.TerminalID,
		dedup.MerchantID,
	)
}

// Close closes the Redis connection
func (s *RedisStore) Close() error {
	return s.client.Close()
}
