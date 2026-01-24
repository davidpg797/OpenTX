# ISO 8583 Variants

This document describes the network-specific implementations of ISO 8583 message formats supported by OpenTX.

## Overview

OpenTX supports multiple ISO 8583 variants to enable connectivity with different payment networks. Each variant has specific field specifications, encoding rules, and message formats.

## Supported Variants

### 1. Visa (VisaNet)

**Network Code:** `VISA`

#### Message Type Indicators (MTI)
- `0100` - Authorization Request
- `0110` - Authorization Response
- `0200` - Financial Transaction Request
- `0210` - Financial Transaction Response
- `0400` - Reversal Request
- `0410` - Reversal Response
- `0800` - Network Management Request
- `0810` - Network Management Response

#### Key Field Specifications

| Field | Name | Type | Length | Encoding |
|-------|------|------|--------|----------|
| 2 | Primary Account Number (PAN) | LLVAR | 19 | ASCII |
| 3 | Processing Code | Fixed | 6 | ASCII |
| 4 | Transaction Amount | Fixed | 12 | ASCII |
| 7 | Transmission Date/Time | Fixed | 10 | MMDDhhmmss |
| 11 | STAN | Fixed | 6 | ASCII |
| 12 | Local Transaction Time | Fixed | 6 | hhmmss |
| 13 | Local Transaction Date | Fixed | 4 | MMDD |
| 22 | POS Entry Mode | Fixed | 3 | ASCII |
| 32 | Acquiring Institution ID | LLVAR | 11 | ASCII |
| 37 | Retrieval Reference Number | Fixed | 12 | ASCII |
| 38 | Authorization ID | Fixed | 6 | ASCII |
| 39 | Response Code | Fixed | 2 | ASCII |
| 41 | Terminal ID | Fixed | 8 | ASCII |
| 42 | Merchant ID | Fixed | 15 | ASCII |
| 43 | Card Acceptor Name/Location | Fixed | 40 | ASCII |
| 48 | Additional Data | LLLVAR | 999 | ASCII |
| 49 | Transaction Currency Code | Fixed | 3 | ASCII |
| 55 | EMV Data | LLLVAR | 999 | Binary |
| 90 | Original Data Elements | Fixed | 42 | ASCII |

#### Visa-Specific Features
- **BASE I Specification**: MTI format 0xxx (version 0)
- **PAN Encryption**: Optional field 52 for encrypted PIN
- **CVV2/CVC2**: Transmitted in field 48
- **3D Secure**: Authentication data in field 48
- **Token Support**: Token PAN in field 2, Token Assurance Level in field 48

#### Example Message
```
0100B220000000000000000016123456789012345060000000000100007041523451234560704235900000123456789012311MERCHANT NAME   CITY    USABCD
```

### 2. Mastercard (Banknet)

**Network Code:** `MASTERCARD`

#### Message Type Indicators (MTI)
- `0100` - Authorization Request
- `0110` - Authorization Response  
- `0200` - Financial Request
- `0210` - Financial Response
- `0400` - Reversal Request
- `0410` - Reversal Response
- `0420` - Reversal Advice
- `0430` - Reversal Advice Response

#### Key Field Specifications

| Field | Name | Type | Length | Encoding |
|-------|------|------|--------|----------|
| 2 | PAN | LLVAR | 19 | ASCII |
| 3 | Processing Code | Fixed | 6 | ASCII |
| 4 | Amount | Fixed | 12 | ASCII |
| 7 | Transmission Date/Time | Fixed | 10 | MMDDhhmmss |
| 11 | STAN | Fixed | 6 | ASCII |
| 14 | Expiration Date | Fixed | 4 | YYMM |
| 18 | Merchant Type | Fixed | 4 | ASCII |
| 22 | POS Entry Mode | Fixed | 3 | ASCII |
| 23 | Card Sequence Number | Fixed | 3 | ASCII |
| 25 | POS Condition Code | Fixed | 2 | ASCII |
| 32 | Acquiring Institution ID | LLVAR | 11 | ASCII |
| 37 | RRN | Fixed | 12 | ASCII |
| 38 | Auth ID | Fixed | 6 | ASCII |
| 39 | Response Code | Fixed | 2 | ASCII |
| 41 | Terminal ID | Fixed | 8 | ASCII |
| 42 | Merchant ID | Fixed | 15 | ASCII |
| 43 | Merchant Name/Location | Fixed | 40 | ASCII |
| 48 | Additional Data | LLLVAR | 999 | ASCII |
| 49 | Currency Code | Fixed | 3 | ASCII |
| 55 | ICC Data | LLLVAR | 999 | Binary |
| 63 | Network Data | LLLVAR | 999 | ASCII |

