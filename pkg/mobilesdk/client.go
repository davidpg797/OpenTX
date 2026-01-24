package mobilesdk

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Mobile SDK Client for iOS and Android
// Provides simplified payment processing for mobile apps

// Client represents mobile SDK client
type Client struct {
	config     Config
	httpClient *http.Client
	session    *Session
	cache      *Cache
	logger     Logger
	mu         sync.RWMutex
}

// Config holds SDK configuration
type Config struct {
	APIKey          string
	Environment     Environment
	BaseURL         string
	Timeout         time.Duration
	MaxRetries      int
	EnableLogging   bool
	EnableCaching   bool
	CacheTTL        time.Duration
	TLSConfig       *tls.Config
	CustomHeaders   map[string]string
}

// Environment represents deployment environment
type Environment string

const (
	EnvironmentProduction Environment = "production"
	EnvironmentSandbox    Environment = "sandbox"
	EnvironmentDevelopment Environment = "development"
)

// Session represents user session
type Session struct {
	SessionID     string
	UserID        string
	DeviceID      string
	AccessToken   string
	RefreshToken  string
	ExpiresAt     time.Time
	CreatedAt     time.Time
	LastActivity  time.Time
	DeviceInfo    DeviceInfo
}

// DeviceInfo contains device information
type DeviceInfo struct {
	Platform      string // iOS, Android
	OSVersion     string
	AppVersion    string
	DeviceModel   string
	DeviceName    string
	Language      string
	Timezone      string
	PushToken     string
}

// Logger interface for SDK logging
type Logger interface {
	Debug(msg string, fields map[string]interface{})
	Info(msg string, fields map[string]interface{})
	Warn(msg string, fields map[string]interface{})
	Error(msg string, fields map[string]interface{})
}

// Cache interface for SDK caching
type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{}, ttl time.Duration)
	Delete(key string)
	Clear()
}

// PaymentRequest represents a payment request
type PaymentRequest struct {
	Amount          int64             `json:"amount"`
	Currency        string            `json:"currency"`
	Description     string            `json:"description,omitempty"`
	CustomerID      string            `json:"customer_id,omitempty"`
	PaymentMethod   PaymentMethodType `json:"payment_method"`
	Card            *CardInfo         `json:"card,omitempty"`
	BillingAddress  *Address          `json:"billing_address,omitempty"`
	ShippingAddress *Address          `json:"shipping_address,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	IdempotencyKey  string            `json:"idempotency_key,omitempty"`
	ThreeDSecure    bool              `json:"three_d_secure,omitempty"`
}

// PaymentMethodType represents payment method
type PaymentMethodType string

const (
	PaymentMethodCard       PaymentMethodType = "card"
	PaymentMethodApplePay   PaymentMethodType = "apple_pay"
	PaymentMethodGooglePay  PaymentMethodType = "google_pay"
	PaymentMethodBank       PaymentMethodType = "bank"
	PaymentMethodWallet     PaymentMethodType = "wallet"
)

// CardInfo contains card information
type CardInfo struct {
	Number          string `json:"number"`
	ExpiryMonth     int    `json:"expiry_month"`
	ExpiryYear      int    `json:"expiry_year"`
	CVV             string `json:"cvv"`
	CardholderName  string `json:"cardholder_name"`
	SaveCard        bool   `json:"save_card,omitempty"`
}

// Address represents billing/shipping address
type Address struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state,omitempty"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

// PaymentResponse represents payment response
type PaymentResponse struct {
	ID              string            `json:"id"`
	Status          PaymentStatus     `json:"status"`
	Amount          int64             `json:"amount"`
	Currency        string            `json:"currency"`
	Description     string            `json:"description,omitempty"`
	CustomerID      string            `json:"customer_id,omitempty"`
	PaymentMethod   PaymentMethodType `json:"payment_method"`
	Card            *CardResponse     `json:"card,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	ErrorCode       string            `json:"error_code,omitempty"`
	ErrorMessage    string            `json:"error_message,omitempty"`
	ThreeDSRequired bool              `json:"three_ds_required,omitempty"`
	ThreeDSURL      string            `json:"three_ds_url,omitempty"`
}

// PaymentStatus represents payment status
type PaymentStatus string

const (
	StatusPending   PaymentStatus = "pending"
	StatusSucceeded PaymentStatus = "succeeded"
	StatusFailed    PaymentStatus = "failed"
	StatusCanceled  PaymentStatus = "canceled"
	StatusRequiresAction PaymentStatus = "requires_action"
)

