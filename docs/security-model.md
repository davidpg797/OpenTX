# Security Model

This document describes the comprehensive security architecture of OpenTX, covering encryption, digital signatures, HSM integration, and security best practices.

## Security Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Security Layers                          │
├─────────────────────────────────────────────────────────────┤
│  1. Transport Security (TLS 1.3)                             │
│  2. Message-Level Encryption (AES-256-GCM)                   │
│  3. Digital Signatures (RSA-PSS / ECDSA)                     │
│  4. PIN Security (DUKPT / 3DES)                              │
│  5. HSM Integration (PKCS#11)                                │
│  6. Key Management (Rotation, Versioning)                    │
│  7. Anti-Replay Protection (Nonce-based)                     │
│  8. Audit Logging (Immutable, Encrypted)                     │
└─────────────────────────────────────────────────────────────┘
```

## 1. Transport Security

### TLS 1.3 Configuration

All network communications use TLS 1.3 with strong cipher suites:

```go
tlsConfig := &tls.Config{
    MinVersion: tls.VersionTLS13,
    CipherSuites: []uint16{
        tls.TLS_AES_256_GCM_SHA384,
        tls.TLS_CHACHA20_POLY1305_SHA256,
    },
    CurvePreferences: []tls.CurveID{
        tls.X25519,
        tls.CurveP384,
    },
    ClientAuth: tls.RequireAndVerifyClientCert,
}
```

### Mutual TLS (mTLS)

Client and server certificates are required for all connections:

```yaml
tls:
  server_cert: /etc/opentx/certs/server.crt
  server_key: /etc/opentx/certs/server.key
  ca_cert: /etc/opentx/certs/ca.crt
  client_cert: /etc/opentx/certs/client.crt
  client_key: /etc/opentx/certs/client.key
```

## 2. Message-Level Encryption

### AES-256-GCM Encryption

Message payloads are encrypted using AES-256 in GCM mode for authenticated encryption:

```go
// Encrypt sensitive fields
encrypted, err := securityProvider.Encrypt([]byte(sensitiveData), keyID)
if err != nil {
    return err
}

// Decrypt on receiving end
decrypted, err := securityProvider.Decrypt(encrypted, keyID)
if err != nil {
    return err
}
```

### Field-Level Encryption

Specific fields can be encrypted individually:

```yaml
encryption:
  fields:
    - field: 2  # PAN
      algorithm: AES-256-GCM
      key_id: pan-encryption-key
    - field: 52 # PIN Block
      algorithm: 3DES-DUKPT
      key_id: pin-encryption-key
    - field: 55 # EMV Data
      algorithm: AES-256-GCM
      key_id: emv-encryption-key
```

### Encryption Format

```
┌──────────────────────────────────────────────────┐
│ Version (1 byte) │ Key ID (16 bytes)             │
├──────────────────────────────────────────────────┤
│ Nonce/IV (12 bytes)                              │
├──────────────────────────────────────────────────┤
│ Ciphertext (variable)                            │
├──────────────────────────────────────────────────┤
│ Authentication Tag (16 bytes)                    │
└──────────────────────────────────────────────────┘
```

## 3. Digital Signatures

### RSA-PSS Signatures

Messages are signed using RSA-PSS with SHA-384:

```go
// Sign message
signature, err := securityProvider.Sign(messageHash, signingKeyID)
if err != nil {
    return err
}

// Verify signature
valid, err := securityProvider.Verify(messageHash, signature, verifyKeyID)
if err != nil || !valid {
    return errors.New("signature verification failed")
}
```

### ECDSA Support

For high-performance scenarios, ECDSA with P-384 curve:

```go
ecdsaProvider := security.NewECDSAProvider(security.ECDSAConfig{
    Curve: elliptic.P384(),
    Hash:  crypto.SHA384,
})

signature, err := ecdsaProvider.Sign(data, privateKey)
```

### Signature Format

```json
{
  "algorithm": "RSA-PSS-SHA384",
  "key_id": "signing-key-2024-01",
  "signature": "base64_encoded_signature",
  "timestamp": "2024-01-24T12:00:00Z",
  "message_digest": "sha384_hash"
}
```

## 4. PIN Security

### DUKPT (Derived Unique Key Per Transaction)

PIN blocks are encrypted using DUKPT methodology:

```go
type DUKPTProvider struct {
    BDK        []byte // Base Derivation Key
    KSN        string // Key Serial Number
    KeyCounter uint32
}

func (d *DUKPTProvider) EncryptPIN(pin string, pan string) ([]byte, error) {
    // Derive session key
    sessionKey := d.deriveKey(d.KSN)
    
    // Format PIN block (ISO Format 0)
    pinBlock := formatPINBlock(pin, pan)
    
    // Encrypt with 3DES
    encrypted := encrypt3DES(pinBlock, sessionKey)
    
    return encrypted, nil
}
```

### PIN Block Formats

**ISO Format 0 (ANSI X9.8)**
```
0 | PIN Length | PIN | Padding | XOR with PAN
```

**ISO Format 1**
```
1 | PIN Block Identifier | PIN | Random Padding
```

**ISO Format 4**
```
4 | PIN Length | PIN | Random Padding | XOR with PAN
```

### PIN Translation

Converting between different PIN encryption methods:

```go
func TranslatePIN(
    encryptedPIN []byte,
    sourceKey []byte,
    destKey []byte,
    pan string,
) ([]byte, error) {
    // Decrypt with source key
    pinBlock := decrypt3DES(encryptedPIN, sourceKey)
    
    // Re-encrypt with destination key
    reencrypted := encrypt3DES(pinBlock, destKey)
    
    return reencrypted, nil
}
```

## 5. HSM Integration

### PKCS#11 Interface

Integration with Hardware Security Modules:

```go
type HSMConfig struct {
    Library    string
    SlotID     uint
    PIN        string
    Label      string
}

type HSMProvider struct {
    ctx    *pkcs11.Ctx
    session pkcs11.SessionHandle
}

func NewHSMProvider(config HSMConfig) (*HSMProvider, error) {
    p := pkcs11.New(config.Library)
    
    err := p.Initialize()
    if err != nil {
        return nil, err
    }
    
    session, err := p.OpenSession(config.SlotID, pkcs11.CKF_SERIAL_SESSION|pkcs11.CKF_RW_SESSION)
    if err != nil {
        return nil, err
    }
    
    err = p.Login(session, pkcs11.CKU_USER, config.PIN)
    if err != nil {
        return nil, err
    }
    
    return &HSMProvider{ctx: p, session: session}, nil
}
```

### Key Generation in HSM

```go
func (h *HSMProvider) GenerateKeyPair(label string) error {
    publicKeyTemplate := []*pkcs11.Attribute{
        pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PUBLIC_KEY),
        pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_RSA),
        pkcs11.NewAttribute(pkcs11.CKA_TOKEN, true),
        pkcs11.NewAttribute(pkcs11.CKA_VERIFY, true),
        pkcs11.NewAttribute(pkcs11.CKA_PUBLIC_EXPONENT, []byte{1, 0, 1}),
        pkcs11.NewAttribute(pkcs11.CKA_MODULUS_BITS, 4096),
        pkcs11.NewAttribute(pkcs11.CKA_LABEL, label),
    }
    
    privateKeyTemplate := []*pkcs11.Attribute{
        pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PRIVATE_KEY),
        pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_RSA),
        pkcs11.NewAttribute(pkcs11.CKA_TOKEN, true),
        pkcs11.NewAttribute(pkcs11.CKA_SIGN, true),
        pkcs11.NewAttribute(pkcs11.CKA_SENSITIVE, true),
        pkcs11.NewAttribute(pkcs11.CKA_EXTRACTABLE, false),
        pkcs11.NewAttribute(pkcs11.CKA_LABEL, label),
    }
    
    pubKey, privKey, err := h.ctx.GenerateKeyPair(
        h.session,
        []*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_RSA_PKCS_KEY_PAIR_GEN, nil)},
        publicKeyTemplate,
        privateKeyTemplate,
    )
    
    return err
}
```

### Supported HSM Vendors

- **Thales (nShield)**: Via PKCS#11
- **Utimaco**: Via PKCS#11
- **AWS CloudHSM**: Via PKCS#11
- **Azure Key Vault**: Via REST API
- **Google Cloud HSM**: Via Cloud KMS API

## 6. Key Management

### Key Hierarchy

```
┌─────────────────────────────────────┐
│   Master Key (KEK)                  │
│   Protected by HSM                  │
└──────────────┬──────────────────────┘
               │
     ┌─────────┴─────────┐
     │                   │
