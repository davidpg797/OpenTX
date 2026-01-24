# EMV Parsing - EMV TLV Data Handling

This document describes EMV (Europay, Mastercard, and Visa) data parsing and handling in OpenTX, including TLV (Tag-Length-Value) structure processing.

## Overview

EMV data is transmitted in ISO 8583 field 55 as a series of TLV-encoded data elements. OpenTX provides comprehensive EMV parsing capabilities to extract and process chip card transaction data.

## EMV Data Structure

### TLV Format

```
┌──────────────┬──────────────┬──────────────┐
│     Tag      │    Length    │    Value     │
├──────────────┼──────────────┼──────────────┤
│  1-4 bytes   │  1-4 bytes   │  Variable    │
└──────────────┴──────────────┴──────────────┘
```

### Tag Encoding

**Single-byte tags**: `0x00` to `0xFF` (except tags ending in `0x1F`)

**Multi-byte tags**: First byte ends in `0x1F` (e.g., `0x9F`, `0xBF`)
- Subsequent bytes: bit 8 = 1 means more bytes follow
- Last byte: bit 8 = 0

Examples:
- Single-byte: `0x9F` followed by `0x02` = Tag `0x9F02`
- Multi-byte: `0x9F` followed by `0x1A` = Tag `0x9F1A`

### Length Encoding

**Short form** (length ≤ 127): 1 byte
```
0xxxxxxx (0x00 to 0x7F)
```

**Long form** (length > 127): Multiple bytes
```
First byte: 1xxxxxxx (bit 7 = 1, bits 6-0 = number of length bytes)
Following bytes: actual length value
```

Examples:
- Length 100: `0x64`
- Length 200: `0x81 0xC8` (1 length byte + value 200)
- Length 1000: `0x82 0x03 0xE8` (2 length bytes + value 1000)

## Common EMV Tags

### Authorization Request Tags

| Tag | Name | Format | Length | Description |
|-----|------|--------|--------|-------------|
| 0x82 | Application Interchange Profile (AIP) | Binary | 2 | Card capabilities |
| 0x84 | Dedicated File Name (DF Name) | Binary | 5-16 | Application identifier |
| 0x95 | Terminal Verification Results (TVR) | Binary | 5 | Verification checks status |
| 0x9A | Transaction Date | Numeric (n6) | 3 | YYMMDD format |
| 0x9C | Transaction Type | Numeric (n2) | 1 | Purchase, cash, etc. |
| 0x9F02 | Amount, Authorized | Numeric (n12) | 6 | Transaction amount |
| 0x9F03 | Amount, Other | Numeric (n12) | 6 | Cashback amount |
| 0x9F09 | Application Version Number | Binary | 2 | App version on card |
| 0x9F10 | Issuer Application Data (IAD) | Binary | 1-32 | Issuer-specific data |
| 0x9F1A | Terminal Country Code | Numeric (n3) | 2 | ISO 3166-1 code |
| 0x9F1E | Interface Device Serial Number | Alphanumeric | 8 | Terminal serial |
| 0x9F26 | Application Cryptogram (AC) | Binary | 8 | Transaction cryptogram |
| 0x9F27 | Cryptogram Information Data (CID) | Binary | 1 | Cryptogram type |
| 0x9F33 | Terminal Capabilities | Binary | 3 | Terminal features |
| 0x9F34 | Cardholder Verification (CVM) Results | Binary | 3 | PIN/signature status |
| 0x9F35 | Terminal Type | Numeric (n2) | 1 | POS, ATM, etc. |
| 0x9F36 | Application Transaction Counter (ATC) | Binary | 2 | Card transaction count |
| 0x9F37 | Unpredictable Number | Binary | 4 | Terminal random number |
| 0x9F41 | Transaction Sequence Counter | Numeric (n4) | 2-4 | Transaction number |
| 0x9F53 | Transaction Category Code | Numeric (n2) | 1 | Transaction category |

### Authorization Response Tags