// CardResponse contains card response data
type CardResponse struct {
	Last4          string `json:"last4"`
	Brand          string `json:"brand"`
	ExpiryMonth    int    `json:"expiry_month"`
	ExpiryYear     int    `json:"expiry_year"`
	CardholderName string `json:"cardholder_name,omitempty"`
	Fingerprint    string `json:"fingerprint,omitempty"`
}

// RefundRequest represents refund request
type RefundRequest struct {
	PaymentID      string            `json:"payment_id"`
	Amount         int64             `json:"amount,omitempty"` // Partial refund if specified
	Reason         string            `json:"reason,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
}

// RefundResponse represents refund response
type RefundResponse struct {
	ID         string            `json:"id"`
	PaymentID  string            `json:"payment_id"`
	Amount     int64             `json:"amount"`
	Currency   string            `json:"currency"`
	Status     string            `json:"status"`
	Reason     string            `json:"reason,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// TokenRequest represents tokenization request
type TokenRequest struct {
	Card *CardInfo `json:"card"`
}

// TokenResponse represents tokenization response
type TokenResponse struct {
	ID        string        `json:"id"`
	Type      string        `json:"type"`
	Card      *CardResponse `json:"card"`
	CreatedAt time.Time     `json:"created_at"`
}

// NewClient creates a new mobile SDK client
func NewClient(config Config) (*Client, error) {
	if config.APIKey == "" {
		return nil, errors.New("API key is required")
	}

	if config.BaseURL == "" {
		switch config.Environment {
		case EnvironmentProduction:
			config.BaseURL = "https://api.opentx.io/v1"
		case EnvironmentSandbox:
			config.BaseURL = "https://sandbox.opentx.io/v1"
		default:
			config.BaseURL = "https://dev.opentx.io/v1"
		}
	}

	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	client := &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
			Transport: &http.Transport{
				TLSClientConfig: config.TLSConfig,
			},
		},
	}

	if config.EnableCaching {
		client.cache = NewMemoryCache()
	}

	return client, nil
}

// SetSession sets the current session
func (c *Client) SetSession(session *Session) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = session
}

// GetSession returns the current session
func (c *Client) GetSession() *Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}

// CreatePayment creates a new payment
func (c *Client) CreatePayment(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error) {
	if req == nil {
		return nil, errors.New("payment request is nil")
	}

	// Validate request
	if err := c.validatePaymentRequest(req); err != nil {
		return nil, err
	}

	// Check cache
	if c.config.EnableCaching && req.IdempotencyKey != "" {
		if cached, ok := c.cache.Get("payment:" + req.IdempotencyKey); ok {
			return cached.(*PaymentResponse), nil
		}
	}

	var resp PaymentResponse
	if err := c.doRequest(ctx, "POST", "/payments", req, &resp); err != nil {
		return nil, err
	}

	// Cache response
	if c.config.EnableCaching && req.IdempotencyKey != "" {
		c.cache.Set("payment:"+req.IdempotencyKey, &resp, c.config.CacheTTL)
	}

	return &resp, nil
}

// GetPayment retrieves payment by ID
func (c *Client) GetPayment(ctx context.Context, paymentID string) (*PaymentResponse, error) {
	if paymentID == "" {
		return nil, errors.New("payment ID is required")
	}

	// Check cache
	if c.config.EnableCaching {
		if cached, ok := c.cache.Get("payment:" + paymentID); ok {
			return cached.(*PaymentResponse), nil
		}
	}

	var resp PaymentResponse
	if err := c.doRequest(ctx, "GET", "/payments/"+paymentID, nil, &resp); err != nil {
		return nil, err
	}

	// Cache response
	if c.config.EnableCaching {
		c.cache.Set("payment:"+paymentID, &resp, c.config.CacheTTL)
	}

	return &resp, nil
}

// RefundPayment refunds a payment
func (c *Client) RefundPayment(ctx context.Context, req *RefundRequest) (*RefundResponse, error) {
	if req == nil || req.PaymentID == "" {
		return nil, errors.New("payment ID is required")
	}

	var resp RefundResponse
	if err := c.doRequest(ctx, "POST", "/refunds", req, &resp); err != nil {
		return nil, err
	}

	// Invalidate payment cache
	if c.config.EnableCaching {
		c.cache.Delete("payment:" + req.PaymentID)
	}

	return &resp, nil
}

