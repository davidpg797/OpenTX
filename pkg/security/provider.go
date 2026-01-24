package security

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"time"
)

// SecurityProvider handles message-level security
type SecurityProvider interface {
	// Encrypt encrypts a payload
	Encrypt(payload []byte, keyID string) (*SecurityEnvelope, error)
	
	// Decrypt decrypts a security envelope
	Decrypt(envelope *SecurityEnvelope) ([]byte, error)
	
	// Sign signs a payload
	Sign(payload []byte, keyID string) ([]byte, error)
	
	// Verify verifies a signature
	Verify(payload []byte, signature []byte, keyID string) error
	
	// ValidateReplay validates replay protection
	ValidateReplay(envelope *SecurityEnvelope) error
}

// SecurityEnvelope contains encrypted/signed message data
type SecurityEnvelope struct {
	KeyID            string
	KeyVersion       string
	EncryptedPayload []byte
	Signature        []byte
	Nonce            string
	Timestamp        time.Time
	Algorithm        string
}

// DefaultSecurityProvider implements basic security operations
type DefaultSecurityProvider struct {
	keys         map[string]*KeyMaterial
	replayWindow time.Duration
	nonceCache   NonceCache
}

// KeyMaterial contains cryptographic key material
type KeyMaterial struct {
	KeyID      string
	Version    string
	PublicKey  *rsa.PublicKey
	PrivateKey *rsa.PrivateKey
	SymmetricKey []byte
	CreatedAt  time.Time
}

// NonceCache provides nonce-based replay protection
type NonceCache interface {
	Add(nonce string, expiry time.Time) error
	Exists(nonce string) bool
}

// NewDefaultSecurityProvider creates a new security provider
func NewDefaultSecurityProvider(replayWindow time.Duration) *DefaultSecurityProvider {
	return &DefaultSecurityProvider{
		keys:         make(map[string]*KeyMaterial),
		replayWindow: replayWindow,
		nonceCache:   NewMemoryNonceCache(),
	}
}

// LoadKey loads a key into the provider
func (p *DefaultSecurityProvider) LoadKey(keyID string, publicKeyPEM, privateKeyPEM []byte) error {
	km := &KeyMaterial{
		KeyID:     keyID,
		Version:   "v1",
		CreatedAt: time.Now(),
	}
	
	// Parse public key
	if publicKeyPEM != nil {
		block, _ := pem.Decode(publicKeyPEM)
		if block == nil {
			return fmt.Errorf("failed to decode public key PEM")
		}
		
		pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse public key: %w", err)
		}
		
		rsaPubKey, ok := pubKey.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("not an RSA public key")
		}
		km.PublicKey = rsaPubKey
	}
	
	// Parse private key
	if privateKeyPEM != nil {
		block, _ := pem.Decode(privateKeyPEM)
		if block == nil {
			return fmt.Errorf("failed to decode private key PEM")
		}
		
		privKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse private key: %w", err)
		}
		
		rsaPrivKey, ok := privKey.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("not an RSA private key")
		}
		km.PrivateKey = rsaPrivKey
	}
	
	p.keys[keyID] = km
	return nil
}

// LoadSymmetricKey loads a symmetric key
func (p *DefaultSecurityProvider) LoadSymmetricKey(keyID string, key []byte) error {
	km := &KeyMaterial{
		KeyID:        keyID,
		Version:      "v1",
		SymmetricKey: key,
		CreatedAt:    time.Now(),
	}
	
	p.keys[keyID] = km
	return nil
}

// Encrypt encrypts a payload using AES-GCM
func (p *DefaultSecurityProvider) Encrypt(payload []byte, keyID string) (*SecurityEnvelope, error) {
	km, exists := p.keys[keyID]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", keyID)
	}
	
	if km.SymmetricKey == nil {
		return nil, fmt.Errorf("symmetric key not available for: %s", keyID)
	}
	
	// Create AES cipher
	block, err := aes.NewCipher(km.SymmetricKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	
	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	
	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	
	// Encrypt
	ciphertext := gcm.Seal(nil, nonce, payload, nil)
	
	envelope := &SecurityEnvelope{
		KeyID:            keyID,
		KeyVersion:       km.Version,
		EncryptedPayload: ciphertext,
		Nonce:            base64.StdEncoding.EncodeToString(nonce),
		Timestamp:        time.Now(),
		Algorithm:        "AES-256-GCM",
	}
	
	return envelope, nil
}

