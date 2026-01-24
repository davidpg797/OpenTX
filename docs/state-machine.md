# State Machine - Transaction Lifecycle Management

This document describes the state machine that manages the complete lifecycle of payment transactions in OpenTX.

## Overview

The state machine ensures transactions progress through well-defined states with controlled transitions, enabling:
- **Idempotency**: Duplicate requests are safely handled
- **Recovery**: Failed transactions can be retried or compensated
- **Auditability**: Complete transaction history is maintained
- **Consistency**: Distributed transaction coordination

## State Diagram

```
                    ┌──────────┐
                    │   INIT   │
                    └────┬─────┘
                         │
                    Send Request
                         │
                         ▼
                    ┌──────────┐
              ┌─────┤   SENT   ├─────┐
              │     └────┬─────┘     │
              │          │           │
           Timeout   Receive ACK  Network Error
              │          │           │
              ▼          ▼           ▼
         ┌────────┐ ┌────────┐ ┌─────────┐
         │TIMEOUT │ │ ACKED  │ │ FAILED  │
         └───┬────┘ └───┬────┘ └────┬────┘
             │          │            │
          Retry    Process Response  │
             │          │            │
             │          ▼            │
             │    ┌──────────┐      │
             │    │ APPROVED │      │
             │    └────┬─────┘      │
             │         │            │
             │    Post to GL        │
             │         │            │
             │         ▼            │
             │    ┌──────────┐     │
             │    │ SETTLED  │     │
             │    └──────────┘     │
             │                     │
             └──────────┬──────────┘
                        │
                   Send Reversal
                        │
                        ▼
                   ┌──────────┐
                   │ REVERSED │
                   └──────────┘
```

## States

### INIT (Initial State)
**Description**: Transaction has been created but not yet sent to the network.

**Entry Actions**:
- Generate unique message ID
- Create idempotency key
- Initialize transaction context
- Start transaction timer

**Valid Transitions**:
- → `SENT`: Request sent successfully
- → `FAILED`: Validation error or system failure

```go
const (
    StateInit TransactionState = "INIT"
)

type InitState struct {
    TransactionID   string
    IdempotencyKey  string
    CreatedAt       time.Time
    RequestPayload  []byte
}
```

### SENT (Request Sent)
**Description**: Authorization request has been sent to the acquiring network.

**Entry Actions**:
- Record send timestamp
- Start response timeout timer
- Update transaction state in Redis
- Emit `auth.requested` event

**Valid Transitions**:
- → `ACKED`: ACK received from network
- → `TIMEOUT`: No response within timeout window
- → `FAILED`: Network error or connection failure

```go
const (
    StateSent TransactionState = "SENT"
)

type SentState struct {
    SentAt          time.Time
    TimeoutAt       time.Time
    RetryCount      int
    NetworkEndpoint string
}
```

### ACKED (Acknowledged)
**Description**: Network has acknowledged receipt of the request.

**Entry Actions**:
- Record ACK timestamp
- Reset timeout timer
- Wait for final response

**Valid Transitions**:
- → `APPROVED`: Authorization approved
- → `DECLINED`: Authorization declined
- → `TIMEOUT`: Final response not received

```go
const (
    StateAcked TransactionState = "ACKED"
)

type AckedState struct {
    AckedAt     time.Time
    STAN        string
    RRN         string
}
```

### APPROVED (Authorization Approved)
**Description**: Transaction has been approved by the issuer.

**Entry Actions**:
- Record approval timestamp
- Store authorization code
- Update available balance (if tracked)
- Emit `auth.approved` event
- Schedule settlement

**Valid Transitions**:
- → `SETTLED`: Transaction posted to GL
- → `REVERSED`: Merchant or system reversal

```go
const (
    StateApproved TransactionState = "APPROVED"
)

type ApprovedState struct {
    ApprovedAt      time.Time
    AuthCode        string
    ResponseCode    string
    ApprovedAmount  int64
    BalanceInfo     []BalanceInfo
}
```

### DECLINED (Authorization Declined)
**Description**: Transaction has been declined by the issuer or network.

**Entry Actions**:
- Record decline timestamp
- Store decline reason
- Emit `auth.declined` event
- Release reserved funds

**Valid Transitions**:
- None (terminal state for normal flow)
- → `REVERSED`: For accounting purposes (rare)

```go
const (
    StateDeclined TransactionState = "DECLINED"
)

type DeclinedState struct {
    DeclinedAt      time.Time
    ResponseCode    string
    DeclineReason   string
    IssuerResponse  string
}
```

### REVERSED (Transaction Reversed)
**Description**: Transaction has been reversed due to merchant cancellation, timeout, or error.

