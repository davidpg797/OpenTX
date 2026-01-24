package hsm

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"sync"
	"time"
)

// HSM Vault Integration for Hardware Security Module
// Provides secure key management and cryptographic operations

// HSMProvider defines HSM interface
type HSMProvider interface {
	// Key management
	GenerateKey(ctx interface{}, keyType KeyType, keySize int) (string, error)
	ImportKey(ctx interface{}, keyData []byte, keyType KeyType) (string, error)
	DeleteKey(ctx interface{}, keyID string) error
	GetKeyMetadata(ctx interface{}, keyID string) (*KeyMetadata, error)
	
	// Cryptographic operations
	Encrypt(ctx interface{}, keyID string, plaintext []byte) ([]byte, error)
	Decrypt(ctx interface{}, keyID string, ciphertext []byte) ([]byte, error)
	Sign(ctx interface{}, keyID string, data []byte) ([]byte, error)
	Verify(ctx interface{}, keyID string, data []byte, signature []byte) error
	
	// Key rotation
	RotateKey(ctx interface{}, keyID string) (string, error)
}

// KeyType represents type of cryptographic key
type KeyType string

const (
	KeyTypeRSA2048   KeyType = "RSA-2048"
	KeyTypeRSA4096   KeyType = "RSA-4096"
	KeyTypeAES256    KeyType = "AES-256"
	KeyTypeECDSAP256 KeyType = "ECDSA-P256"
	KeyTypeECDSAP384 KeyType = "ECDSA-P384"
)

// KeyUsage represents key usage
type KeyUsage string

const (
	KeyUsageEncrypt KeyUsage = "encrypt"
	KeyUsageDecrypt KeyUsage = "decrypt"
	KeyUsageSign    KeyUsage = "sign"
	KeyUsageVerify  KeyUsage = "verify"
	KeyUsageWrap    KeyUsage = "wrap"
	KeyUsageUnwrap  KeyUsage = "unwrap"
)

// KeyMetadata holds key metadata
type KeyMetadata struct {
	KeyID       string
	KeyType     KeyType
	Usage       []KeyUsage
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ExpiresAt   *time.Time
	Version     int
	Active      bool
	Description string
	Tags        map[string]string
}

// KeyStore manages cryptographic keys
type KeyStore struct {
	keys     map[string]*KeyEntry
	provider HSMProvider
	mu       sync.RWMutex
}

// KeyEntry represents a stored key
type KeyEntry struct {
	Metadata   *KeyMetadata
	PrivateKey interface{}
	PublicKey  interface{}
}

// NewKeyStore creates a new key store
func NewKeyStore(provider HSMProvider) *KeyStore {
	return &KeyStore{
		keys:     make(map[string]*KeyEntry),
		provider: provider,
	}
}

// GenerateKey generates a new cryptographic key
func (ks *KeyStore) GenerateKey(keyType KeyType, usage []KeyUsage, description string) (*KeyMetadata, error) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	// Determine key size
	keySize := 2048
	if keyType == KeyTypeRSA4096 {
		keySize = 4096
	}

	// Generate key using HSM
	keyID, err := ks.provider.GenerateKey(nil, keyType, keySize)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	// Create metadata
	metadata := &KeyMetadata{
		KeyID:       keyID,
		KeyType:     keyType,
		Usage:       usage,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Version:     1,
		Active:      true,
		Description: description,
		Tags:        make(map[string]string),
	}

	// Store key entry
	ks.keys[keyID] = &KeyEntry{
		Metadata: metadata,
	}

	return metadata, nil
}

// GetKey retrieves a key by ID
func (ks *KeyStore) GetKey(keyID string) (*KeyEntry, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	entry, exists := ks.keys[keyID]
	if !exists {
		return nil, fmt.Errorf("key %s not found", keyID)
	}

	if !entry.Metadata.Active {
		return nil, fmt.Errorf("key %s is inactive", keyID)
	}

	return entry, nil
}

// RotateKey rotates a cryptographic key
func (ks *KeyStore) RotateKey(keyID string) (*KeyMetadata, error) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	entry, exists := ks.keys[keyID]
	if !exists {
		return nil, fmt.Errorf("key %s not found", keyID)
	}

	// Rotate key using HSM
	newKeyID, err := ks.provider.RotateKey(nil, keyID)
	if err != nil {
		return nil, fmt.Errorf("failed to rotate key: %w", err)
	}

	// Mark old key as inactive
	entry.Metadata.Active = false

	// Create new key metadata
	newMetadata := &KeyMetadata{
		KeyID:       newKeyID,
		KeyType:     entry.Metadata.KeyType,
		Usage:       entry.Metadata.Usage,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Version:     entry.Metadata.Version + 1,
		Active:      true,
		Description: entry.Metadata.Description,
		Tags:        entry.Metadata.Tags,
	}

	// Store new key entry
	ks.keys[newKeyID] = &KeyEntry{
		Metadata: newMetadata,
	}

	return newMetadata, nil
}