// Decrypt decrypts a security envelope
func (p *DefaultSecurityProvider) Decrypt(envelope *SecurityEnvelope) ([]byte, error) {
	km, exists := p.keys[envelope.KeyID]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", envelope.KeyID)
	}
	
	if km.SymmetricKey == nil {
		return nil, fmt.Errorf("symmetric key not available")
	}
	
	// Create AES cipher
	block, err := aes.NewCipher(km.SymmetricKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	
	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	
	// Decode nonce
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decode nonce: %w", err)
	}
	
	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, envelope.EncryptedPayload, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}
	
	return plaintext, nil
}

// Sign signs a payload using RSA-PSS
func (p *DefaultSecurityProvider) Sign(payload []byte, keyID string) ([]byte, error) {
	km, exists := p.keys[keyID]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", keyID)
	}
	
	if km.PrivateKey == nil {
		return nil, fmt.Errorf("private key not available")
	}
	
	// Hash the payload
	hash := sha256.Sum256(payload)
	
	// Sign using RSA-PSS
	signature, err := rsa.SignPSS(rand.Reader, km.PrivateKey, crypto.SHA256, hash[:], nil)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}
	
	return signature, nil
}

// Verify verifies a signature
func (p *DefaultSecurityProvider) Verify(payload []byte, signature []byte, keyID string) error {
	km, exists := p.keys[keyID]
	if !exists {
		return fmt.Errorf("key not found: %s", keyID)
	}
	
	if km.PublicKey == nil {
		return fmt.Errorf("public key not available")
	}
	
	// Hash the payload
	hash := sha256.Sum256(payload)
	
	// Verify using RSA-PSS
	err := rsa.VerifyPSS(km.PublicKey, crypto.SHA256, hash[:], signature, nil)
	if err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}
	
	return nil
}

// ValidateReplay validates replay protection
func (p *DefaultSecurityProvider) ValidateReplay(envelope *SecurityEnvelope) error {
	// Check timestamp
	age := time.Since(envelope.Timestamp)
	if age > p.replayWindow {
		return fmt.Errorf("message too old: %v", age)
	}
	
	// Check if timestamp is in future (clock skew tolerance: 5 minutes)
	if envelope.Timestamp.After(time.Now().Add(5 * time.Minute)) {
		return fmt.Errorf("message timestamp is in the future")
	}
	
	// Check nonce
	if p.nonceCache.Exists(envelope.Nonce) {
		return fmt.Errorf("nonce already used (replay attack)")
	}
	
	// Add nonce to cache
	expiry := envelope.Timestamp.Add(p.replayWindow)
	if err := p.nonceCache.Add(envelope.Nonce, expiry); err != nil {
		return fmt.Errorf("failed to cache nonce: %w", err)
	}
	
	return nil
}

// MemoryNonceCache implements an in-memory nonce cache
type MemoryNonceCache struct {
	cache map[string]time.Time
}

// NewMemoryNonceCache creates a new memory-based nonce cache
func NewMemoryNonceCache() *MemoryNonceCache {
	return &MemoryNonceCache{
		cache: make(map[string]time.Time),
	}
}

// Add adds a nonce to the cache
func (c *MemoryNonceCache) Add(nonce string, expiry time.Time) error {
	c.cache[nonce] = expiry
	// TODO: Add cleanup goroutine to remove expired nonces
	return nil
}

// Exists checks if a nonce exists in the cache
func (c *MemoryNonceCache) Exists(nonce string) bool {
	expiry, exists := c.cache[nonce]
	if !exists {
		return false
	}
	
	// Check if expired
	if time.Now().After(expiry) {
		delete(c.cache, nonce)
		return false
	}
	
	return true
}

// HSMProvider interface for Hardware Security Module integration
type HSMProvider interface {
	// GenerateKey generates a new key in the HSM
	GenerateKey(keyID string, keyType string) error
	
	// Encrypt encrypts data using HSM
	Encrypt(keyID string, plaintext []byte) ([]byte, error)
	
	// Decrypt decrypts data using HSM
	Decrypt(keyID string, ciphertext []byte) ([]byte, error)
	
	// Sign signs data using HSM
	Sign(keyID string, data []byte) ([]byte, error)
	
	// Verify verifies signature using HSM
	Verify(keyID string, data []byte, signature []byte) error
}

// TODO: Implement actual HSM provider (e.g., AWS CloudHSM, Azure Key Vault, etc.)
