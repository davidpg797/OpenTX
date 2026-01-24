# API Reference - gRPC and REST API Documentation

This document provides comprehensive API documentation for OpenTX gateway services, including gRPC and REST endpoints.

## Overview

OpenTX provides two API interfaces:
1. **gRPC API**: High-performance binary protocol for service-to-service communication
2. **REST API**: HTTP/JSON interface for web and mobile applications

## gRPC API

### Service Definition

```protobuf
syntax = "proto3";

package opentx.gateway.v1;

import "canonical/transaction.proto";
import "google/protobuf/timestamp.proto";

service GatewayService {
  // Process authorization request
  rpc ProcessAuthorization(AuthorizationRequest) returns (AuthorizationResponse);
  
  // Process financial transaction
  rpc ProcessFinancial(FinancialRequest) returns (FinancialResponse);
  
  // Send reversal
  rpc SendReversal(ReversalRequest) returns (ReversalResponse);
  
  // Query transaction status
  rpc GetTransactionStatus(TransactionStatusRequest) returns (TransactionStatusResponse);
  
  // Batch operations
  rpc ProcessBatch(BatchRequest) returns (stream BatchResponse);
  
  // Health check
  rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
}
```

### Authorization Request

#### Request

```protobuf
message AuthorizationRequest {
  // Transaction identifiers
  string message_id = 1;
  string idempotency_key = 2;
  
  // Card information
  CardData card_data = 3;
  
  // Transaction details
  Money amount = 4;
  string currency_code = 5;
  
  // Merchant information
  MerchantData merchant = 6;
  
  // Terminal information
  TerminalData terminal = 7;
  
  // Transaction metadata
  google.protobuf.Timestamp transaction_datetime = 8;
  TransactionType transaction_type = 9;
  POSEntryMode pos_entry_mode = 10;
  
  // Optional EMV data
  EMVData emv_data = 11;
  
  // Additional data
  map<string, string> additional_data = 20;
}

message CardData {
  string pan = 1;                          // Primary Account Number
  string expiration_date = 2;              // YYMM format
  string cvv = 3;                          // Optional
  CardBrand brand = 4;
  string token = 5;                        // If tokenized
}

message MerchantData {
  string merchant_id = 1;
  string merchant_name = 2;
  string merchant_category_code = 3;
  string merchant_city = 4;
  string merchant_country = 5;
}

message TerminalData {
  string terminal_id = 1;
  string terminal_name = 2;
  TerminalType terminal_type = 3;
  TerminalCapabilities capabilities = 4;
}
```

#### Response

```protobuf
message AuthorizationResponse {
  // Status
  ResponseStatus status = 1;
  string response_code = 2;
  string response_message = 3;
  
  // Transaction identifiers
  string message_id = 4;
  string stan = 5;
  string rrn = 6;
  string auth_id = 7;
  
  // Approval details (if approved)
  Money approved_amount = 8;
  google.protobuf.Timestamp approval_datetime = 9;
  
  // Balance information (optional)
  repeated BalanceInfo balances = 10;
  
  // Issuer response
  IssuerResponse issuer_response = 11;
  
  // EMV response data (if chip transaction)
  bytes emv_response_data = 12;
  
  // Processing metadata
  google.protobuf.Timestamp processing_datetime = 20;
  int64 processing_duration_ms = 21;
}

enum ResponseStatus {
  STATUS_UNKNOWN = 0;
  STATUS_APPROVED = 1;
  STATUS_DECLINED = 2;
  STATUS_PROCESSING = 3;
  STATUS_ERROR = 4;
}

message IssuerResponse {
  string issuer_name = 1;
  string issuer_country = 2;
  string action_code = 3;
  map<string, string> additional_data = 10;
}
```

#### Example gRPC Call (Go)