┌────▼─────┐      ┌─────▼────┐
│ Data     │      │ Session  │
│ Encryption│     │ Keys     │
│ Keys (DEK)│     │ (Ephemeral)
└──────────┘      └──────────┘
```

### Key Rotation

Automatic key rotation policy:

```yaml
key_management:
  rotation:
    enabled: true
    interval: 90d
    grace_period: 7d
  
  keys:
    - id: encryption-key-2024-01
      type: AES-256
      purpose: data_encryption
      created: 2024-01-01
      expires: 2024-04-01
      status: active
    
    - id: encryption-key-2023-10
      type: AES-256
      purpose: data_encryption
      created: 2023-10-01
      expires: 2024-01-08
      status: deprecated
```

### Key Versioning

```go
type KeyVersion struct {
    ID        string    `json:"id"`
    Version   int       `json:"version"`
    Algorithm string    `json:"algorithm"`
    Status    KeyStatus `json:"status"`
    CreatedAt time.Time `json:"created_at"`
    ExpiresAt time.Time `json:"expires_at"`
}

func (km *KeyManager) GetActiveKey(purpose string) (*KeyVersion, error) {
    keys := km.listKeys(purpose)
    
    for _, key := range keys {
        if key.Status == KeyStatusActive && time.Now().Before(key.ExpiresAt) {
            return &key, nil
        }
    }
    
    return nil, errors.New("no active key found")
}
```

## 7. Anti-Replay Protection

### Nonce-Based Protection

Each message includes a unique nonce to prevent replay attacks:

```go
type NonceStore interface {
    Store(nonce string, expiry time.Duration) error
    Exists(nonce string) (bool, error)
    Cleanup() error
}

