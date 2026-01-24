package threeds

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 3D Secure 2.0 Authentication Implementation
// EMV 3DS 2.2.0 Specification Compliant

// Version represents 3DS protocol version
type Version string

const (
	Version200 Version = "2.0.0"
	Version210 Version = "2.1.0"
	Version220 Version = "2.2.0"
)

// MessageType represents 3DS message types
type MessageType string

const (
	MessageTypeAReq  MessageType = "AReq"  // Authentication Request
	MessageTypeARes  MessageType = "ARes"  // Authentication Response
	MessageTypeCReq  MessageType = "CReq"  // Challenge Request
	MessageTypeCRes  MessageType = "CRes"  // Challenge Response
	MessageTypeRReq  MessageType = "RReq"  // Results Request
	MessageTypeRRes  MessageType = "RRes"  // Results Response
	MessageTypePReq  MessageType = "PReq"  // Preparation Request
	MessageTypePRes  MessageType = "PRes"  // Preparation Response
	MessageTypeErro  MessageType = "Erro"  // Error Message
)

// TransactionStatus represents authentication status
type TransactionStatus string

const (
	StatusAuthenticated      TransactionStatus = "Y" // Authenticated
	StatusNotAuthenticated   TransactionStatus = "N" // Not Authenticated
	StatusAttempted          TransactionStatus = "A" // Authentication Attempted
	StatusChallengeRequired  TransactionStatus = "C" // Challenge Required
	StatusRejected           TransactionStatus = "R" // Rejected
	StatusUnavailable        TransactionStatus = "U" // Unavailable
)

// AuthenticationRequest represents 3DS authentication request
type AuthenticationRequest struct {
	MessageType            MessageType       `json:"messageType"`
	MessageVersion         Version           `json:"messageVersion"`
	ThreeDSServerTransID   string            `json:"threeDSServerTransID"`
	ThreeDSRequestorID     string            `json:"threeDSRequestorID"`
	ThreeDSRequestorName   string            `json:"threeDSRequestorName"`
	ThreeDSRequestorURL    string            `json:"threeDSRequestorURL"`
	AcquirerBIN            string            `json:"acquirerBIN"`
	AcquirerMerchantID     string            `json:"acquirerMerchantID"`
	MerchantName           string            `json:"merchantName"`
	MerchantCategoryCode   string            `json:"mcc"`
	CardholderAccount      string            `json:"cardholderAccount"`
	PurchaseAmount         string            `json:"purchaseAmount"`
	PurchaseCurrency       string            `json:"purchaseCurrency"`
	PurchaseExponent       string            `json:"purchaseExponent"`
	PurchaseDate           string            `json:"purchaseDate"`
	TransType              string            `json:"transType"`
	DeviceChannel          string            `json:"deviceChannel"`
	BrowserAcceptHeader    string            `json:"browserAcceptHeader,omitempty"`
	BrowserIP              string            `json:"browserIP,omitempty"`
	BrowserJavaEnabled     bool              `json:"browserJavaEnabled,omitempty"`
	BrowserLanguage        string            `json:"browserLanguage,omitempty"`
	BrowserColorDepth      string            `json:"browserColorDepth,omitempty"`
	BrowserScreenHeight    string            `json:"browserScreenHeight,omitempty"`
	BrowserScreenWidth     string            `json:"browserScreenWidth,omitempty"`
	BrowserTZ              string            `json:"browserTZ,omitempty"`
	BrowserUserAgent       string            `json:"browserUserAgent,omitempty"`
	ChallengeWindowSize    string            `json:"challengeWindowSize,omitempty"`
	Email                  string            `json:"email,omitempty"`
	MobilePhone            string            `json:"mobilePhone,omitempty"`
	CardholderName         string            `json:"cardholderName,omitempty"`
	ShipAddrCity           string            `json:"shipAddrCity,omitempty"`
	ShipAddrCountry        string            `json:"shipAddrCountry,omitempty"`
	ShipAddrLine1          string            `json:"shipAddrLine1,omitempty"`
	ShipAddrPostCode       string            `json:"shipAddrPostCode,omitempty"`
	ShipAddrState          string            `json:"shipAddrState,omitempty"`
	BillAddrCity           string            `json:"billAddrCity,omitempty"`
	BillAddrCountry        string            `json:"billAddrCountry,omitempty"`
	BillAddrLine1          string            `json:"billAddrLine1,omitempty"`
	BillAddrPostCode       string            `json:"billAddrPostCode,omitempty"`
	BillAddrState          string            `json:"billAddrState,omitempty"`
	ThreeDSRequestorAuthenticationInd string `json:"threeDSRequestorAuthenticationInd,omitempty"`
	ThreeDSRequestorChallengeInd      string `json:"threeDSRequestorChallengeInd,omitempty"`
}