// DeleteKey deletes a key
func (ks *KeyStore) DeleteKey(keyID string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if err := ks.provider.DeleteKey(nil, keyID); err != nil {
		return err
	}

	delete(ks.keys, keyID)
	return nil
}

// Encrypt encrypts data using a key
func (ks *KeyStore) Encrypt(keyID string, plaintext []byte) ([]byte, error) {
	entry, err := ks.GetKey(keyID)
	if err != nil {
		return nil, err
	}

	// Check key usage
	if !hasUsage(entry.Metadata.Usage, KeyUsageEncrypt) {
		return nil, errors.New("key does not have encrypt usage")
	}

	return ks.provider.Encrypt(nil, keyID, plaintext)
}

// Decrypt decrypts data using a key
func (ks *KeyStore) Decrypt(keyID string, ciphertext []byte) ([]byte, error) {
	entry, err := ks.GetKey(keyID)
	if err != nil {
		return nil, err
	}

	// Check key usage
	if !hasUsage(entry.Metadata.Usage, KeyUsageDecrypt) {
		return nil, errors.New("key does not have decrypt usage")
	}

	return ks.provider.Decrypt(nil, keyID, ciphertext)
}

// Sign signs data using a key
func (ks *KeyStore) Sign(keyID string, data []byte) ([]byte, error) {
	entry, err := ks.GetKey(keyID)
	if err != nil {
		return nil, err
	}

	// Check key usage
	if !hasUsage(entry.Metadata.Usage, KeyUsageSign) {
		return nil, errors.New("key does not have sign usage")
	}

	return ks.provider.Sign(nil, keyID, data)
}

// Verify verifies a signature using a key
func (ks *KeyStore) Verify(keyID string, data []byte, signature []byte) error {
	entry, err := ks.GetKey(keyID)
	if err != nil {
		return err
	}

	// Check key usage
	if !hasUsage(entry.Metadata.Usage, KeyUsageVerify) {
		return errors.New("key does not have verify usage")
	}

	return ks.provider.Verify(nil, keyID, data, signature)
}

// hasUsage checks if key has specific usage
func hasUsage(usages []KeyUsage, usage KeyUsage) bool {
	for _, u := range usages {
		if u == usage {
			return true
		}
	}
	return false
}

// SoftHSM is a software-based HSM implementation for testing
type SoftHSM struct {
	keys map[string]*softKey
	mu   sync.RWMutex
}

type softKey struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	keyType    KeyType
	createdAt  time.Time
}

// NewSoftHSM creates a new software HSM
func NewSoftHSM() *SoftHSM {
	return &SoftHSM{
		keys: make(map[string]*softKey),
	}
}

// GenerateKey generates a key in software HSM
func (s *SoftHSM) GenerateKey(ctx interface{}, keyType KeyType, keySize int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return "", err
	}

	// Generate key ID
	keyID := fmt.Sprintf("soft-key-%d", time.Now().UnixNano())

	// Store key
	s.keys[keyID] = &softKey{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		keyType:    keyType,
		createdAt:  time.Now(),
	}

	return keyID, nil
}

// ImportKey imports a key into software HSM
func (s *SoftHSM) ImportKey(ctx interface{}, keyData []byte, keyType KeyType) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Parse PEM
	block, _ := pem.Decode(keyData)
	if block == nil {
		return "", errors.New("failed to decode PEM")
	}

	// Parse private key
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}

	// Generate key ID
	keyID := fmt.Sprintf("soft-key-%d", time.Now().UnixNano())

	// Store key
	s.keys[keyID] = &softKey{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		keyType:    keyType,
		createdAt:  time.Now(),
	}

	return keyID, nil
}

// DeleteKey deletes a key from software HSM
func (s *SoftHSM) DeleteKey(ctx interface{}, keyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.keys, keyID)
	return nil
}

// GetKeyMetadata returns key metadata
func (s *SoftHSM) GetKeyMetadata(ctx interface{}, keyID string) (*KeyMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, exists := s.keys[keyID]
	if !exists {
		return nil, fmt.Errorf("key %s not found", keyID)
	}

	return &KeyMetadata{
		KeyID:     keyID,
		KeyType:   key.keyType,
		CreatedAt: key.createdAt,
		Active:    true,
		Version:   1,
	}, nil
}