func (s *SecurityProvider) ValidateNonce(nonce string) error {
    exists, err := s.nonceStore.Exists(nonce)
    if err != nil {
        return err
    }
    
    if exists {
        return errors.New("duplicate nonce: replay attack detected")
    }
    
    // Store nonce with 5-minute expiry
    return s.nonceStore.Store(nonce, 5*time.Minute)
}
```

### Timestamp Validation

Messages must arrive within acceptable time window:

```go
func ValidateTimestamp(msgTimestamp time.Time, maxSkew time.Duration) error {
    now := time.Now()
    diff := now.Sub(msgTimestamp)
    
    if diff < 0 {
        diff = -diff
    }
    
    if diff > maxSkew {
        return fmt.Errorf("timestamp skew too large: %v", diff)
    }
    
    return nil
}
```

## 8. Audit Logging

### Immutable Audit Trail

All security events are logged to an immutable audit log:

```go
type AuditEvent struct {
    EventID     string                 `json:"event_id"`
    Timestamp   time.Time              `json:"timestamp"`
    EventType   string                 `json:"event_type"`
    Actor       string                 `json:"actor"`
    Resource    string                 `json:"resource"`
    Action      string                 `json:"action"`
    Result      string                 `json:"result"`
    Details     map[string]interface{} `json:"details"`
    Signature   string                 `json:"signature"`
}

func (a *AuditLogger) Log(event AuditEvent) error {
    // Sign event
    eventJSON, _ := json.Marshal(event)
    signature, _ := a.signer.Sign(eventJSON)
    event.Signature = signature
    
    // Store in tamper-proof storage
    return a.storage.Append(event)
}
```

### Logged Security Events

- Key generation/rotation
- Encryption/decryption operations
- Signature generation/verification
- Authentication attempts
- Authorization decisions
- HSM operations
- Configuration changes
- Security policy violations

## Security Configuration

### Complete Configuration Example

```yaml
security:
  # TLS Configuration
  tls:
    enabled: true
    min_version: "1.3"
    client_auth: required
    certs:
      server_cert: /etc/opentx/certs/server.crt
      server_key: /etc/opentx/certs/server.key
      ca_cert: /etc/opentx/certs/ca.crt
  
  # Encryption Configuration
  encryption:
    algorithm: AES-256-GCM
    key_derivation: PBKDF2-SHA384
    iterations: 100000
    fields:
      - field: 2
        enabled: true
      - field: 52
        enabled: true
  
  # Signature Configuration
  signature:
    algorithm: RSA-PSS-SHA384
    key_size: 4096
    verify_incoming: true
    sign_outgoing: true
  
  # HSM Configuration
  hsm:
    enabled: true
    provider: pkcs11
    library: /usr/lib/libCryptoki2_64.so
    slot_id: 0
    pin: ${HSM_PIN}
    key_label: opentx-master-key
  
  # Anti-Replay Configuration
  anti_replay:
    enabled: true
    nonce_cache_ttl: 300s
    max_timestamp_skew: 60s
  
  # Audit Configuration
  audit:
    enabled: true
    output: /var/log/opentx/audit.log
    encryption: true
    rotation: daily
    retention: 90d
```

## Security Best Practices

1. **Key Storage**: Never store keys in plaintext; always use HSM or encrypted key store
2. **Key Rotation**: Implement automated key rotation with 90-day cycles
3. **Least Privilege**: Grant minimum necessary permissions
4. **Defense in Depth**: Use multiple security layers
5. **Regular Audits**: Review audit logs and conduct security assessments
6. **Incident Response**: Have documented procedures for security incidents
7. **Compliance**: Maintain PCI DSS, SOC 2, ISO 27001 compliance
8. **Testing**: Regular penetration testing and vulnerability scanning

## Compliance

### PCI DSS Requirements

- **Requirement 3**: Protect stored cardholder data
- **Requirement 4**: Encrypt transmission of cardholder data
- **Requirement 8**: Identify and authenticate access
- **Requirement 10**: Track and monitor all access

### PA-DSS Requirements

- Do not retain full track data, card validation code, or PIN
- Protect stored cardholder data
- Provide secure authentication features
- Log payment application activity

## References

- [PCI DSS v4.0](https://www.pcisecuritystandards.org/)
- [NIST Cryptographic Standards](https://csrc.nist.gov/)
- [OWASP Cryptographic Storage Cheat Sheet](https://cheatsheetseries.owasp.org/)
- [PKCS#11 v2.40 Specification](http://docs.oasis-open.org/pkcs11/)