// AuthenticationResponse represents 3DS authentication response
type AuthenticationResponse struct {
	MessageType          MessageType       `json:"messageType"`
	MessageVersion       Version           `json:"messageVersion"`
	ThreeDSServerTransID string            `json:"threeDSServerTransID"`
	ACSTransID           string            `json:"acsTransID"`
	DSTransID            string            `json:"dsTransID"`
	TransStatus          TransactionStatus `json:"transStatus"`
	TransStatusReason    string            `json:"transStatusReason,omitempty"`
	AuthenticationType   string            `json:"authenticationType,omitempty"`
	AuthenticationValue  string            `json:"authenticationValue,omitempty"`
	ECI                  string            `json:"eci,omitempty"`
	ACSChallengeMandated string            `json:"acsChallengeMandated,omitempty"`
	ACSDecConInd         string            `json:"acsDecConInd,omitempty"`
	ACSURL               string            `json:"acsURL,omitempty"`
	ChallengeRequest     *ChallengeRequest `json:"challengeRequest,omitempty"`
	ErrorCode            string            `json:"errorCode,omitempty"`
	ErrorDescription     string            `json:"errorDescription,omitempty"`
	ErrorDetail          string            `json:"errorDetail,omitempty"`
}

// ChallengeRequest contains challenge flow data
type ChallengeRequest struct {
	CReq                 string `json:"creq"`
	ACSURL               string `json:"acsURL"`
	ThreeDSSessionData   string `json:"threeDSSessionData"`
}

// ChallengeResponse contains challenge result
type ChallengeResponse struct {
	MessageType          MessageType       `json:"messageType"`
	ThreeDSServerTransID string            `json:"threeDSServerTransID"`
	ACSTransID           string            `json:"acsTransID"`
	TransStatus          TransactionStatus `json:"transStatus"`
	AuthenticationValue  string            `json:"authenticationValue"`
	ECI                  string            `json:"eci"`
}

// RiskIndicators contains transaction risk data
type RiskIndicators struct {
	ShipIndicator              string `json:"shipIndicator,omitempty"`
	DeliveryTimeframe          string `json:"deliveryTimeframe,omitempty"`
	DeliveryEmailAddress       string `json:"deliveryEmailAddress,omitempty"`
	ReorderItemsInd            string `json:"reorderItemsInd,omitempty"`
	PreOrderPurchaseInd        string `json:"preOrderPurchaseInd,omitempty"`
	PreOrderDate               string `json:"preOrderDate,omitempty"`
	GiftCardAmount             string `json:"giftCardAmount,omitempty"`
	GiftCardCount              string `json:"giftCardCount,omitempty"`
	GiftCardCurr               string `json:"giftCardCurr,omitempty"`
}