| Tag | Name | Format | Length | Description |
|-----|------|--------|--------|-------------|
| 0x91 | Issuer Authentication Data | Binary | 8-16 | ARPC from issuer |
| 0x71 | Issuer Script Template 1 | Binary | Variable | Pre-auth scripts |
| 0x72 | Issuer Script Template 2 | Binary | Variable | Post-auth scripts |
| 0x89 | Authorization Code | Alphanumeric | 6 | Approval code |
| 0x8A | Authorization Response Code | Alphanumeric | 2 | Response code |

### Contactless-Specific Tags

| Tag | Name | Format | Length | Description |
|-----|------|--------|--------|-------------|
| 0x9F6E | Form Factor Indicator | Binary | 4 | Device type (phone, watch) |
| 0x9F39 | POS Entry Mode | Numeric (n2) | 1 | Contactless indicator |
| 0x9F66 | Terminal Transaction Qualifiers | Binary | 4 | Contactless capabilities |
| 0x9F02 | Amount, Authorized | Numeric (n12) | 6 | Transaction amount |
| 0x9F03 | Amount, Other | Numeric (n12) | 6 | Additional amounts |

## EMV Parser Implementation

### Core Parser

```go
type EMVParser struct {
    data   []byte
    offset int
}

func NewEMVParser(data []byte) *EMVParser {
    return &EMVParser{
        data:   data,
        offset: 0,
    }
}

func (p *EMVParser) Parse() (*EMVData, error) {
    emvData := &EMVData{
        Tags: make(map[string]*EMVTag),
    }
    
    for p.offset < len(p.data) {
        tag, err := p.parseTag()
        if err != nil {
            return nil, fmt.Errorf("failed to parse tag: %w", err)
        }
        
        length, err := p.parseLength()
        if err != nil {
            return nil, fmt.Errorf("failed to parse length: %w", err)
        }
        
        value, err := p.parseValue(length)
        if err != nil {
            return nil, fmt.Errorf("failed to parse value: %w", err)
        }
        
        emvTag := &EMVTag{
            Tag:    tag,
            Length: length,
            Value:  value,
        }
        
        emvData.Tags[tag] = emvTag
        
        // If this is a constructed tag, parse nested TLVs
        if p.isConstructed(tag) {
            nested, err := NewEMVParser(value).Parse()
            if err != nil {
                return nil, fmt.Errorf("failed to parse nested TLV: %w", err)
            }
            emvTag.Nested = nested.Tags
        }
    }
    
    return emvData, nil
}
```

### Tag Parsing

```go
func (p *EMVParser) parseTag() (string, error) {
    if p.offset >= len(p.data) {
        return "", io.EOF
    }
    
    firstByte := p.data[p.offset]
    p.offset++
    
    // Single-byte tag (bits 1-5 not all 1s)
    if (firstByte & 0x1F) != 0x1F {
        return fmt.Sprintf("%02X", firstByte), nil
    }
    
    // Multi-byte tag
    tagBytes := []byte{firstByte}
    
    for p.offset < len(p.data) {
        nextByte := p.data[p.offset]
        p.offset++
        tagBytes = append(tagBytes, nextByte)
        
        // If bit 8 is 0, this is the last byte
        if (nextByte & 0x80) == 0 {
            break
        }
    }
    
    return strings.ToUpper(hex.EncodeToString(tagBytes)), nil
}
```

### Length Parsing

```go
func (p *EMVParser) parseLength() (int, error) {
    if p.offset >= len(p.data) {
        return 0, io.EOF
    }
    
    firstByte := p.data[p.offset]
    p.offset++
    
    // Short form (bit 8 = 0)
    if (firstByte & 0x80) == 0 {
        return int(firstByte), nil
    }
    
    // Long form (bit 8 = 1)
    numLengthBytes := int(firstByte & 0x7F)
    
    if numLengthBytes == 0 {
        return 0, errors.New("indefinite length not supported")
    }
    
    if p.offset+numLengthBytes > len(p.data) {
        return 0, io.ErrUnexpectedEOF
    }
    
    length := 0
    for i := 0; i < numLengthBytes; i++ {
        length = (length << 8) | int(p.data[p.offset])
        p.offset++
    }
    
    return length, nil
}
```

### Value Parsing

