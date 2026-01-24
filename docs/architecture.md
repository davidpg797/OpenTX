# Architecture Documentation

## System Overview

The Canonical Transaction Gateway is a high-performance, production-grade payment gateway that bridges legacy ISO 8583 networks with modern cloud-native payment systems.

## Key Components

### 1. Canonical Transaction Model (Protobuf)

The heart of the system is a versioned, strongly-typed canonical schema that models all ISO 8583 semantics:

- **Message Types**: Authorization, Reversal, Advice, Settlement, Network Management
- **Processing Codes**: Transaction type, account types
- **Identifiers**: STAN, RRN, Auth ID
- **Amounts**: Transaction, settlement, billing with currency
- **Card Data**: PAN/Token, expiry, service code
- **EMV Data**: TVR, TSI, AIP, cryptograms
- **Response Codes**: Categorized and normalized

### 2. ISO 8583 Gateway Layer

#### Parsing & Packing
- **Pluggable Packagers**: Support for multiple network variants (Visa, Mastercard, NPCI/RuPay)
- **Config-Driven**: Field specifications, encodings, lengths defined in configuration
- **Multi-Format**: Binary, ASCII, BCD encoding support
- **EMV TLV Parsing**: Full support for EMV data in DE55
- **Original Data Elements**: Reconstruction for reversals (DE90)

#### Network Variants
Each network has its own packager with specific field layouts:
- **Visa**: ASCII bitmap, specific field formats
- **Mastercard**: Binary bitmap, different field specifications
- **NPCI**: Custom header, RuPay-specific fields
- **Extensible**: Easy to add new variants

### 3. Bidirectional Mapping

The mapper converts between ISO 8583 and canonical format:

```
ISO 8583 → Canonical:
1. Parse raw bytes to ISO message
2. Extract fields using packager
3. Map to canonical structure
4. Validate and enrich
5. Return canonical transaction

Canonical → ISO 8583:
1. Start with canonical transaction
2. Determine target network variant
3. Map canonical fields to ISO fields
4. Pack using network packager
5. Return raw bytes
```

### 4. Transport Layer

Multiple transport protocols supported:

- **TCP/IP**: Legacy synchronous request-response for ISO 8583
- **gRPC**: Low-latency modern RPC for canonical API
- **HTTP/REST**: Stateless web API
- **Kafka**: Asynchronous event-driven messaging

Each transport is abstracted, allowing business logic to be independent of protocol.

### 5. Security Model

Message-level security independent of transport:

- **Encryption**: AES-256-GCM for payload encryption
- **Signing**: RSA-PSS for digital signatures
- **Key Management**: Key ID and version in message header
- **HSM Support**: Interface for hardware security modules
- **Anti-Replay**: Nonce + timestamp + sliding window validation

### 6. Idempotency & State Machine

Exactly-once semantics enforced:

```
State Machine:
INIT → SENT → ACKED → APPROVED/DECLINED → REVERSED → SETTLED

Deduplication Key:
- Network + STAN + RRN + Amount + Terminal + Merchant
- 24-hour TTL in Redis
- Fast duplicate detection
```

### 7. Observability

Built-in observability using OpenTelemetry:

#### Tracing
- Correlation ID injected at ingress
- Spans for each processing stage:
  - ISO parse
  - Canonical mapping
  - Security validation
  - Network send
  - Response processing

#### Logging
- Structured JSON logs
- Transaction-specific context
- Correlation and message IDs
- Field-level logging for debugging

#### Metrics (Prometheus)
- Transaction counters by network, MTI, response code
- Latency histograms per stage
- Error rates by type
- Reversal and timeout counters

### 8. Event Streaming

Every transaction emits normalized events to Kafka:

Events:
- `auth.requested`
- `auth.approved`
- `auth.declined`
- `reversal.sent`
- `reversal.completed`
- `settlement.posted`
- `transaction.failed`

Consumers:
- Fraud detection systems
- Reconciliation services
- Analytics platforms
- Ledger systems
- Audit logs

## Data Flow

### Authorization Request Flow

```
1. TCP ISO 8583 bytes arrive
   ↓
2. Parse using network-specific packager
   ↓
3. Convert to canonical format
   ↓
4. Check idempotency (duplicate detection)
   ↓
5. Validate message integrity
   ↓
6. Apply security (decrypt if needed)
   ↓
7. Execute business logic
   ↓
8. Route to target network
   ↓
9. Update state machine
   ↓
10. Emit event (auth.requested)
    ↓
11. Wait for response
    ↓
12. Map response to canonical
    ↓
13. Update state (APPROVED/DECLINED)
    ↓
14. Emit event (auth.approved/declined)
    ↓
15. Pack response to ISO 8583
    ↓
16. Return to caller
```

### Reversal Flow

```
1. Receive reversal request
   ↓
2. Parse DE90 (original data elements)
   ↓
3. Lookup original transaction
   ↓
4. Validate reversal eligibility
   ↓
5. Check reversal reason
   ↓
6. Route to network
   ↓
7. Update state to REVERSED
   ↓
8. Emit reversal.sent event
   ↓
9. Process response
   ↓
10. Emit reversal.completed event
```

## Scalability

### Horizontal Scaling
- Stateless gateway services
- Redis for distributed state
- Kafka for event distribution
- Load balancer for TCP connections

### Performance Optimizations
- Connection pooling
- Message batching
- Async processing where possible
- Efficient bitmap operations
- Zero-copy where applicable

### Reliability
- Circuit breakers for network calls
- Retry with exponential backoff
- Dead letter queues for failed messages
- Store-and-forward for offline scenarios
- Health checks and readiness probes

## Deployment

### Docker Compose (Development)
```bash
docker-compose up -d
```

### Kubernetes (Production)
- StatefulSet for gateway pods
- ConfigMaps for configuration
- Secrets for keys
- PersistentVolumes for state
- Horizontal Pod Autoscaler

## Monitoring

### Health Endpoints
- `/health`: Liveness probe
- `/ready`: Readiness probe
- `/metrics`: Prometheus metrics

### Dashboards
- Transaction volume and success rate
- Latency percentiles (p50, p95, p99)
- Error breakdown by type
- Network-specific metrics
- System resource utilization

## Security Considerations

1. **Network Isolation**: Gateway in DMZ, internal services in private network
2. **TLS Everywhere**: Encrypt all network communication
3. **Key Rotation**: Regular rotation of encryption keys
4. **HSM Integration**: Store sensitive keys in HSM
5. **Audit Logging**: All transactions logged for compliance
6. **Rate Limiting**: Prevent abuse and DDoS
7. **Input Validation**: Strict validation of all inputs

## Testing Strategy

1. **Unit Tests**: Individual component testing
2. **Integration Tests**: End-to-end flow testing
3. **Certification Tests**: Network conformance testing
4. **Load Tests**: Performance under stress
5. **Chaos Engineering**: Failure scenario testing

## Future Enhancements

1. **ML-based Fraud Detection**: Real-time scoring
2. **Dynamic Routing**: Intelligent network selection
3. **A/B Testing**: Gradual rollout of changes
4. **Multi-Region**: Global deployment with regional failover
5. **Blockchain Settlement**: DLT-based settlement layer