```go
package main

import (
    "context"
    "log"
    "time"
    
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    pb "github.com/krish567366/OpenTX/pkg/proto/gateway"
    "google.golang.org/protobuf/types/known/timestamppb"
)

func main() {
    // Connect to gateway
    conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
    
    client := pb.NewGatewayServiceClient(conn)
    
    // Create authorization request
    req := &pb.AuthorizationRequest{
        MessageId:       "msg-123456",
        IdempotencyKey:  "idempotency-abc123",
        CardData: &pb.CardData{
            Pan:            "4111111111111111",
            ExpirationDate: "2512",
            Brand:          pb.CardBrand_CARD_BRAND_VISA,
        },
        Amount: &pb.Money{
            Amount:   10000, // $100.00
            Currency: "USD",
        },
        CurrencyCode: "840",
        Merchant: &pb.MerchantData{
            MerchantId:   "MERCH123",
            MerchantName: "Test Merchant",
            MerchantCategoryCode: "5411",
        },
        Terminal: &pb.TerminalData{
            TerminalId:   "TERM001",
            TerminalType: pb.TerminalType_TERMINAL_TYPE_POS,
        },
        TransactionDatetime: timestamppb.New(time.Now()),
        TransactionType:     pb.TransactionType_TRANSACTION_TYPE_PURCHASE,
        PosEntryMode:        pb.POSEntryMode_POS_ENTRY_MODE_CHIP,
    }
    
    // Call authorization
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    resp, err := client.ProcessAuthorization(ctx, req)
    if err != nil {
        log.Fatalf("Authorization failed: %v", err)
    }
    
    // Process response
    log.Printf("Response: %s - %s", resp.ResponseCode, resp.ResponseMessage)
    if resp.Status == pb.ResponseStatus_STATUS_APPROVED {
        log.Printf("Approved! Auth ID: %s, Amount: %d", resp.AuthId, resp.ApprovedAmount.Amount)
    }
}
```

### Financial Transaction Request

```protobuf
message FinancialRequest {
  string message_id = 1;
  string idempotency_key = 2;
  
  // Original authorization (if capturing)
  string original_auth_id = 3;
  string original_stan = 4;
  string original_rrn = 5;
  
  // Transaction details
  CardData card_data = 6;
  Money amount = 7;
  MerchantData merchant = 8;
  TerminalData terminal = 9;
  
  FinancialTransactionType transaction_type = 10;
  google.protobuf.Timestamp transaction_datetime = 11;
}

enum FinancialTransactionType {
  FINANCIAL_TYPE_UNKNOWN = 0;
  FINANCIAL_TYPE_SALE = 1;
  FINANCIAL_TYPE_REFUND = 2;
  FINANCIAL_TYPE_CASH_ADVANCE = 3;
  FINANCIAL_TYPE_CAPTURE = 4;
}

message FinancialResponse {
  ResponseStatus status = 1;
  string response_code = 2;
  string message_id = 3;
  string stan = 4;
  string rrn = 5;
  Money processed_amount = 6;
  google.protobuf.Timestamp processing_datetime = 7;
}
```

### Reversal Request

```protobuf
message ReversalRequest {
  string message_id = 1;
  
  // Original transaction identifiers
  string original_message_id = 2;
  string original_stan = 3;
  string original_rrn = 4;
  string original_auth_id = 5;
  
  // Reversal reason
  ReversalReason reason = 6;
  string reason_description = 7;
  
  // Amount to reverse (may be partial)
  Money reversal_amount = 8;
  
  // Original transaction details
  CardData card_data = 9;
  MerchantData merchant = 10;
  TerminalData terminal = 11;
  
  google.protobuf.Timestamp reversal_datetime = 12;
}

enum ReversalReason {
  REVERSAL_REASON_UNKNOWN = 0;
  REVERSAL_REASON_TIMEOUT = 1;
  REVERSAL_REASON_CUSTOMER_CANCELLATION = 2;
  REVERSAL_REASON_MERCHANT_CANCELLATION = 3;
  REVERSAL_REASON_SYSTEM_ERROR = 4;
  REVERSAL_REASON_SUSPECTED_FRAUD = 5;
}

message ReversalResponse {
  ResponseStatus status = 1;
  string response_code = 2;
  string message_id = 3;
  bool reversal_accepted = 4;
  google.protobuf.Timestamp processing_datetime = 5;
}
```

