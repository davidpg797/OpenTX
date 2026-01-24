# Canonical Transaction Protocol & Gateway

A production-grade payment gateway that bridges legacy ISO 8583 networks and modern cloud-native payment systems.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                     External Systems                             │
├─────────────┬──────────────┬─────────────┬──────────────────────┤
│ Visa Network│ MC Network   │ NPCI/RuPay  │ Bank Networks       │
└──────┬──────┴──────┬───────┴──────┬──────┴──────┬───────────────┘
       │             │              │             │
       │ ISO 8583    │ ISO 8583     │ ISO 8583    │ ISO 8583
       │             │              │             │
┌──────▼─────────────▼──────────────▼─────────────▼───────────────┐
│              ISO 8583 Transport Layer (TCP/MQ)                   │
│  - Session Management  - Timeouts  - Retries  - Circuit Breaker │
└──────────────────────────────┬───────────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────────┐
│              ISO 8583 Parsing & Packing Layer                    │
│  - Variant Packagers (Visa/MC/NPCI/Custom)                      │
│  - Binary/ASCII/BCD Handling                                     │
│  - EMV TLV Parsing (DE55)                                        │
│  - Original Data Element Reconstruction                          │
└──────────────────────────────┬───────────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────────┐
│              Canonical Transaction Model (Protobuf)              │
│  - Versioned Schema  - Strong Typing  - Validation              │
│  - Message Security  - Idempotency  - State Machine             │
└──────────────────────────────┬───────────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────────┐
│              Internal APIs & Event Streams                       │
│  gRPC  │  REST  │  Kafka Events  │  Message Queue               │
└──────────────────────────────┬───────────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────────┐
│              Downstream Services                                 │
│  Fraud  │  Ledger  │  Reconciliation  │  Analytics              │
└──────────────────────────────────────────────────────────────────┘
```

## Key Features

### 1. Canonical Transaction Schema
- Protobuf-first design with semantic field names
- Versioned schema with backward compatibility
- Strong typing for money, dates, codes
- Support for all ISO 8583 message types

### 2. ISO 8583 Gateway
- Multi-variant support (Visa, Mastercard, NPCI, Custom)
- Config-driven packagers
- Binary/ASCII/BCD handling
- EMV TLV parsing
- Reversals and original data element handling

### 3. Security
- Message-level encryption (JWE)
- Digital signatures (JWS)
- HSM integration
- Anti-replay protection
- Key versioning

### 4. Reliability
- Idempotent processing
- Exactly-once semantics
- Duplicate detection
- State machine-based transaction lifecycle
- Store-and-forward queues

### 5. Observability
- OpenTelemetry integration
- Distributed tracing
- Structured logging
- Rich metrics

### 6. Extensibility
- Pluggable variant packagers
- Rules engine for field mapping
- Validation framework
- Event-driven architecture

## Project Structure

```
.
├── proto/                      # Protocol Buffer definitions
│   ├── canonical/              # Canonical transaction model
│   ├── iso8583/                # ISO 8583 messages
│   └── events/                 # Event schemas
├── pkg/
│   ├── canonical/              # Canonical model implementation
│   ├── iso8583/                # ISO 8583 parsing & packing
│   │   ├── packager/           # Variant packagers
│   │   ├── fields/             # Field handlers
│   │   └── emv/                # EMV TLV parsing
│   ├── transport/              # Transport layer (TCP, HTTP, gRPC, MQ)
│   ├── security/               # Message security & HSM
│   ├── idempotency/            # Deduplication & state machine
│   ├── mapper/                 # Canonical ↔ ISO mapping
│   ├── rules/                  # Rules engine
│   ├── observability/          # Tracing, logging, metrics
│   └── events/                 # Event publishing
├── cmd/
│   ├── gateway/                # Gateway service
│   ├── simulator/              # ISO message simulator
│   └── certtest/               # Certification test harness
├── configs/                    # Configuration files
│   ├── variants/               # Network variant configs
│   ├── mappings/               # Field mapping rules
│   └── networks/               # Network profiles
├── test/
│   ├── vectors/                # Test message vectors
│   └── certification/          # Certification test cases
└── docs/                       # Documentation
```

## Technology Stack

- **Language**: Go (high performance, excellent concurrency)
- **Protocol**: Protocol Buffers (gRPC)
- **Storage**: Redis (deduplication), PostgreSQL (state)
- **Messaging**: Kafka (event streaming)
- **Observability**: OpenTelemetry, Prometheus, Grafana
- **Security**: HSM integration, JWT/JWE/JWS

## Getting Started

### Prerequisites
```bash
go 1.21+
protoc 3.21+
redis 7+
postgresql 15+
kafka 3+
```

### Build
```bash
make proto    # Generate protobuf code
make build    # Build all services
make test     # Run tests
```

### Run Gateway
```bash
./bin/gateway --config configs/gateway.yaml
```

### Run Simulator
```bash
./bin/simulator --config configs/simulator.yaml
```

## Configuration

See `configs/` directory for example configurations.

## Testing

```bash
make test              # Unit tests
make integration       # Integration tests
make cert-test         # Certification tests
```

## Documentation

- [Architecture](docs/architecture.md)
- [ISO 8583 Variants](docs/iso8583-variants.md)
- [Security Model](docs/security.md)
- [State Machine](docs/state-machine.md)
- [EMV Parsing](docs/emv.md)
- [Certification Guide](docs/certification.md)

## License

Proprietary
