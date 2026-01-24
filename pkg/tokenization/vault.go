package tokenization

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrTokenExpired     = errors.New("token expired")
	ErrTokenNotFound    = errors.New("token not found")
	ErrDetokenizeFailed = errors.New("detokenization failed")
)

// Token represents a tokenized value
type Token struct {
	ID             string
	Token          string
	OriginalValue  string
	Type           string // card, account, etc.
	CreatedAt      time.Time
	ExpiresAt      time.Time
	UsageCount     int
	MaxUsageCount  int
	Metadata       map[string]string
}

// TokenVault manages tokens
type TokenVault struct {
	mu            sync.RWMutex
	tokens        map[string]*Token
	reverseIndex  map[string]string // token -> id
	encryptionKey []byte
	logger        *zap.Logger
	
	// Configuration
	defaultTTL    time.Duration
	tokenLength   int
}

// Config configures token vault
type Config struct {
	EncryptionKey string
	DefaultTTL    time.Duration
	TokenLength   int
}

// NewTokenVault creates a new token vault
func NewTokenVault(config Config, logger *zap.Logger) (*TokenVault, error) {
	if config.EncryptionKey == "" {
		return nil, errors.New("encryption key is required")
	}

	// Derive 32-byte key from config key
	hash := sha256.Sum256([]byte(config.EncryptionKey))
	encryptionKey := hash[:]

	if config.DefaultTTL == 0 {
		config.DefaultTTL = 365 * 24 * time.Hour // 1 year
	}

	if config.TokenLength == 0 {
		config.TokenLength = 16
	}

	tv := &TokenVault{
		tokens:        make(map[string]*Token),
		reverseIndex:  make(map[string]string),
		encryptionKey: encryptionKey,
		logger:        logger,
		defaultTTL:    config.DefaultTTL,
		tokenLength:   config.TokenLength,
	}

	// Start cleanup goroutine
	go tv.cleanupLoop()

	return tv, nil
}

// Tokenize creates a token for a sensitive value
func (tv *TokenVault) Tokenize(value string, tokenType string, ttl time.Duration, maxUsage int) (*Token, error) {
	if value == "" {
		return nil, errors.New("value cannot be empty")
	}

	// Check if already tokenized
	tv.mu.RLock()
	for _, token := range tv.tokens {
		if token.OriginalValue == value && token.Type == tokenType {
			tv.mu.RUnlock()
			return token, nil
		}
	}
	tv.mu.RUnlock()

	// Generate token
	tokenStr := tv.generateToken()

	// Encrypt original value
	encrypted, err := tv.encrypt(value)
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %w", err)
	}

	if ttl == 0 {
		ttl = tv.defaultTTL
	}

	token := &Token{
		ID:            generateID(),
		Token:         tokenStr,
		OriginalValue: encrypted,
		Type:          tokenType,
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(ttl),
		UsageCount:    0,
		MaxUsageCount: maxUsage,
		Metadata:      make(map[string]string),
	}

	// Store token
	tv.mu.Lock()
	tv.tokens[token.ID] = token
	tv.reverseIndex[token.Token] = token.ID
	tv.mu.Unlock()

	tv.logger.Info("token created",
		zap.String("token_id", token.ID),
		zap.String("type", tokenType),
		zap.Duration("ttl", ttl))

	return token, nil
}

// Detokenize retrieves original value from token
func (tv *TokenVault) Detokenize(tokenStr string) (string, error) {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	// Find token
	tokenID, exists := tv.reverseIndex[tokenStr]
	if !exists {
		return "", ErrTokenNotFound
	}

	token := tv.tokens[tokenID]

	// Check expiration
	if time.Now().After(token.ExpiresAt) {
		delete(tv.tokens, tokenID)
		delete(tv.reverseIndex, tokenStr)
		return "", ErrTokenExpired
	}

	// Check usage limit
	if token.MaxUsageCount > 0 && token.UsageCount >= token.MaxUsageCount {
		delete(tv.tokens, tokenID)
		delete(tv.reverseIndex, tokenStr)
		return "", errors.New("token usage limit exceeded")
	}

	// Increment usage
	token.UsageCount++

	// Decrypt value
	decrypted, err := tv.decrypt(token.OriginalValue)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDetokenizeFailed, err)
	}

	tv.logger.Debug("token detokenized",
		zap.String("token_id", token.ID),
		zap.Int("usage_count", token.UsageCount))

	return decrypted, nil
}

// Delete deletes a token
func (tv *TokenVault) Delete(tokenStr string) error {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	tokenID, exists := tv.reverseIndex[tokenStr]
	if !exists {
		return ErrTokenNotFound
	}

	delete(tv.tokens, tokenID)
	delete(tv.reverseIndex, tokenStr)

	tv.logger.Info("token deleted", zap.String("token_id", tokenID))
	return nil
}