### Transaction Status Query

```protobuf
message TransactionStatusRequest {
  oneof identifier {
    string message_id = 1;
    string idempotency_key = 2;
    string stan = 3;
    string rrn = 4;
    string auth_id = 5;
  }
}

message TransactionStatusResponse {
  string transaction_id = 1;
  TransactionState current_state = 2;
  
  // Transaction details
  Money amount = 3;
  CardData card_data = 4;
  MerchantData merchant = 5;
  
  // Timestamps
  google.protobuf.Timestamp created_at = 6;
  google.protobuf.Timestamp updated_at = 7;
  
  // Result (if completed)
  ResponseStatus final_status = 8;
  string response_code = 9;
  string auth_id = 10;
  
  // State history
  repeated StateTransition state_history = 11;
}

enum TransactionState {
  STATE_UNKNOWN = 0;
  STATE_INIT = 1;
  STATE_SENT = 2;
  STATE_ACKED = 3;
  STATE_APPROVED = 4;
  STATE_DECLINED = 5;
  STATE_REVERSED = 6;
  STATE_SETTLED = 7;
  STATE_TIMEOUT = 8;
  STATE_FAILED = 9;
}

message StateTransition {
  TransactionState from_state = 1;
  TransactionState to_state = 2;
  google.protobuf.Timestamp timestamp = 3;
  string reason = 4;
}
```

## REST API

### Base URL
```
Production: https://api.opentx.io/v1
Sandbox: https://sandbox-api.opentx.io/v1
```

### Authentication

All API requests require an API key passed in the Authorization header:

```http
Authorization: Bearer YOUR_API_KEY
```

### Common Headers

```http
Content-Type: application/json
X-Request-ID: unique-request-id
X-Idempotency-Key: unique-idempotency-key
```

### Process Authorization

#### Request

```http
POST /v1/authorizations
Content-Type: application/json
Authorization: Bearer YOUR_API_KEY
X-Idempotency-Key: idempotency-abc123

{
  "message_id": "msg-123456",
  "card": {
    "pan": "4111111111111111",
    "expiration_date": "2512",
    "cvv": "123",
    "brand": "VISA"
  },
  "amount": {
    "value": 10000,
    "currency": "USD"
  },
  "merchant": {
    "merchant_id": "MERCH123",
    "merchant_name": "Test Merchant",
    "merchant_category_code": "5411",
    "city": "San Francisco",
    "country": "USA"
  },
  "terminal": {
    "terminal_id": "TERM001",
    "terminal_type": "POS"
  },
  "transaction_type": "PURCHASE",
  "pos_entry_mode": "CHIP",
  "emv_data": "9F26089876543210...",
  "metadata": {
    "customer_id": "CUST123",
    "order_id": "ORDER456"
  }
}
```

#### Response (Success - 200 OK)

```json
{
  "status": "APPROVED",
  "response_code": "00",
  "response_message": "Approved",
  "message_id": "msg-123456",
  "stan": "000001",
  "rrn": "123456789012",
  "auth_id": "ABC123",
  "approved_amount": {
    "value": 10000,
    "currency": "USD"
  },
  "approval_datetime": "2024-01-24T12:00:00Z",
  "balances": [
    {
      "account_type": "CHECKING",
      "amount": {
        "value": 50000,
        "currency": "USD"
      },
      "balance_type": "AVAILABLE"
    }
  ],
  "issuer_response": {
    "issuer_name": "Example Bank",
    "issuer_country": "USA",
    "action_code": "000"
  },
  "emv_response_data": "8A02303091...",
  "processing_datetime": "2024-01-24T12:00:00.123Z",
  "processing_duration_ms": 123
}
```