#### Mastercard-Specific Features
- **IPM Integration**: Direct connection to Mastercard processing
- **UCAF Data**: Universal Cardholder Authentication Field in field 48
- **CVC3**: Track 2 equivalent data validation
- **Mastercard Digital Enablement**: Token data in field 48
- **Account Funding Transactions (AFT)**: Special processing code in field 3

#### Response Codes
- `00` - Approved
- `05` - Do not honor
- `14` - Invalid card number
- `51` - Insufficient funds
- `54` - Expired card
- `57` - Transaction not permitted
- `58` - Transaction not permitted to terminal
- `61` - Exceeds withdrawal limit

### 3. NPCI (National Payments Corporation of India)

**Network Code:** `NPCI`

#### Supported Networks
- **RuPay**: Domestic card scheme
- **UPI**: Unified Payments Interface
- **IMPS**: Immediate Payment Service
- **NFS**: National Financial Switch

#### Message Type Indicators (MTI)
- `0200` - Financial Transaction Request
- `0210` - Financial Transaction Response
- `0220` - Financial Advice
- `0230` - Financial Advice Response
- `0400` - Reversal Request
- `0410` - Reversal Response
- `0800` - Network Management
- `0810` - Network Management Response

#### Key Field Specifications

| Field | Name | Type | Length | Encoding |
|-------|------|------|--------|----------|
| 2 | PAN | LLVAR | 19 | ASCII |
| 3 | Processing Code | Fixed | 6 | ASCII |
| 4 | Transaction Amount | Fixed | 12 | ASCII |
| 7 | Transmission Date/Time | Fixed | 10 | MMDDhhmmss |
| 11 | STAN | Fixed | 6 | ASCII |
| 12 | Local Time | Fixed | 6 | hhmmss |
| 13 | Local Date | Fixed | 4 | MMDD |
| 18 | Merchant Category Code | Fixed | 4 | ASCII |
| 22 | POS Entry Mode | Fixed | 3 | ASCII |
| 25 | POS Condition Code | Fixed | 2 | ASCII |
| 32 | Acquiring Institution ID | LLVAR | 11 | ASCII |
| 37 | RRN | Fixed | 12 | ASCII |
| 38 | Auth Code | Fixed | 6 | ASCII |
| 39 | Response Code | Fixed | 2 | ASCII |
| 41 | Terminal ID | Fixed | 8 | ASCII |
| 42 | Merchant ID | Fixed | 15 | ASCII |
| 48 | Additional Data (UPI) | LLLVAR | 999 | ASCII |
| 49 | Currency Code | Fixed | 3 | ASCII (356 for INR) |
| 54 | Additional Amounts | LLLVAR | 999 | ASCII |
| 55 | EMV Data | LLLVAR | 999 | Binary |
| 62 | Private Data | LLLVAR | 999 | ASCII |
| 102 | Account ID 1 | LLVAR | 28 | ASCII |
| 103 | Account ID 2 | LLVAR | 28 | ASCII |
| 123 | POS Data | LLLVAR | 999 | ASCII |

#### NPCI-Specific Features
- **Aadhaar Integration**: Aadhaar number in field 102
- **UPI VPA**: Virtual Payment Address in field 103
- **IMPS MMID**: Mobile Money ID in field 48
- **RuPay Rewards**: Loyalty points in field 54
- **Merchant MDR**: Merchant Discount Rate in field 48