```go
func (p *EMVParser) parseValue(length int) ([]byte, error) {
    if p.offset+length > len(p.data) {
        return nil, io.ErrUnexpectedEOF
    }
    
    value := make([]byte, length)
    copy(value, p.data[p.offset:p.offset+length])
    p.offset += length
    
    return value, nil
}
```

### Constructed Tag Detection

```go
func (p *EMVParser) isConstructed(tagStr string) bool {
    tagBytes, _ := hex.DecodeString(tagStr)
    if len(tagBytes) == 0 {
        return false
    }
    
    // Bit 6 of first byte indicates constructed (1) or primitive (0)
    return (tagBytes[0] & 0x20) != 0
}
```

## EMV Data Structures

### EMV Data Model

```go
type EMVData struct {
    Tags                    map[string]*EMVTag
    ApplicationID           string
    ApplicationLabel        string
    ApplicationVersionNum   []byte
    TransactionType         byte
    Amount                  int64
    OtherAmount             int64
    TerminalCountryCode     string
    TransactionDate         string
    TransactionTime         string
    Cryptogram              []byte
    CryptogramType          CryptogramType
    CVM                     CVMResults
    TVR                     TerminalVerificationResults
    TSI                     TransactionStatusInformation
    ATC                     uint16
    UnpredictableNumber     []byte
}

type EMVTag struct {
    Tag    string
    Length int
    Value  []byte
    Nested map[string]*EMVTag
}

type CryptogramType byte

const (
    CryptogramAAC CryptogramType = 0x00 // Application Authentication Cryptogram (declined)
    CryptogramTC  CryptogramType = 0x40 // Transaction Certificate (approved)
    CryptogramARQC CryptogramType = 0x80 // Authorization Request Cryptogram (online)
)

type CVMResults struct {
    Method         byte // CVM method performed
    Condition      byte // CVM condition code
    Result         byte // CVM result
}

type TerminalVerificationResults struct {
    OfflineDataAuthFailed       bool
    ICCDataMissing              bool
    CardholderVerificationFailed bool
    UnrecognizedCVM             bool
    RequestedServiceNotAllowed   bool
    NewCard                     bool
    // ... more flags
}
```

## EMV Data Extraction

### Extract Key Fields

```go
func (e *EMVData) ExtractKeyFields() error {
    // Application ID (AID)
    if tag, ok := e.Tags["84"]; ok {
        e.ApplicationID = hex.EncodeToString(tag.Value)
    }
    
    // Application Label
    if tag, ok := e.Tags["50"]; ok {
        e.ApplicationLabel = string(tag.Value)
    }
    
    // Amount, Authorized (BCD format)
    if tag, ok := e.Tags["9F02"]; ok {
        e.Amount = decodeBCDAmount(tag.Value)
    }
    
    // Amount, Other (cashback)
    if tag, ok := e.Tags["9F03"]; ok {
        e.OtherAmount = decodeBCDAmount(tag.Value)
    }
    
    // Transaction Date (YYMMDD)
    if tag, ok := e.Tags["9A"]; ok {
        e.TransactionDate = decodeBCDDate(tag.Value)
    }
    
    // Transaction Type
    if tag, ok := e.Tags["9C"]; ok && len(tag.Value) > 0 {
        e.TransactionType = tag.Value[0]
    }
    
    // Application Cryptogram
    if tag, ok := e.Tags["9F26"]; ok {
        e.Cryptogram = tag.Value
    }
    
    // Cryptogram Information Data
    if tag, ok := e.Tags["9F27"]; ok && len(tag.Value) > 0 {
        e.CryptogramType = CryptogramType(tag.Value[0] & 0xC0)
    }
    
    // CVM Results
    if tag, ok := e.Tags["9F34"]; ok {
        e.CVM = parseCVMResults(tag.Value)
    }
    
    // Terminal Verification Results
    if tag, ok := e.Tags["95"]; ok {
        e.TVR = parseTVR(tag.Value)
    }
    
    // Application Transaction Counter
    if tag, ok := e.Tags["9F36"]; ok && len(tag.Value) == 2 {
        e.ATC = binary.BigEndian.Uint16(tag.Value)
    }
    
    // Unpredictable Number
    if tag, ok := e.Tags["9F37"]; ok {
        e.UnpredictableNumber = tag.Value
    }
    
    return nil
}
```