// Encrypt encrypts data using RSA
func (s *SoftHSM) Encrypt(ctx interface{}, keyID string, plaintext []byte) ([]byte, error) {
	s.mu.RLock()
	key, exists := s.keys[keyID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("key %s not found", keyID)
	}

	// Encrypt using RSA-OAEP
	ciphertext, err := rsa.EncryptOAEP(
		sha256.New(),
		rand.Reader,
		key.publicKey,
		plaintext,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return ciphertext, nil
}

// Decrypt decrypts data using RSA
func (s *SoftHSM) Decrypt(ctx interface{}, keyID string, ciphertext []byte) ([]byte, error) {
	s.mu.RLock()
	key, exists := s.keys[keyID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("key %s not found", keyID)
	}

	// Decrypt using RSA-OAEP
	plaintext, err := rsa.DecryptOAEP(
		sha256.New(),
		rand.Reader,
		key.privateKey,
		ciphertext,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// Sign signs data using RSA-PSS
func (s *SoftHSM) Sign(ctx interface{}, keyID string, data []byte) ([]byte, error) {
	s.mu.RLock()
	key, exists := s.keys[keyID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("key %s not found", keyID)
	}

	// Hash data
	hashed := sha256.Sum256(data)

	// Sign using RSA-PSS
	signature, err := rsa.SignPSS(
		rand.Reader,
		key.privateKey,
		crypto.SHA256,
		hashed[:],
		nil,
	)
	if err != nil {
		return nil, err
	}

	return signature, nil
}

// Verify verifies signature using RSA-PSS
func (s *SoftHSM) Verify(ctx interface{}, keyID string, data []byte, signature []byte) error {
	s.mu.RLock()
	key, exists := s.keys[keyID]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("key %s not found", keyID)
	}

	// Hash data
	hashed := sha256.Sum256(data)

	// Verify using RSA-PSS
	err := rsa.VerifyPSS(
		key.publicKey,
		crypto.SHA256,
		hashed[:],
		signature,
		nil,
	)

	return err
}

// RotateKey rotates a key in software HSM
func (s *SoftHSM) RotateKey(ctx interface{}, keyID string) (string, error) {
	s.mu.RLock()
	key, exists := s.keys[keyID]
	s.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("key %s not found", keyID)
	}

	// Generate new key with same type
	keySize := key.privateKey.N.BitLen()
	return s.GenerateKey(ctx, key.keyType, keySize)
}

// KeyManager manages HSM operations
type KeyManager struct {
	store      *KeyStore
	autoRotate bool
	rotationPeriod time.Duration
	stopChan   chan struct{}
}

// NewKeyManager creates a new key manager
func NewKeyManager(provider HSMProvider, autoRotate bool, rotationPeriod time.Duration) *KeyManager {
	return &KeyManager{
		store:          NewKeyStore(provider),
		autoRotate:     autoRotate,
		rotationPeriod: rotationPeriod,
		stopChan:       make(chan struct{}),
	}
}

// Start starts key manager
func (km *KeyManager) Start() {
	if km.autoRotate {
		go km.autoRotateKeys()
	}
}

// Stop stops key manager
func (km *KeyManager) Stop() {
	close(km.stopChan)
}

// autoRotateKeys automatically rotates keys
func (km *KeyManager) autoRotateKeys() {
	ticker := time.NewTicker(km.rotationPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			km.rotateExpiredKeys()
		case <-km.stopChan:
			return
		}
	}
}

// rotateExpiredKeys rotates keys that are due for rotation
func (km *KeyManager) rotateExpiredKeys() {
	km.store.mu.RLock()
	keysToRotate := make([]string, 0)
	
	for keyID, entry := range km.store.keys {
		if entry.Metadata.Active {
			age := time.Since(entry.Metadata.CreatedAt)
			if age >= km.rotationPeriod {
				keysToRotate = append(keysToRotate, keyID)
			}
		}
	}
	km.store.mu.RUnlock()

	// Rotate keys
	for _, keyID := range keysToRotate {
		if _, err := km.store.RotateKey(keyID); err != nil {
			// Log error
		}
	}
}

// GetStore returns the key store
func (km *KeyManager) GetStore() *KeyStore {
	return km.store
}

// EncryptionService provides high-level encryption service
type EncryptionService struct {
	keyManager *KeyManager
	defaultKey string
}

// NewEncryptionService creates a new encryption service
func NewEncryptionService(keyManager *KeyManager, defaultKey string) *EncryptionService {
	return &EncryptionService{
		keyManager: keyManager,
		defaultKey: defaultKey,
	}
}

// EncryptData encrypts data using default key
func (es *EncryptionService) EncryptData(data []byte) ([]byte, error) {
	return es.keyManager.store.Encrypt(es.defaultKey, data)
}

// DecryptData decrypts data using default key
func (es *EncryptionService) DecryptData(ciphertext []byte) ([]byte, error) {
	return es.keyManager.store.Decrypt(es.defaultKey, ciphertext)
}

// SignData signs data using default key
func (es *EncryptionService) SignData(data []byte) ([]byte, error) {
	return es.keyManager.store.Sign(es.defaultKey, data)
}

// VerifyData verifies signature using default key
func (es *EncryptionService) VerifyData(data []byte, signature []byte) error {
	return es.keyManager.store.Verify(es.defaultKey, data, signature)
}