#### UPI-Specific Fields (Field 48 Sub-fields)
```
48.001 - UPI Transaction ID
48.002 - Payer VPA
48.003 - Payee VPA
48.004 - Payer Name
48.005 - Payee Name
48.006 - Payer Account
48.007 - Payee Account
48.008 - IFSC Code
48.009 - Transaction Note
48.010 - Merchant Category
```

## Variant Selection

### Automatic Detection
The gateway automatically selects the appropriate variant based on:
1. **BIN Range**: First 6-8 digits of PAN
2. **Network Configuration**: Configured routing rules
3. **Message Format**: MTI and field presence

### Configuration
```yaml
variants:
  visa:
    enabled: true
    bin_ranges:
      - "4xxxxx"
    endpoints:
      - "visa-acquirer.example.com:8583"
    
  mastercard:
    enabled: true
    bin_ranges:
      - "51xxxx-55xxxx"
      - "222100-272099"
    endpoints:
      - "mastercard-acquirer.example.com:8583"
    
  npci:
    enabled: true
    networks:
      - rupay
      - upi
    bin_ranges:
      - "60xxxx"
      - "65xxxx"
      - "81xxxx"
    endpoints:
      - "npci-switch.example.com:8583"
```

## Custom Variant Implementation

### Creating a New Variant

```go
package packager

import "github.com/krish567366/OpenTX/pkg/iso8583"

type CustomVariantPackager struct {
    *GenericPackager
}

func NewCustomVariantPackager() *CustomVariantPackager {
    fields := map[int]*FieldSpec{
        2: {
            Name:     "PAN",
            Type:     FieldTypeLLVAR,
            MaxLen:   19,
            Encoding: EncodingASCII,
        },
        // Define all fields...
    }
    
    return &CustomVariantPackager{
        GenericPackager: &GenericPackager{
            Fields:      fields,
            MTIEncoding: EncodingASCII,
        },
    }
}

func (p *CustomVariantPackager) Validate(msg *iso8583.Message) error {
    // Custom validation logic
    return p.GenericPackager.Validate(msg)
}
```

### Registering the Variant

```go
func init() {
    RegisterVariant("CUSTOM", NewCustomVariantPackager())
}
```

## Field Encoding Types

### ASCII
Standard ASCII text encoding for alphanumeric data.

### BCD (Binary Coded Decimal)
Each decimal digit encoded in 4 bits. Used for numeric fields.

### Binary
Raw binary data, typically for EMV or encrypted fields.

### EBCDIC
Extended Binary Coded Decimal Interchange Code (legacy systems).

## Testing

Each variant includes comprehensive test suites:

```bash
# Test specific variant
go test ./pkg/iso8583/packager -run TestVisaPackager
go test ./pkg/iso8583/packager -run TestMastercardPackager
go test ./pkg/iso8583/packager -run TestNPCIPackager

# Test all variants
go test ./pkg/iso8583/packager -v
```

## Certification Requirements

### Visa
- **VisaNet Certification**: Required for production
- **Test Cases**: ~500 test scenarios
- **Compliance**: PCI DSS, PA-DSS

### Mastercard
- **M-TIP Certification**: Mastercard Terminal Integration Process
- **Test Cases**: ~600 test scenarios
- **Compliance**: Mastercard SDP requirements

### NPCI
- **RuPay Certification**: For RuPay card acceptance
- **UPI Certification**: For UPI payment integration
- **Test Cases**: ~400 test scenarios
- **Compliance**: RBI guidelines, NPCI security standards

## References

- [Visa ISO 8583 Specification v2.0](https://developer.visa.com)
- [Mastercard IPM Clearing Formats](https://developer.mastercard.com)
- [NPCI Technical Specifications](https://www.npci.org.in/technical-specifications)
- [ISO 8583:1987 Standard](https://www.iso.org/standard/31628.html)