// CardholderAccountInfo contains cardholder account information
type CardholderAccountInfo struct {
	ChAccAgeInd              string `json:"chAccAgeInd,omitempty"`
	ChAccDate                string `json:"chAccDate,omitempty"`
	ChAccChangeInd           string `json:"chAccChangeInd,omitempty"`
	ChAccChange              string `json:"chAccChange,omitempty"`
	ChAccPwChangeInd         string `json:"chAccPwChangeInd,omitempty"`
	ChAccPwChange            string `json:"chAccPwChange,omitempty"`
	ShipAddressUsageInd      string `json:"shipAddressUsageInd,omitempty"`
	ShipAddressUsage         string `json:"shipAddressUsage,omitempty"`
	TxnActivityDay           string `json:"txnActivityDay,omitempty"`
	TxnActivityYear          string `json:"txnActivityYear,omitempty"`
	ProvisionAttemptsDay     string `json:"provisionAttemptsDay,omitempty"`
	NbPurchaseAccount        string `json:"nbPurchaseAccount,omitempty"`
	SuspiciousAccActivity    string `json:"suspiciousAccActivity,omitempty"`
	ShipNameIndicator        string `json:"shipNameIndicator,omitempty"`
	PaymentAccInd            string `json:"paymentAccInd,omitempty"`
	PaymentAccAge            string `json:"paymentAccAge,omitempty"`
}

// Authenticator handles 3DS authentication
type Authenticator struct {
	directoryServerURL string
	threeDSServerID    string
	threeDSRequestorID string
	threeDSRequestorName string
	acquirerBIN        string
	acquirerMerchantID string
	client             *http.Client
	sessions           map[string]*AuthSession
	mu                 sync.RWMutex
}

// AuthSession represents an active authentication session
type AuthSession struct {
	TransID            string
	ACSTransID         string
	DSTransID          string
	Status             TransactionStatus
	CreatedAt          time.Time
	ExpiresAt          time.Time
	Request            *AuthenticationRequest
	Response           *AuthenticationResponse
	ChallengeCompleted bool
}

// Config holds authenticator configuration
type Config struct {
	DirectoryServerURL   string
	ThreeDSServerID      string
	ThreeDSRequestorID   string
	ThreeDSRequestorName string
	AcquirerBIN          string
	AcquirerMerchantID   string
	Timeout              time.Duration
}

// NewAuthenticator creates a new 3DS authenticator
func NewAuthenticator(cfg Config) *Authenticator {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	return &Authenticator{
		directoryServerURL:   cfg.DirectoryServerURL,
		threeDSServerID:      cfg.ThreeDSServerID,
		threeDSRequestorID:   cfg.ThreeDSRequestorID,
		threeDSRequestorName: cfg.ThreeDSRequestorName,
		acquirerBIN:          cfg.AcquirerBIN,
		acquirerMerchantID:   cfg.AcquirerMerchantID,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		sessions: make(map[string]*AuthSession),
	}
}

// Authenticate initiates 3DS authentication flow
func (a *Authenticator) Authenticate(ctx context.Context, req *AuthenticationRequest) (*AuthenticationResponse, error) {
	// Generate transaction ID
	transID, err := a.generateTransactionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate transaction ID: %w", err)
	}

	// Set required fields
	req.MessageType = MessageTypeAReq
	req.MessageVersion = Version220
	req.ThreeDSServerTransID = transID
	req.ThreeDSRequestorID = a.threeDSRequestorID
	req.ThreeDSRequestorName = a.threeDSRequestorName
	req.AcquirerBIN = a.acquirerBIN
	req.AcquirerMerchantID = a.acquirerMerchantID

	// Validate request
	if err := a.validateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Create session
	session := &AuthSession{
		TransID:   transID,
		Status:    StatusChallengeRequired,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(15 * time.Minute),
		Request:   req,
	}

	a.mu.Lock()
	a.sessions[transID] = session
	a.mu.Unlock()

	// Send authentication request to directory server
	resp, err := a.sendAuthRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("authentication request failed: %w", err)
	}

	// Update session
	a.mu.Lock()
	session.Response = resp
	session.Status = resp.TransStatus
	session.ACSTransID = resp.ACSTransID
	session.DSTransID = resp.DSTransID
	a.mu.Unlock()

	return resp, nil
}

