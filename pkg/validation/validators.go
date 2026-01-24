package validation

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrValidationFailed = errors.New("validation failed")
	ErrInvalidFormat    = errors.New("invalid format")
	ErrOutOfRange       = errors.New("value out of range")
)

// Validator interface for validation
type Validator interface {
	Validate(value interface{}) error
}

// CardNumberValidator validates credit card numbers
type CardNumberValidator struct{}

func (v *CardNumberValidator) Validate(value interface{}) error {
	cardNumber, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: expected string", ErrInvalidFormat)
	}

	// Remove spaces and dashes
	cardNumber = strings.ReplaceAll(cardNumber, " ", "")
	cardNumber = strings.ReplaceAll(cardNumber, "-", "")

	// Check length
	if len(cardNumber) < 13 || len(cardNumber) > 19 {
		return fmt.Errorf("%w: card number must be 13-19 digits", ErrInvalidFormat)
	}

	// Check if all digits
	for _, c := range cardNumber {
		if c < '0' || c > '9' {
			return fmt.Errorf("%w: card number must contain only digits", ErrInvalidFormat)
		}
	}

	// Luhn algorithm
	if !luhnCheck(cardNumber) {
		return fmt.Errorf("%w: invalid card number (failed Luhn check)", ErrValidationFailed)
	}

	return nil
}

// luhnCheck performs Luhn algorithm validation
func luhnCheck(cardNumber string) bool {
	sum := 0
	alternate := false

	for i := len(cardNumber) - 1; i >= 0; i-- {
		digit := int(cardNumber[i] - '0')

		if alternate {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}

		sum += digit
		alternate = !alternate
	}

	return sum%10 == 0
}

// CVVValidator validates CVV/CVC codes
type CVVValidator struct{}

func (v *CVVValidator) Validate(value interface{}) error {
	cvv, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: expected string", ErrInvalidFormat)
	}

	// Check length (3 or 4 digits)
	if len(cvv) != 3 && len(cvv) != 4 {
		return fmt.Errorf("%w: CVV must be 3 or 4 digits", ErrInvalidFormat)
	}

	// Check if all digits
	for _, c := range cvv {
		if c < '0' || c > '9' {
			return fmt.Errorf("%w: CVV must contain only digits", ErrInvalidFormat)
		}
	}

	return nil
}

// ExpiryDateValidator validates card expiry dates
type ExpiryDateValidator struct{}

func (v *ExpiryDateValidator) Validate(value interface{}) error {
	expiry, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: expected string", ErrInvalidFormat)
	}

	// Parse MM/YY or MMYY format
	expiry = strings.ReplaceAll(expiry, "/", "")
	if len(expiry) != 4 {
		return fmt.Errorf("%w: expiry must be MMYY format", ErrInvalidFormat)
	}

	var month, year int
	if _, err := fmt.Sscanf(expiry, "%02d%02d", &month, &year); err != nil {
		return fmt.Errorf("%w: invalid expiry format", ErrInvalidFormat)
	}

	if month < 1 || month > 12 {
		return fmt.Errorf("%w: invalid month", ErrValidationFailed)
	}

	// Check if expired (assuming YY is 20YY)
	now := time.Now()
	currentYear := now.Year() % 100
	currentMonth := int(now.Month())

	if year < currentYear || (year == currentYear && month < currentMonth) {
		return fmt.Errorf("%w: card expired", ErrValidationFailed)
	}

	return nil
}

// AmountValidator validates transaction amounts
type AmountValidator struct {
	Min int64
	Max int64
}

func (v *AmountValidator) Validate(value interface{}) error {
	amount, ok := value.(int64)
	if !ok {
		return fmt.Errorf("%w: expected int64", ErrInvalidFormat)
	}

	if amount < 0 {
		return fmt.Errorf("%w: amount cannot be negative", ErrValidationFailed)
	}

	if v.Min > 0 && amount < v.Min {
		return fmt.Errorf("%w: amount below minimum (%d)", ErrOutOfRange, v.Min)
	}

	if v.Max > 0 && amount > v.Max {
		return fmt.Errorf("%w: amount exceeds maximum (%d)", ErrOutOfRange, v.Max)
	}

	return nil
}

// CurrencyValidator validates currency codes
type CurrencyValidator struct{}