// GetToken retrieves token info (without original value)
func (tv *TokenVault) GetToken(tokenStr string) (*Token, error) {
	tv.mu.RLock()
	defer tv.mu.RUnlock()

	tokenID, exists := tv.reverseIndex[tokenStr]
	if !exists {
		return nil, ErrTokenNotFound
	}

	token := tv.tokens[tokenID]

	// Check expiration
	if time.Now().After(token.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	// Return copy without original value
	return &Token{
		ID:            token.ID,
		Token:         token.Token,
		Type:          token.Type,
		CreatedAt:     token.CreatedAt,
		ExpiresAt:     token.ExpiresAt,
		UsageCount:    token.UsageCount,
		MaxUsageCount: token.MaxUsageCount,
		Metadata:      token.Metadata,
	}, nil
}

// UpdateMetadata updates token metadata
func (tv *TokenVault) UpdateMetadata(tokenStr string, metadata map[string]string) error {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	tokenID, exists := tv.reverseIndex[tokenStr]
	if !exists {
		return ErrTokenNotFound
	}

	token := tv.tokens[tokenID]
	for k, v := range metadata {
		token.Metadata[k] = v
	}

	return nil
}

func (tv *TokenVault) generateToken() string {
	// Generate random token
	bytes := make([]byte, tv.tokenLength)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to time-based token
		return fmt.Sprintf("tok_%d", time.Now().UnixNano())
	}
	return "tok_" + base64.URLEncoding.EncodeToString(bytes)[:tv.tokenLength]
}

func (tv *TokenVault) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(tv.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (tv *TokenVault) decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(tv.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func (tv *TokenVault) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		tv.cleanup()
	}
}

func (tv *TokenVault) cleanup() {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	now := time.Now()
	var toDelete []string

	for id, token := range tv.tokens {
		if now.After(token.ExpiresAt) {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		token := tv.tokens[id]
		delete(tv.reverseIndex, token.Token)
		delete(tv.tokens, id)
	}

	if len(toDelete) > 0 {
		tv.logger.Info("cleaned up expired tokens", zap.Int("count", len(toDelete)))
	}
}

// Stats returns token vault statistics
func (tv *TokenVault) Stats() map[string]interface{} {
	tv.mu.RLock()
	defer tv.mu.RUnlock()

	typeCount := make(map[string]int)
	var expiredCount int

	now := time.Now()
	for _, token := range tv.tokens {
		typeCount[token.Type]++
		if now.After(token.ExpiresAt) {
			expiredCount++
		}
	}

	return map[string]interface{}{
		"total_tokens":   len(tv.tokens),
		"expired_tokens": expiredCount,
		"by_type":        typeCount,
	}
}

// CardTokenizer provides card-specific tokenization
type CardTokenizer struct {
	vault  *TokenVault
	logger *zap.Logger
}

// NewCardTokenizer creates card tokenizer
func NewCardTokenizer(vault *TokenVault, logger *zap.Logger) *CardTokenizer {
	return &CardTokenizer{
		vault:  vault,
		logger: logger,
	}
}

// TokenizeCard tokenizes a card number
func (ct *CardTokenizer) TokenizeCard(cardNumber string, ttl time.Duration) (*Token, error) {
	// Validate card number (basic check)
	if len(cardNumber) < 13 || len(cardNumber) > 19 {
		return nil, errors.New("invalid card number length")
	}

	return ct.vault.Tokenize(cardNumber, "card", ttl, 0)
}

// DetokenizeCard retrieves card number from token
func (ct *CardTokenizer) DetokenizeCard(token string) (string, error) {
	return ct.vault.Detokenize(token)
}

// TokenizePAN tokenizes PAN with BIN preservation
func (ct *CardTokenizer) TokenizePAN(pan string) (string, error) {
	if len(pan) < 16 {
		return "", errors.New("invalid PAN length")
	}

	// Create token preserving first 6 (BIN) and last 4 digits
	token, err := ct.vault.Tokenize(pan, "card", 0, 0)
	if err != nil {
		return "", err
	}

	// Format: BIN + token + last4
	bin := pan[:6]
	last4 := pan[len(pan)-4:]
	
	// Get first 6 chars of token
	tokenPart := token.Token
	if len(tokenPart) > 6 {
		tokenPart = tokenPart[:6]
	}

	preservedToken := bin + tokenPart + last4

	// Store mapping
	ct.vault.UpdateMetadata(token.Token, map[string]string{
		"preserved_token": preservedToken,
	})

	return preservedToken, nil
}

func generateID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// Format-Preserving Encryption (basic implementation)
type FPETokenizer struct {
	vault  *TokenVault
	logger *zap.Logger
}

// NewFPETokenizer creates FPE tokenizer
func NewFPETokenizer(vault *TokenVault, logger *zap.Logger) *FPETokenizer {
	return &FPETokenizer{
		vault:  vault,
		logger: logger,
	}
}

// Tokenize performs format-preserving tokenization
func (fpe *FPETokenizer) Tokenize(value string, preserveFormat bool) (string, error) {
	token, err := fpe.vault.Tokenize(value, "fpe", 0, 0)
	if err != nil {
		return "", err
	}

	if !preserveFormat {
		return token.Token, nil
	}

	// Generate token matching original format (digits only for cards)
	result := ""
	tokenBytes := []byte(token.Token)
	tokenIdx := 0

	for _, c := range value {
		if c >= '0' && c <= '9' {
			// Replace with random digit
			if tokenIdx < len(tokenBytes) {
				result += fmt.Sprintf("%d", tokenBytes[tokenIdx]%10)
				tokenIdx++
			} else {
				result += fmt.Sprintf("%d", time.Now().UnixNano()%10)
			}
		} else {
			result += string(c)
		}
	}

	// Store mapping
	fpe.vault.UpdateMetadata(token.Token, map[string]string{
		"fpe_token": result,
	})

	return result, nil
}