**Entry Actions**:
- Record reversal timestamp
- Store reversal reason
- Send reversal message to network
- Emit `reversal.sent` event
- Restore original balance

**Valid Transitions**:
- None (terminal state)

```go
const (
    StateReversed TransactionState = "REVERSED"
)

type ReversedState struct {
    ReversedAt      time.Time
    ReversalReason  ReversalReason
    OriginalSTAN    string
    OriginalRRN     string
    ReversalSTAN    string
}

type ReversalReason string

const (
    ReversalTimeout         ReversalReason = "TIMEOUT"
    ReversalCustomer        ReversalReason = "CUSTOMER_CANCELLATION"
    ReversalMerchant        ReversalReason = "MERCHANT_CANCELLATION"
    ReversalSystemError     ReversalReason = "SYSTEM_ERROR"
    ReversalSuspectedFraud  ReversalReason = "SUSPECTED_FRAUD"
)
```

### SETTLED (Transaction Settled)
**Description**: Transaction has been posted to general ledger and included in settlement batch.

**Entry Actions**:
- Record settlement timestamp
- Update settlement batch
- Mark transaction as complete
- Emit `settlement.posted` event
- Archive transaction data

**Valid Transitions**:
- None (terminal state)

```go
const (
    StateSettled TransactionState = "SETTLED"
)

type SettledState struct {
    SettledAt       time.Time
    BatchNumber     string
    SettlementDate  time.Time
    SettledAmount   int64
    GLPostingID     string
}
```

### TIMEOUT (Response Timeout)
**Description**: Network did not respond within the configured timeout period.

**Entry Actions**:
- Record timeout timestamp
- Check retry policy
- Send reversal if applicable
- Emit `transaction.failed` event

**Valid Transitions**:
- → `SENT`: Retry the request
- → `REVERSED`: Send reversal after max retries
- → `FAILED`: Abandon transaction

```go
const (
    StateTimeout TransactionState = "TIMEOUT"
)

type TimeoutState struct {
    TimedOutAt      time.Time
    RetryCount      int
    MaxRetries      int
    NextRetryAt     time.Time
}
```

### FAILED (Transaction Failed)
**Description**: Transaction failed due to system error, validation error, or exceeded retries.

**Entry Actions**:
- Record failure timestamp
- Store error details
- Emit `transaction.failed` event
- Trigger alerting if needed

**Valid Transitions**:
- None (terminal state)

```go
const (
    StateFailed TransactionState = "FAILED"
)

type FailedState struct {
    FailedAt        time.Time
    ErrorCode       string
    ErrorMessage    string
    FailureReason   FailureReason
    StackTrace      string
}

type FailureReason string

const (
    FailureValidation   FailureReason = "VALIDATION_ERROR"
    FailureNetwork      FailureReason = "NETWORK_ERROR"
    FailureTimeout      FailureReason = "TIMEOUT"
    FailureSystem       FailureReason = "SYSTEM_ERROR"
    FailureSecurity     FailureReason = "SECURITY_ERROR"
)
```

## State Machine Implementation

### Core State Machine

```go
type StateMachine struct {
    currentState TransactionState
    context      *TransactionContext
    store        IdempotencyStore
    publisher    EventPublisher
    logger       *zap.Logger
    mu           sync.RWMutex
}

type TransactionContext struct {
    TransactionID   string
    IdempotencyKey  string
    Message         *iso8583.Message
    Canonical       *canonical.CanonicalTransaction
    StateData       map[TransactionState]interface{}
    Metadata        map[string]string
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

func NewStateMachine(ctx *TransactionContext, store IdempotencyStore) *StateMachine {
    return &StateMachine{
        currentState: StateInit,
        context:      ctx,
        store:        store,
        logger:       zap.L(),
    }
}
```

### State Transitions

```go
func (sm *StateMachine) Transition(event TransactionEvent) error {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    
    // Validate transition
    if !sm.isValidTransition(sm.currentState, event.TargetState) {
        return fmt.Errorf("invalid transition from %s to %s", 
            sm.currentState, event.TargetState)
    }
    
    // Execute exit actions for current state
    if err := sm.executeExitActions(sm.currentState); err != nil {
        return fmt.Errorf("exit actions failed: %w", err)
    }
    
    // Store previous state
    previousState := sm.currentState
    
    // Transition to new state
    sm.currentState = event.TargetState
    sm.context.UpdatedAt = time.Now()
    
    // Store state data
    sm.context.StateData[event.TargetState] = event.StateData
    
    // Persist state change
    if err := sm.store.UpdateState(sm.context.IdempotencyKey, 
        event.TargetState, event.StateData); err != nil {
        return fmt.Errorf("failed to persist state: %w", err)
    }
    
    // Execute entry actions for new state
    if err := sm.executeEntryActions(event.TargetState); err != nil {
        sm.logger.Error("entry actions failed", 
            zap.String("state", string(event.TargetState)),
            zap.Error(err))
    }
    
    // Emit state change event
    sm.emitStateChangeEvent(previousState, event.TargetState)
    
    sm.logger.Info("state transition",
        zap.String("from", string(previousState)),
        zap.String("to", string(event.TargetState)),
        zap.String("transaction_id", sm.context.TransactionID))
    
    return nil
}
```