var validCurrencies = map[string]bool{
	"USD": true, "EUR": true, "GBP": true, "JPY": true, "CHF": true,
	"CAD": true, "AUD": true, "NZD": true, "CNY": true, "INR": true,
	"BRL": true, "ZAR": true, "RUB": true, "MXN": true, "SGD": true,
	"HKD": true, "NOK": true, "SEK": true, "DKK": true, "PLN": true,
}

func (v *CurrencyValidator) Validate(value interface{}) error {
	currency, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: expected string", ErrInvalidFormat)
	}

	currency = strings.ToUpper(currency)

	if len(currency) != 3 {
		return fmt.Errorf("%w: currency code must be 3 characters", ErrInvalidFormat)
	}

	if !validCurrencies[currency] {
		return fmt.Errorf("%w: unsupported currency %s", ErrValidationFailed, currency)
	}

	return nil
}

// MerchantIDValidator validates merchant IDs
type MerchantIDValidator struct{}

func (v *MerchantIDValidator) Validate(value interface{}) error {
	merchantID, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: expected string", ErrInvalidFormat)
	}

	if len(merchantID) < 1 || len(merchantID) > 15 {
		return fmt.Errorf("%w: merchant ID must be 1-15 characters", ErrInvalidFormat)
	}

	// Check alphanumeric
	matched, _ := regexp.MatchString("^[A-Za-z0-9]+$", merchantID)
	if !matched {
		return fmt.Errorf("%w: merchant ID must be alphanumeric", ErrInvalidFormat)
	}

	return nil
}

// TerminalIDValidator validates terminal IDs
type TerminalIDValidator struct{}

func (v *TerminalIDValidator) Validate(value interface{}) error {
	terminalID, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: expected string", ErrInvalidFormat)
	}

	if len(terminalID) < 1 || len(terminalID) > 8 {
		return fmt.Errorf("%w: terminal ID must be 1-8 characters", ErrInvalidFormat)
	}

	// Check alphanumeric
	matched, _ := regexp.MatchString("^[A-Za-z0-9]+$", terminalID)
	if !matched {
		return fmt.Errorf("%w: terminal ID must be alphanumeric", ErrInvalidFormat)
	}

	return nil
}

// EmailValidator validates email addresses
type EmailValidator struct{}

func (v *EmailValidator) Validate(value interface{}) error {
	email, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: expected string", ErrInvalidFormat)
	}

	// Simple email regex
	pattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	matched, _ := regexp.MatchString(pattern, email)

	if !matched {
		return fmt.Errorf("%w: invalid email format", ErrInvalidFormat)
	}

	return nil
}

// PhoneValidator validates phone numbers
type PhoneValidator struct{}

func (v *PhoneValidator) Validate(value interface{}) error {
	phone, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: expected string", ErrInvalidFormat)
	}

	// Remove common separators
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")
	phone = strings.ReplaceAll(phone, "+", "")

	// Check length (7-15 digits)
	if len(phone) < 7 || len(phone) > 15 {
		return fmt.Errorf("%w: phone number must be 7-15 digits", ErrInvalidFormat)
	}

	// Check if all digits
	for _, c := range phone {
		if c < '0' || c > '9' {
			return fmt.Errorf("%w: phone number must contain only digits", ErrInvalidFormat)
		}
	}

	return nil
}

// IPAddressValidator validates IP addresses
type IPAddressValidator struct{}

func (v *IPAddressValidator) Validate(value interface{}) error {
	ip, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: expected string", ErrInvalidFormat)
	}

	// Simple IPv4 validation
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return fmt.Errorf("%w: invalid IP address format", ErrInvalidFormat)
	}

	for _, part := range parts {
		var num int
		if _, err := fmt.Sscanf(part, "%d", &num); err != nil {
			return fmt.Errorf("%w: invalid IP address", ErrInvalidFormat)
		}
		if num < 0 || num > 255 {
			return fmt.Errorf("%w: IP octet out of range", ErrOutOfRange)
		}
	}

	return nil
}

// TransactionRequest represents a transaction for validation
type TransactionRequest struct {
	CardNumber   string
	CVV          string
	ExpiryDate   string
	Amount       int64
	Currency     string
	MerchantID   string
	TerminalID   string
	CustomerEmail string
	CustomerPhone string
}

// TransactionValidator validates complete transactions
type TransactionValidator struct {
	cardValidator     *CardNumberValidator
	cvvValidator      *CVVValidator
	expiryValidator   *ExpiryDateValidator
	amountValidator   *AmountValidator
	currencyValidator *CurrencyValidator
	merchantValidator *MerchantIDValidator
	terminalValidator *TerminalIDValidator
	emailValidator    *EmailValidator
	phoneValidator    *PhoneValidator
}

