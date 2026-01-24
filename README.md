# OpenTX - Canonical Transaction Protocol & Gateway

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Protocol Buffers](https://img.shields.io/badge/Protocol_Buffers-3.21+-4285F4?style=flat&logo=google)](https://protobuf.dev)

A production-grade payment gateway that bridges legacy ISO 8583 networks and modern cloud-native payment systems with a canonical transaction protocol.

## Overview

OpenTX provides a **modernization layer** for card and payment networks, offering:

- **Canonical Transaction Model**: Protobuf-first schema modeling all ISO 8583 semantics
- **Multi-Network Support**: Pluggable adapters for Visa, Mastercard, NPCI/RuPay, and custom networks
- **Bidirectional Mapping**: Seamless conversion between ISO 8583 and canonical format
- **Message Security**: Encryption, signing, anti-replay protection independent of transport
- **Exactly-Once Semantics**: Idempotency and deduplication with state machine
- **Full Observability**: OpenTelemetry tracing, structured logging, Prometheus metrics
- **Event-Driven Architecture**: Kafka-based event streaming for downstream integration
- **Production-Ready**: Circuit breakers, rate limiting, health checks, and comprehensive testing

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     External Networks                            │
├─────────────┬──────────────┬─────────────┬──────────────────────┤
│ Visa Network│ MC Network   │ NPCI/RuPay  │ Bank Networks       │
└──────┬──────┴──────┬───────┴──────┬──────┴──────┬───────────────┘
       │ ISO 8583    │ ISO 8583     │ ISO 8583    │ ISO 8583
       ▼             ▼              ▼             ▼
┌─────────────────────────────────────────────────────────────────┐
│              ISO 8583 Parsing & Packing Layer                    │
│  • Variant-specific packagers  • EMV TLV parsing                │
└───────────────────────────┬─────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│           Canonical Transaction Protocol (Protobuf)              │
│  • Versioned Schema  • Strong Typing  • Validation              │
└───────────────────────────┬─────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│              Security & Idempotency Layer                        │
│  • Message Encryption/Signing  • Exactly-Once Processing        │
└───────────────────────────┬─────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│           Event Stream & API Layer                               │
│  gRPC │ REST │ Kafka │ WebSocket                                │
└─────────────────────────────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Go 1.21 or higher
- Docker & Docker Compose
- Protocol Buffers compiler (protoc)

### Installation

```bash
# Clone the repository
git clone https://github.com/krish567366/OpenTX.git
cd OpenTX

# Run setup script
./scripts/dev-setup.sh

# Start infrastructure services
./quickstart.sh

# Build and run the gateway
make build
./bin/gateway --config configs/gateway.yaml
```

### Using Docker

```bash
# Start all services
docker-compose -f docker/docker-compose.yml up -d

# View logs
docker-compose -f docker/docker-compose.yml logs -f gateway

# Stop all services
docker-compose -f docker/docker-compose.yml down
```

## Documentation

- [Architecture](docs/architecture.md) - System design and component overview
- [ISO 8583 Variants](docs/iso8583-variants.md) - Network-specific implementations
- [Security Model](docs/security.md) - Encryption, signing, and HSM integration
- [State Machine](docs/state-machine.md) - Transaction lifecycle management
- [EMV Parsing](docs/emv.md) - EMV TLV data handling
- [API Reference](docs/api.md) - gRPC and REST API documentation

## Testing

```bash
# Run unit tests
make test

# Run integration tests
make integration

# Run certification tests
make cert-test

# Run all tests with coverage
make test
go tool cover -html=coverage.out
```

## Key Components

### 1. Canonical Transaction Schema

Protobuf-first design with semantic field names:

```protobuf
message CanonicalTransaction {
  string schema_version = 1;
  string message_id = 2;
  string correlation_id = 3;
  MessageMetadata metadata = 4;
  
  oneof transaction_type {
    AuthorizationRequest auth_request = 10;
    AuthorizationResponse auth_response = 11;
    ReversalRequest reversal_request = 12;
    // ... more types
  }
  
  SecurityEnvelope security = 20;
  string idempotency_key = 21;
  TransactionState state = 22;
}
```

### 2. ISO 8583 Gateway

Multi-variant support with pluggable packagers:

```go
// Visa packager
visaPackager := packager.NewVisaPackager()
isoMsg, err := visaPackager.Unpack(rawBytes)

// Mastercard packager
mcPackager := packager.NewMastercardPackager()
isoMsg, err := mcPackager.Unpack(rawBytes)

// NPCI/RuPay packager
npciPackager := packager.NewNPCIPackager()
isoMsg, err := npciPackager.Unpack(rawBytes)
```

### 3. Bidirectional Mapping

```go
// ISO 8583 → Canonical
mapper := mapper.NewGenericMapper("VISA")
canonical, err := mapper.ToCanonical(isoMsg)

// Canonical → ISO 8583
isoMsg, err := mapper.FromCanonical(canonical)
```

### 4. Idempotency & State Machine

```go
// Check for duplicates
isDuplicate, existingTxn, err := store.CheckAndStore(ctx, key, txn)

// State transitions
stateMachine.Transition(ctx, key, 
    idempotency.StatusInit, 
    idempotency.StatusSent)
```

### 5. Event Publishing

```go
publisher := events.NewKafkaPublisher(brokers, topic, logger)

event := eventBuilder.BuildAuthRequestedEvent(
    messageID, stan, rrn, amount, currency,
    merchantID, merchantName, terminalID, cardLast4,
    metadata,
)

publisher.Publish(ctx, event)
```

## Observability

### Distributed Tracing (Jaeger)

Access at: http://localhost:16686

```go
ctx, span := tracer.StartSpan(ctx, "process_transaction",
    attribute.String("network", "VISA"),
    attribute.String("mti", "0200"),
)
defer span.End()
```

### Metrics (Prometheus)

Access at: http://localhost:9091

Metrics exposed:
- Transaction counters (by network, MTI, response code)
- Latency histograms
- Error rates
- System resource utilization

### Dashboards (Grafana)

Access at: http://localhost:3000 (admin/admin123)

Pre-configured dashboards for:
- Transaction volume and success rate
- Latency percentiles (p50, p95, p99)
- Error breakdown
- Network health

## Security

- **Message-level encryption** (AES-256-GCM)
- **Digital signatures** (RSA-PSS)
- **Anti-replay protection** (nonce + timestamp)
- **HSM integration** support
- **Key versioning** for rotation
- **TLS everywhere**

## Use Cases

1. **Payment Network Modernization**: Replace legacy ISO 8583 infrastructure
2. **Fintech Integration**: Enable cloud-native apps to connect to card networks
3. **Multi-Network Aggregation**: Single API for multiple payment networks
4. **Transaction Normalization**: Consistent data model across networks
5. **Fraud Detection**: Real-time event streaming for fraud systems
6. **Reconciliation**: Standardized events for settlement and recon
7. **Analytics**: Unified transaction data for business intelligence

## Development

```bash
# Install dependencies
make deps

# Generate protobuf code
make proto

# Build all binaries
make build

# Run linters
make lint

# Format code
make fmt

# Run specific test
go test -v ./pkg/iso8583/...
```

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- ISO 8583 specification and standards
- EMVCo for EMV specifications
- OpenTelemetry community
- Protocol Buffers team

## Support

- **Issues**: [GitHub Issues](https://github.com/krish567366/OpenTX/issues)
- **Discussions**: [GitHub Discussions](https://github.com/krish567366/OpenTX/discussions)
- **Email**: krishna@example.com

## Roadmap

- [ ] HSM integration (AWS CloudHSM, Azure Key Vault)
- [ ] Multi-region deployment support
- [ ] ML-based fraud detection
- [ ] Blockchain settlement layer
- [ ] WebAssembly support for edge deployment
- [ ] GraphQL API
- [ ] Real-time analytics dashboard
- [ ] A/B testing framework

---

**Built with ❤️ for the payment industry**

Star ⭐ this repository if you find it useful!