### Transition Validation

```go
var validTransitions = map[TransactionState][]TransactionState{
    StateInit: {
        StateSent,
        StateFailed,
    },
    StateSent: {
        StateAcked,
        StateTimeout,
        StateFailed,
    },
    StateAcked: {
        StateApproved,
        StateDeclined,
        StateTimeout,
    },
    StateApproved: {
        StateSettled,
        StateReversed,
    },
    StateDeclined: {
        StateReversed, // Rare, for accounting
    },
    StateTimeout: {
        StateSent,     // Retry
        StateReversed, // Give up
        StateFailed,
    },
    StateReversed: {}, // Terminal
    StateSettled:  {}, // Terminal
    StateFailed:   {}, // Terminal
}

func (sm *StateMachine) isValidTransition(from, to TransactionState) bool {
    validTargets, exists := validTransitions[from]
    if !exists {
        return false
    }
    
    for _, validTarget := range validTargets {
        if validTarget == to {
            return true
        }
    }
    
    return false
}
```

### Entry and Exit Actions

```go
func (sm *StateMachine) executeEntryActions(state TransactionState) error {
    switch state {
    case StateInit:
        return sm.onEnterInit()
    case StateSent:
        return sm.onEnterSent()
    case StateAcked:
        return sm.onEnterAcked()
    case StateApproved:
        return sm.onEnterApproved()
    case StateDeclined:
        return sm.onEnterDeclined()
    case StateReversed:
        return sm.onEnterReversed()
    case StateSettled:
        return sm.onEnterSettled()
    case StateTimeout:
        return sm.onEnterTimeout()
    case StateFailed:
        return sm.onEnterFailed()
    }
    return nil
}

func (sm *StateMachine) onEnterApproved() error {
    stateData := sm.context.StateData[StateApproved].(*ApprovedState)
    
    // Emit approval event
    event := events.AuthorizationApprovedEvent{
        ApprovedAmount: &canonical.Money{
            Amount:   stateData.ApprovedAmount,
            Currency: sm.context.Canonical.Money.Currency,
        },
        AuthId:           stateData.AuthCode,
        ResponseCode:     stateData.ResponseCode,
        ApprovalDatetime: timestamppb.New(stateData.ApprovedAt),
        Balances:         stateData.BalanceInfo,
    }
    
    return sm.publisher.PublishAuthApproved(sm.context, &event)
}
```

## Idempotency Handling

### Duplicate Detection

```go
func (sm *StateMachine) HandleRequest(msg *iso8583.Message) error {
    // Generate idempotency key
    idempotencyKey := generateIdempotencyKey(msg)
    
    // Check if request already processed
    existingState, err := sm.store.Get(idempotencyKey)
    if err == nil {
        // Request already exists
        return sm.handleDuplicate(existingState)
    }
    
    // New request - proceed normally
    sm.context.IdempotencyKey = idempotencyKey
    return sm.processNewRequest(msg)
}

func (sm *StateMachine) handleDuplicate(state *TransactionState) error {
    switch *state {
    case StateApproved, StateDeclined, StateSettled:
        // Return cached response
        return sm.returnCachedResponse()
    
    case StateSent, StateAcked:
        // Still in progress - return processing status
        return ErrTransactionInProgress
    
    case StateTimeout, StateFailed:
        // Previous attempt failed - allow retry
        return sm.retryTransaction()
    
    default:
        return fmt.Errorf("unexpected state: %s", *state)
    }
}
```

### Idempotency Key Generation

```go
func generateIdempotencyKey(msg *iso8583.Message) string {
    // Combine multiple fields for uniqueness
    components := []string{
        msg.GetField(11).Value, // STAN
        msg.GetField(7).Value,  // Transmission DateTime
        msg.GetField(2).Value,  // PAN (last 4 digits)
        msg.GetField(4).Value,  // Amount
    }
    
    data := strings.Join(components, "|")
    hash := sha256.Sum256([]byte(data))
    return hex.EncodeToString(hash[:])
}
```