// ProcessChallenge processes challenge flow response
func (a *Authenticator) ProcessChallenge(ctx context.Context, transID string, cres string) (*ChallengeResponse, error) {
	a.mu.RLock()
	session, exists := a.sessions[transID]
	a.mu.RUnlock()

	if !exists {
		return nil, errors.New("session not found")
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, errors.New("session expired")
	}

	// Decode challenge response
	challengeResp, err := a.decodeChallengeResponse(cres)
	if err != nil {
		return nil, fmt.Errorf("failed to decode challenge response: %w", err)
	}

	// Update session
	a.mu.Lock()
	session.ChallengeCompleted = true
	session.Status = challengeResp.TransStatus
	a.mu.Unlock()

	return challengeResp, nil
}

// GetSession retrieves authentication session
func (a *Authenticator) GetSession(transID string) (*AuthSession, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	session, exists := a.sessions[transID]
	if !exists {
		return nil, errors.New("session not found")
	}

	return session, nil
}

// CleanupExpiredSessions removes expired sessions
func (a *Authenticator) CleanupExpiredSessions() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for transID, session := range a.sessions {
		if now.After(session.ExpiresAt) {
			delete(a.sessions, transID)
		}
	}
}

// sendAuthRequest sends authentication request to directory server
func (a *Authenticator) sendAuthRequest(ctx context.Context, req *AuthenticationRequest) (*AuthenticationResponse, error) {
	// Marshal request
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.directoryServerURL+"/authenticate", strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Send request
	httpResp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("authentication failed with status %d: %s", httpResp.StatusCode, string(body))
	}

	// Parse response
	var resp AuthenticationResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// generateTransactionID generates a unique transaction ID
func (a *Authenticator) generateTransactionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	// Create SHA-256 hash
	hash := sha256.Sum256(b)
	return base64.URLEncoding.EncodeToString(hash[:])[:36], nil
}

// validateRequest validates authentication request
func (a *Authenticator) validateRequest(req *AuthenticationRequest) error {
	if req.CardholderAccount == "" {
		return errors.New("cardholder account is required")
	}
	if req.PurchaseAmount == "" {
		return errors.New("purchase amount is required")
	}
	if req.PurchaseCurrency == "" {
		return errors.New("purchase currency is required")
	}
	if req.MerchantName == "" {
		return errors.New("merchant name is required")
	}
	if req.DeviceChannel == "" {
		return errors.New("device channel is required")
	}

	// Validate browser data for browser channel
	if req.DeviceChannel == "02" { // Browser
		if req.BrowserAcceptHeader == "" || req.BrowserUserAgent == "" {
			return errors.New("browser data is required for browser channel")
		}
	}

	return nil
}

// decodeChallengeResponse decodes base64 challenge response
func (a *Authenticator) decodeChallengeResponse(cres string) (*ChallengeResponse, error) {
	// Decode base64
	data, err := base64.StdEncoding.DecodeString(cres)
	if err != nil {
		return nil, err
	}

	// Parse JSON
	var resp ChallengeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// GenerateChallengeHTML generates HTML for challenge flow
func GenerateChallengeHTML(acsURL, creq, sessionData string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <title>3D Secure Authentication</title>
</head>
<body>
    <form id="challengeForm" method="POST" action="%s">
        <input type="hidden" name="creq" value="%s" />
        <input type="hidden" name="threeDSSessionData" value="%s" />
    </form>
    <script type="text/javascript">
        window.onload = function() {
            document.getElementById('challengeForm').submit();
        };
    </script>
</body>
</html>
`, acsURL, creq, sessionData)
}

// StartSessionCleanup starts periodic session cleanup
func (a *Authenticator) StartSessionCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			a.CleanupExpiredSessions()
		}
	}()
}