// NewTransactionValidator creates a transaction validator
func NewTransactionValidator(minAmount, maxAmount int64) *TransactionValidator {
	return &TransactionValidator{
		cardValidator:     &CardNumberValidator{},
		cvvValidator:      &CVVValidator{},
		expiryValidator:   &ExpiryDateValidator{},
		amountValidator:   &AmountValidator{Min: minAmount, Max: maxAmount},
		currencyValidator: &CurrencyValidator{},
		merchantValidator: &MerchantIDValidator{},
		terminalValidator: &TerminalIDValidator{},
		emailValidator:    &EmailValidator{},
		phoneValidator:    &PhoneValidator{},
	}
}

// Validate validates a transaction request
func (v *TransactionValidator) Validate(req *TransactionRequest) []error {
	var errors []error

	// Validate card number
	if err := v.cardValidator.Validate(req.CardNumber); err != nil {
		errors = append(errors, fmt.Errorf("card_number: %w", err))
	}

	// Validate CVV
	if err := v.cvvValidator.Validate(req.CVV); err != nil {
		errors = append(errors, fmt.Errorf("cvv: %w", err))
	}

	// Validate expiry
	if err := v.expiryValidator.Validate(req.ExpiryDate); err != nil {
		errors = append(errors, fmt.Errorf("expiry_date: %w", err))
	}

	// Validate amount
	if err := v.amountValidator.Validate(req.Amount); err != nil {
		errors = append(errors, fmt.Errorf("amount: %w", err))
	}

	// Validate currency
	if err := v.currencyValidator.Validate(req.Currency); err != nil {
		errors = append(errors, fmt.Errorf("currency: %w", err))
	}

	// Validate merchant ID
	if err := v.merchantValidator.Validate(req.MerchantID); err != nil {
		errors = append(errors, fmt.Errorf("merchant_id: %w", err))
	}

	// Validate terminal ID
	if err := v.terminalValidator.Validate(req.TerminalID); err != nil {
		errors = append(errors, fmt.Errorf("terminal_id: %w", err))
	}

	// Validate email (optional)
	if req.CustomerEmail != "" {
		if err := v.emailValidator.Validate(req.CustomerEmail); err != nil {
			errors = append(errors, fmt.Errorf("customer_email: %w", err))
		}
	}

	// Validate phone (optional)
	if req.CustomerPhone != "" {
		if err := v.phoneValidator.Validate(req.CustomerPhone); err != nil {
			errors = append(errors, fmt.Errorf("customer_phone: %w", err))
		}
	}

	return errors
}

// ValidateCardBIN validates BIN (Bank Identification Number)
func ValidateCardBIN(cardNumber string) (string, error) {
	if len(cardNumber) < 6 {
		return "", fmt.Errorf("%w: card number too short", ErrInvalidFormat)
	}

	bin := cardNumber[:6]

	// Detect card type based on BIN
	switch {
	case bin[0] == '4':
		return "VISA", nil
	case bin[0] == '5' && bin[1] >= '1' && bin[1] <= '5':
		return "MASTERCARD", nil
	case bin[:2] == "34" || bin[:2] == "37":
		return "AMEX", nil
	case bin[:4] == "6011" || (bin[:2] == "65") || (bin[:3] >= "644" && bin[:3] <= "649"):
		return "DISCOVER", nil
	case bin[:4] >= "3528" && bin[:4] <= "3589":
		return "JCB", nil
	default:
		return "UNKNOWN", nil
	}
}

// SanitizeCardNumber removes non-digit characters from card number
func SanitizeCardNumber(cardNumber string) string {
	var result strings.Builder
	for _, c := range cardNumber {
		if c >= '0' && c <= '9' {
			result.WriteRune(c)
		}
	}
	return result.String()
}

// MaskCardNumber masks a card number for display
func MaskCardNumber(cardNumber string) string {
	cardNumber = SanitizeCardNumber(cardNumber)
	if len(cardNumber) < 4 {
		return strings.Repeat("*", len(cardNumber))
	}

	// Show last 4 digits
	masked := strings.Repeat("*", len(cardNumber)-4) + cardNumber[len(cardNumber)-4:]
	
	// Add spaces for readability (groups of 4)
	var result strings.Builder
	for i, c := range masked {
		if i > 0 && i%4 == 0 {
			result.WriteRune(' ')
		}
		result.WriteRune(c)
	}
	
	return result.String()
}