#### Response (Declined - 200 OK)

```json
{
  "status": "DECLINED",
  "response_code": "05",
  "response_message": "Do not honor",
  "message_id": "msg-123456",
  "stan": "000001",
  "rrn": "123456789012",
  "processing_datetime": "2024-01-24T12:00:00.123Z"
}
```

#### Error Response (400 Bad Request)

```json
{
  "error": {
    "code": "INVALID_REQUEST",
    "message": "Invalid card number format",
    "details": [
      {
        "field": "card.pan",
        "issue": "Must be 13-19 digits"
      }
    ]
  }
}
```

### Process Financial Transaction

```http
POST /v1/financial
Content-Type: application/json
Authorization: Bearer YOUR_API_KEY

{
  "message_id": "msg-789012",
  "transaction_type": "SALE",
  "card": {
    "pan": "4111111111111111",
    "expiration_date": "2512"
  },
  "amount": {
    "value": 5000,
    "currency": "USD"
  },
  "merchant": {
    "merchant_id": "MERCH123",
    "merchant_name": "Test Merchant"
  },
  "terminal": {
    "terminal_id": "TERM001"
  }
}
```

### Send Reversal

```http
POST /v1/reversals
Content-Type: application/json
Authorization: Bearer YOUR_API_KEY

{
  "message_id": "msg-reversal-001",
  "original_transaction": {
    "message_id": "msg-123456",
    "stan": "000001",
    "rrn": "123456789012",
    "auth_id": "ABC123"
  },
  "reversal_reason": "TIMEOUT",
  "reversal_reason_description": "Network timeout occurred",
  "reversal_amount": {
    "value": 10000,
    "currency": "USD"
  }
}
```

### Query Transaction Status

```http
GET /v1/transactions/{transaction_id}
Authorization: Bearer YOUR_API_KEY
```

Or query by different identifier:

```http
GET /v1/transactions?stan=000001
GET /v1/transactions?rrn=123456789012
GET /v1/transactions?auth_id=ABC123
```

#### Response

```json
{
  "transaction_id": "550e8400-e29b-41d4-a716-446655440000",
  "current_state": "APPROVED",
  "amount": {
    "value": 10000,
    "currency": "USD"
  },
  "card": {
    "last4": "1111",
    "brand": "VISA"
  },
  "merchant": {
    "merchant_id": "MERCH123",
    "merchant_name": "Test Merchant"
  },
  "created_at": "2024-01-24T12:00:00Z",
  "updated_at": "2024-01-24T12:00:01Z",
  "final_status": "APPROVED",
  "response_code": "00",
  "auth_id": "ABC123",
  "state_history": [
    {
      "from_state": "INIT",
      "to_state": "SENT",
      "timestamp": "2024-01-24T12:00:00.100Z"
    },
    {
      "from_state": "SENT",
      "to_state": "ACKED",
      "timestamp": "2024-01-24T12:00:00.500Z"
    },
    {
      "from_state": "ACKED",
      "to_state": "APPROVED",
      "timestamp": "2024-01-24T12:00:01.000Z"
    }
  ]
}
```

### Health Check

```http
GET /v1/health
```

#### Response

```json
{
  "status": "healthy",
  "version": "1.0.0",
  "timestamp": "2024-01-24T12:00:00Z",
  "components": {
    "database": {
      "status": "healthy",
      "latency_ms": 2
    },
    "redis": {
      "status": "healthy",
      "latency_ms": 1
    },
    "kafka": {
      "status": "healthy",
      "latency_ms": 5
    }
  }
}
```

## Error Codes

### HTTP Status Codes