// TokenizeCard tokenizes card information
func (c *Client) TokenizeCard(ctx context.Context, req *TokenRequest) (*TokenResponse, error) {
	if req == nil || req.Card == nil {
		return nil, errors.New("card information is required")
	}

	var resp TokenResponse
	if err := c.doRequest(ctx, "POST", "/tokens", req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// doRequest performs HTTP request with retry
func (c *Client) doRequest(ctx context.Context, method, path string, body, result interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		err := c.doSingleRequest(ctx, method, path, body, result)
		if err == nil {
			return nil
		}

		lastErr = err

		// Don't retry on client errors (4xx)
		if strings.Contains(err.Error(), "status code: 4") {
			break
		}
	}

	return lastErr
}

// doSingleRequest performs a single HTTP request
func (c *Client) doSingleRequest(ctx context.Context, method, path string, body, result interface{}) error {
	url := c.config.BaseURL + path

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = strings.NewReader(string(data))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "OpenTX-Mobile-SDK/1.0")

	// Add session token if available
	c.mu.RLock()
	if c.session != nil && c.session.AccessToken != "" {
		req.Header.Set("X-Session-Token", c.session.AccessToken)
	}
	c.mu.RUnlock()

	// Add custom headers
	for key, value := range c.config.CustomHeaders {
		req.Header.Set(key, value)
	}

	// Perform request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("request failed with status code: %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// validatePaymentRequest validates payment request
func (c *Client) validatePaymentRequest(req *PaymentRequest) error {
	if req.Amount <= 0 {
		return errors.New("amount must be greater than 0")
	}

	if req.Currency == "" {
		return errors.New("currency is required")
	}

	if req.PaymentMethod == "" {
		return errors.New("payment method is required")
	}

	if req.PaymentMethod == PaymentMethodCard && req.Card == nil {
		return errors.New("card information is required for card payments")
	}

	if req.Card != nil {
		if err := c.validateCard(req.Card); err != nil {
			return err
		}
	}

	return nil
}

// validateCard validates card information
func (c *Client) validateCard(card *CardInfo) error {
	if card.Number == "" {
		return errors.New("card number is required")
	}

	if card.ExpiryMonth < 1 || card.ExpiryMonth > 12 {
		return errors.New("invalid expiry month")
	}

	currentYear := time.Now().Year()
	if card.ExpiryYear < currentYear {
		return errors.New("card is expired")
	}

	if card.CVV == "" {
		return errors.New("CVV is required")
	}

	return nil
}

// MemoryCache is a simple in-memory cache
type MemoryCache struct {
	items map[string]*cacheItem
	mu    sync.RWMutex
}

type cacheItem struct {
	value     interface{}
	expiresAt time.Time
}

// NewMemoryCache creates a new memory cache
func NewMemoryCache() *Cache {
	cache := &MemoryCache{
		items: make(map[string]*cacheItem),
	}

	// Start cleanup goroutine
	go cache.cleanup()

	var c Cache = cache
	return &c
}

func (mc *MemoryCache) Get(key string) (interface{}, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	item, exists := mc.items[key]
	if !exists {
		return nil, false
	}

	if time.Now().After(item.expiresAt) {
		return nil, false
	}

	return item.value, true
}

func (mc *MemoryCache) Set(key string, value interface{}, ttl time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.items[key] = &cacheItem{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
}

func (mc *MemoryCache) Delete(key string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	delete(mc.items, key)
}

func (mc *MemoryCache) Clear() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.items = make(map[string]*cacheItem)
}

func (mc *MemoryCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		mc.mu.Lock()
		now := time.Now()
		for key, item := range mc.items {
			if now.After(item.expiresAt) {
				delete(mc.items, key)
			}
		}
		mc.mu.Unlock()
	}
}

// Helper methods for mobile platforms

// ApplePayRequest represents Apple Pay request
type ApplePayRequest struct {
	PaymentToken string `json:"payment_token"`
	Amount       int64  `json:"amount"`
	Currency     string `json:"currency"`
}

// GooglePayRequest represents Google Pay request
type GooglePayRequest struct {
	PaymentToken string `json:"payment_token"`
	Amount       int64  `json:"amount"`
	Currency     string `json:"currency"`
}

// ProcessApplePay processes Apple Pay payment
func (c *Client) ProcessApplePay(ctx context.Context, req *ApplePayRequest) (*PaymentResponse, error) {
	paymentReq := &PaymentRequest{
		Amount:        req.Amount,
		Currency:      req.Currency,
		PaymentMethod: PaymentMethodApplePay,
		Metadata: map[string]string{
			"payment_token": req.PaymentToken,
		},
	}

	return c.CreatePayment(ctx, paymentReq)
}

// ProcessGooglePay processes Google Pay payment
func (c *Client) ProcessGooglePay(ctx context.Context, req *GooglePayRequest) (*PaymentResponse, error) {
	paymentReq := &PaymentRequest{
		Amount:        req.Amount,
		Currency:      req.Currency,
		PaymentMethod: PaymentMethodGooglePay,
		Metadata: map[string]string{
			"payment_token": req.PaymentToken,
		},
	}

	return c.CreatePayment(ctx, paymentReq)
}