### BCD Decoding

```go
func decodeBCDAmount(bcd []byte) int64 {
    var amount int64
    for _, b := range bcd {
        highNibble := (b >> 4) & 0x0F
        lowNibble := b & 0x0F
        amount = amount*100 + int64(highNibble)*10 + int64(lowNibble)
    }
    return amount
}

func decodeBCDDate(bcd []byte) string {
    if len(bcd) != 3 {
        return ""
    }
    
    year := fmt.Sprintf("%02d", bcd[0])
    month := fmt.Sprintf("%02d", bcd[1])
    day := fmt.Sprintf("%02d", bcd[2])
    
    return fmt.Sprintf("20%s-%s-%s", year, month, day)
}
```

### CVM Results Parsing

```go
func parseCVMResults(data []byte) CVMResults {
    if len(data) != 3 {
        return CVMResults{}
    }
    
    return CVMResults{
        Method:    data[0],
        Condition: data[1],
        Result:    data[2],
    }
}

func (cvm CVMResults) String() string {
    methodStr := map[byte]string{
        0x00: "Fail CVM processing",
        0x01: "Plaintext PIN verification by ICC",
        0x02: "Enciphered PIN verified online",
        0x03: "Plaintext PIN verification by ICC and signature",
        0x04: "Enciphered PIN verification by ICC",
        0x05: "Enciphered PIN verification by ICC and signature",
        0x1E: "Signature",
        0x1F: "No CVM required",
    }[cvm.Method]
    
    resultStr := map[byte]string{
        0x00: "Unknown",
        0x01: "Failed",
        0x02: "Successful",
    }[cvm.Result]
    
    return fmt.Sprintf("%s: %s", methodStr, resultStr)
}
```

## EMV Transaction Flow

### Authorization Request Processing

```go
func ProcessEMVAuthRequest(iso8583Msg *iso8583.Message) (*EMVData, error) {
    // Extract field 55 (EMV data)
    field55 := iso8583Msg.GetField(55)
    if field55 == nil {
        return nil, errors.New("EMV data not present")
    }
    
    // Parse EMV TLV data
    parser := NewEMVParser(field55.Value)
    emvData, err := parser.Parse()
    if err != nil {
        return nil, fmt.Errorf("failed to parse EMV data: %w", err)
    }
    
    // Extract key fields
    if err := emvData.ExtractKeyFields(); err != nil {
        return nil, fmt.Errorf("failed to extract EMV fields: %w", err)
    }
    
    // Validate EMV data
    if err := validateEMVData(emvData); err != nil {
        return nil, fmt.Errorf("EMV validation failed: %w", err)
    }
    
    return emvData, nil
}
```

### EMV Validation

```go
func validateEMVData(emv *EMVData) error {
    // Cryptogram must be present
    if len(emv.Cryptogram) != 8 {
        return errors.New("invalid or missing application cryptogram")
    }
    
    // Amount must match transaction amount
    if emv.Amount <= 0 {
        return errors.New("invalid transaction amount in EMV data")
    }
    
    // ATC should be non-zero
    if emv.ATC == 0 {
        return errors.New("invalid application transaction counter")
    }
    
    // Unpredictable number should be present
    if len(emv.UnpredictableNumber) != 4 {
        return errors.New("invalid unpredictable number")
    }
    
    // Check TVR for critical failures
    if emv.TVR.OfflineDataAuthFailed {
        return errors.New("offline data authentication failed")
    }
    
    return nil
}
```

## Building EMV Response Data

### Create Issuer Response