| Code | Description |
|------|-------------|
| 200 | OK - Request successful |
| 201 | Created - Resource created |
| 400 | Bad Request - Invalid request format |
| 401 | Unauthorized - Invalid or missing API key |
| 403 | Forbidden - Insufficient permissions |
| 404 | Not Found - Resource not found |
| 409 | Conflict - Duplicate request |
| 422 | Unprocessable Entity - Validation error |
| 429 | Too Many Requests - Rate limit exceeded |
| 500 | Internal Server Error |
| 502 | Bad Gateway - Upstream service error |
| 503 | Service Unavailable |
| 504 | Gateway Timeout |

### Application Error Codes

| Code | Description |
|------|-------------|
| `INVALID_REQUEST` | Request validation failed |
| `INVALID_CARD` | Invalid card data |
| `INVALID_AMOUNT` | Invalid transaction amount |
| `DUPLICATE_REQUEST` | Idempotency key already used |
| `TRANSACTION_NOT_FOUND` | Transaction not found |
| `NETWORK_ERROR` | Network communication error |
| `TIMEOUT` | Request timeout |
| `SYSTEM_ERROR` | Internal system error |
| `CONFIGURATION_ERROR` | Configuration issue |

## Rate Limiting

API requests are rate limited:
- **Production**: 1000 requests/minute per API key
- **Sandbox**: 100 requests/minute per API key

Rate limit headers:
```http
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1706097600
```

## Idempotency

Use the `X-Idempotency-Key` header to ensure requests are processed exactly once. The idempotency key should be unique per transaction and remain the same for retries.

```http
X-Idempotency-Key: idempotency-abc123
```

Idempotent requests are cached for 24 hours.

## Webhooks

Configure webhooks to receive real-time transaction events:

### Events

- `authorization.requested`
- `authorization.approved`
- `authorization.declined`
- `reversal.sent`
- `reversal.completed`
- `settlement.posted`
- `transaction.failed`

### Webhook Payload

```json
{
  "event_id": "evt_123456",
  "event_type": "authorization.approved",
  "event_timestamp": "2024-01-24T12:00:00Z",
  "data": {
    "transaction_id": "550e8400-e29b-41d4-a716-446655440000",
    "message_id": "msg-123456",
    "auth_id": "ABC123",
    "amount": {
      "value": 10000,
      "currency": "USD"
    },
    "merchant_id": "MERCH123"
  }
}
```

### Webhook Signature

Webhooks are signed with HMAC-SHA256:

```
Signature = HMAC-SHA256(webhook_secret, event_payload)
```

Verify signature from header:
```http
X-Webhook-Signature: sha256=abcdef123456...
```

## SDKs

Official SDKs available for:
- **Go**: `go get github.com/krish567366/OpenTX/sdk/go`
- **Python**: `pip install opentx-sdk`
- **Node.js**: `npm install opentx-sdk`
- **Java**: Maven/Gradle support
- **C#**: NuGet package

## Testing

### Sandbox Environment

Use sandbox for testing:
```
Endpoint: https://sandbox-api.opentx.io/v1
```

### Test Cards

| Card Number | Brand | Expected Result |
|-------------|-------|-----------------|
| 4111111111111111 | Visa | Approved |
| 5555555555554444 | Mastercard | Approved |
| 4000000000000002 | Visa | Declined |
| 5555555555550002 | Mastercard | Declined |
| 4000000000000119 | Visa | Timeout |

### Test Amounts

| Amount | Result |
|--------|--------|
| $1.00 - $100.00 | Approved |
| $100.01 - $200.00 | Declined |
| $200.01+ | Timeout |

## Support

- **Documentation**: https://docs.opentx.io
- **API Status**: https://status.opentx.io
- **Support Email**: support@opentx.io
- **GitHub**: https://github.com/krish567366/OpenTX

## Versioning

API versioning is done via URL path:
```
/v1/authorizations  (current)
/v2/authorizations  (future)
```

Major versions remain supported for 12 months after deprecation notice.