## Timeout and Retry Logic

### Configurable Timeouts

```yaml
timeouts:
  network_response: 30s
  acknowledgment: 5s
  final_response: 25s
  reversal: 20s

retry:
  max_attempts: 3
  initial_backoff: 1s
  max_backoff: 10s
  backoff_multiplier: 2.0
```

### Retry Strategy

```go
type RetryPolicy struct {
    MaxAttempts        int
    InitialBackoff     time.Duration
    MaxBackoff         time.Duration
    BackoffMultiplier  float64
}

func (sm *StateMachine) retryWithBackoff() error {
    timeoutState := sm.context.StateData[StateTimeout].(*TimeoutState)
    
    if timeoutState.RetryCount >= timeoutState.MaxRetries {
        // Max retries exceeded - send reversal
        return sm.Transition(TransactionEvent{
            TargetState: StateReversed,
            StateData: &ReversedState{
                ReversedAt:     time.Now(),
                ReversalReason: ReversalTimeout,
                OriginalSTAN:   sm.context.Message.GetField(11).Value,
            },
        })
    }
    
    // Calculate backoff
    backoff := sm.calculateBackoff(timeoutState.RetryCount)
    time.Sleep(backoff)
    
    // Retry request
    timeoutState.RetryCount++
    timeoutState.NextRetryAt = time.Now().Add(backoff)
    
    return sm.Transition(TransactionEvent{
        TargetState: StateSent,
        StateData: &SentState{
            SentAt:     time.Now(),
            RetryCount: timeoutState.RetryCount,
        },
    })
}
```

## Persistence

### State Storage Schema

```sql
CREATE TABLE transaction_states (
    idempotency_key VARCHAR(64) PRIMARY KEY,
    transaction_id VARCHAR(36) NOT NULL,
    current_state VARCHAR(20) NOT NULL,
    state_data JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    INDEX idx_transaction_id (transaction_id),
    INDEX idx_expires_at (expires_at)
);
```

### Redis Cache Structure

```
Key: txn:state:{idempotency_key}
Type: Hash
TTL: 24 hours

Fields:
- state: "APPROVED"
- transaction_id: "550e8400-e29b-41d4-a716-446655440000"
- state_data: "{\"approved_at\":\"2024-01-24T12:00:00Z\",\"auth_code\":\"ABC123\"}"
- created_at: "2024-01-24T11:59:00Z"
- updated_at: "2024-01-24T12:00:00Z"
```

## Observability

### State Metrics

```go
var (
    stateTransitionCounter = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "transaction_state_transitions_total",
            Help: "Total number of state transitions",
        },
        []string{"from_state", "to_state"},
    )
    
    stateDurationHistogram = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "transaction_state_duration_seconds",
            Help:    "Time spent in each state",
            Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
        },
        []string{"state"},
    )
    
    currentStateGauge = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "transactions_by_state",
            Help: "Current number of transactions in each state",
        },
        []string{"state"},
    )
)
```

## Testing

### State Machine Tests

```go
func TestStateMachine_HappyPath(t *testing.T) {
    sm := setupStateMachine(t)
    
    // INIT -> SENT
    err := sm.Transition(TransactionEvent{
        TargetState: StateSent,
        StateData:   &SentState{SentAt: time.Now()},
    })
    assert.NoError(t, err)
    assert.Equal(t, StateSent, sm.GetState())
    
    // SENT -> ACKED
    err = sm.Transition(TransactionEvent{
        TargetState: StateAcked,
        StateData:   &AckedState{AckedAt: time.Now()},
    })
    assert.NoError(t, err)
    
    // ACKED -> APPROVED
    err = sm.Transition(TransactionEvent{
        TargetState: StateApproved,
        StateData:   &ApprovedState{
            ApprovedAt:     time.Now(),
            AuthCode:       "123456",
            ApprovedAmount: 10000,
        },
    })
    assert.NoError(t, err)
    
    // APPROVED -> SETTLED
    err = sm.Transition(TransactionEvent{
        TargetState: StateSettled,
        StateData:   &SettledState{SettledAt: time.Now()},
    })
    assert.NoError(t, err)
    assert.Equal(t, StateSettled, sm.GetState())
}
```

## References

- [Finite State Machine Pattern](https://en.wikipedia.org/wiki/Finite-state_machine)
- [Transaction Processing: Concepts and Techniques](https://www.amazon.com/Transaction-Processing-Concepts-Techniques-Management/dp/1558601902)
- [Designing Data-Intensive Applications](https://dataintensive.net/)