```go
func BuildEMVResponse(authCode string, arpc []byte) ([]byte, error) {
    tlvData := NewTLVBuilder()
    
    // Authorization Response Code (tag 8A)
    tlvData.AddTag("8A", []byte(authCode))
    
    // Issuer Authentication Data (tag 91) - ARPC
    if len(arpc) > 0 {
        tlvData.AddTag("91", arpc)
    }
    
    // Issuer Script Template 1 (tag 71) - optional
    // Issuer Script Template 2 (tag 72) - optional
    
    return tlvData.Build(), nil
}

type TLVBuilder struct {
    buffer bytes.Buffer
}

func NewTLVBuilder() *TLVBuilder {
    return &TLVBuilder{}
}

func (b *TLVBuilder) AddTag(tagStr string, value []byte) error {
    // Write tag
    tagBytes, err := hex.DecodeString(tagStr)
    if err != nil {
        return err
    }
    b.buffer.Write(tagBytes)
    
    // Write length
    length := len(value)
    if length <= 127 {
        b.buffer.WriteByte(byte(length))
    } else {
        // Long form
        if length <= 255 {
            b.buffer.WriteByte(0x81)
            b.buffer.WriteByte(byte(length))
        } else {
            b.buffer.WriteByte(0x82)
            b.buffer.WriteByte(byte(length >> 8))
            b.buffer.WriteByte(byte(length & 0xFF))
        }
    }
    
    // Write value
    b.buffer.Write(value)
    
    return nil
}

func (b *TLVBuilder) Build() []byte {
    return b.buffer.Bytes()
}
```

## Contactless EMV

### Kernel-Specific Processing

Different contactless kernels (Visa, Mastercard, Amex, Discover) have specific requirements:

```go
type ContactlessKernel string

const (
    KernelVisa       ContactlessKernel = "VISA_PAYWAVE"
    KernelMastercard ContactlessKernel = "MC_PAYPASS"
    KernelAmex       ContactlessKernel = "AMEX_EXPRESSPAY"
    KernelDiscover   ContactlessKernel = "DISCOVER_DPAS"
)

func DetectKernel(aid string) ContactlessKernel {
    switch {
    case strings.HasPrefix(aid, "A0000000031010"):
        return KernelVisa
    case strings.HasPrefix(aid, "A0000000041010"):
        return KernelMastercard
    case strings.HasPrefix(aid, "A00000002501"):
        return KernelAmex
    case strings.HasPrefix(aid, "A0000001523010"):
        return KernelDiscover
    default:
        return ""
    }
}
```

## Testing

### EMV Parser Tests

```go
func TestEMVParser_Parse(t *testing.T) {
    // Sample EMV data from real transaction
    emvHex := "9F2608AB1234567890CDEF9F2701809F1012345678901234569F3602012395050000000000"
    emvData, _ := hex.DecodeString(emvHex)
    
    parser := NewEMVParser(emvData)
    result, err := parser.Parse()
    
    assert.NoError(t, err)
    assert.NotNil(t, result.Tags["9F26"]) // Cryptogram
    assert.NotNil(t, result.Tags["9F27"]) // CID
    assert.Equal(t, 8, len(result.Tags["9F26"].Value))
}
```

## EMV Debugging

### Pretty Print EMV Data

```go
func (e *EMVData) PrettyPrint() string {
    var buf bytes.Buffer
    
    buf.WriteString("EMV Data:\n")
    buf.WriteString(fmt.Sprintf("  Application ID: %s\n", e.ApplicationID))
    buf.WriteString(fmt.Sprintf("  Amount: %d\n", e.Amount))
    buf.WriteString(fmt.Sprintf("  Cryptogram Type: %s\n", e.CryptogramType))
    buf.WriteString(fmt.Sprintf("  ATC: %d\n", e.ATC))
    
    buf.WriteString("\nAll Tags:\n")
    for tag, data := range e.Tags {
        buf.WriteString(fmt.Sprintf("  %s: %s\n", tag, hex.EncodeToString(data.Value)))
    }
    
    return buf.String()
}
```

## References

- [EMV 4.3 Book 3: Application Specification](https://www.emvco.com/specifications/)
- [EMV Contactless Specifications](https://www.emvco.com/emv-technologies/contactless/)
- [Payment Card Industry TLV Reference](https://www.pcisecuritystandards.org/)
- [ISO/IEC 7816-4: Smart Cards Standard](https://www.iso.org/standard/54550.html)
