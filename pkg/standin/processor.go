package standin

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	ErrStandinNotEnabled = errors.New("stand-in processing not enabled")
	ErrNoDecisionRule    = errors.New("no matching decision rule")
	ErrStandinExpired    = errors.New("stand-in authorization expired")
)

// Decision represents stand-in authorization decision
type Decision string

const (
	DecisionApprove Decision = "APPROVE"
	DecisionDecline Decision = "DECLINE"
	DecisionRefer   Decision = "REFER"
)

// Rule represents a stand-in decision rule
type Rule struct {
	ID          string
	Name        string
	Priority    int
	Conditions  []Condition
	Decision    Decision
	MaxAmount   int64
	Description string
	Enabled     bool
}

// Condition represents a rule condition
type Condition struct {
	Field    string
	Operator string // eq, ne, gt, lt, gte, lte, in, contains
	Value    interface{}
}

// Authorization represents a stand-in authorization
type Authorization struct {
	ID              string
	TransactionID   string
	CardNumber      string
	Amount          int64
	Currency        string
	MerchantID      string
	TerminalID      string
	Decision        Decision
	ResponseCode    string
	RuleID          string
	Timestamp       time.Time
	ExpiresAt       time.Time
	Reversed        bool
	ReversedAt      time.Time
}

// StandinProcessor handles stand-in processing
type StandinProcessor struct {
	enabled         bool
	rules           []*Rule
	authorizations  sync.Map // map[string]*Authorization
	logger          *zap.Logger
	
	// Configuration
	maxAmount       int64
	expiryDuration  time.Duration
	
	// Callbacks
	onAuthorization func(*Authorization)
	onReversal      func(*Authorization)
}

// Config configures stand-in processor
type Config struct {
	Enabled        bool
	MaxAmount      int64         // Maximum amount for stand-in
	ExpiryDuration time.Duration // Authorization expiry
	Rules          []*Rule
}

// NewStandinProcessor creates a stand-in processor
func NewStandinProcessor(config Config, logger *zap.Logger) *StandinProcessor {
	if config.MaxAmount == 0 {
		config.MaxAmount = 10000 // Default $100.00
	}
	if config.ExpiryDuration == 0 {
		config.ExpiryDuration = 24 * time.Hour
	}

	sp := &StandinProcessor{
		enabled:        config.Enabled,
		rules:          config.Rules,
		maxAmount:      config.MaxAmount,
		expiryDuration: config.ExpiryDuration,
		logger:         logger,
	}

	// Add default rules if none provided
	if len(sp.rules) == 0 {
		sp.rules = defaultRules()
	}

	// Sort rules by priority
	sp.sortRules()

	// Start cleanup goroutine
	go sp.cleanupLoop()

	return sp
}

// Process processes a transaction in stand-in mode
func (sp *StandinProcessor) Process(ctx context.Context, req *TransactionRequest) (*Authorization, error) {
	if !sp.enabled {
		return nil, ErrStandinNotEnabled
	}

	sp.logger.Info("processing stand-in authorization",
		zap.String("transaction_id", req.TransactionID),
		zap.Int64("amount", req.Amount))

	// Check maximum amount
	if req.Amount > sp.maxAmount {
		auth := &Authorization{
			ID:            generateID(),
			TransactionID: req.TransactionID,
			CardNumber:    req.CardNumber,
			Amount:        req.Amount,
			Currency:      req.Currency,
			MerchantID:    req.MerchantID,
			TerminalID:    req.TerminalID,
			Decision:      DecisionDecline,
			ResponseCode:  "61", // Exceeds withdrawal limit
			Timestamp:     time.Now(),
		}
		return auth, nil
	}

	// Evaluate rules
	for _, rule := range sp.rules {
		if !rule.Enabled {
			continue
		}

		if sp.evaluateRule(rule, req) {
			auth := &Authorization{
				ID:            generateID(),
				TransactionID: req.TransactionID,
				CardNumber:    req.CardNumber,
				Amount:        req.Amount,
				Currency:      req.Currency,
				MerchantID:    req.MerchantID,
				TerminalID:    req.TerminalID,
				Decision:      rule.Decision,
				ResponseCode:  sp.getResponseCode(rule.Decision),
				RuleID:        rule.ID,
				Timestamp:     time.Now(),
				ExpiresAt:     time.Now().Add(sp.expiryDuration),
			}

			// Store authorization
			sp.authorizations.Store(auth.ID, auth)

			sp.logger.Info("stand-in decision made",
				zap.String("auth_id", auth.ID),
				zap.String("decision", string(auth.Decision)),
				zap.String("rule", rule.Name))

			if sp.onAuthorization != nil {
				go sp.onAuthorization(auth)
			}

			return auth, nil
		}
	}

	// No rule matched - default to decline
	auth := &Authorization{
		ID:            generateID(),
		TransactionID: req.TransactionID,
		CardNumber:    req.CardNumber,
		Amount:        req.Amount,
		Currency:      req.Currency,
		MerchantID:    req.MerchantID,
		TerminalID:    req.TerminalID,
		Decision:      DecisionDecline,
		ResponseCode:  "05", // Do not honor
		Timestamp:     time.Now(),
	}

	return auth, nil
}

func (sp *StandinProcessor) evaluateRule(rule *Rule, req *TransactionRequest) bool {
	// Check amount limit
	if rule.MaxAmount > 0 && req.Amount > rule.MaxAmount {
		return false
	}

	// Evaluate all conditions (AND logic)
	for _, condition := range rule.Conditions {
		if !sp.evaluateCondition(condition, req) {
			return false
		}
	}

	return true
}

func (sp *StandinProcessor) evaluateCondition(condition Condition, req *TransactionRequest) bool {
	var fieldValue interface{}

	switch condition.Field {
	case "amount":
		fieldValue = req.Amount
	case "currency":
		fieldValue = req.Currency
	case "merchant_id":
		fieldValue = req.MerchantID
	case "terminal_id":
		fieldValue = req.TerminalID
	case "card_type":
		fieldValue = req.CardType
	case "transaction_type":
		fieldValue = req.TransactionType
	default:
		return false
	}

	switch condition.Operator {
	case "eq":
		return fieldValue == condition.Value
	case "ne":
		return fieldValue != condition.Value
	case "gt":
		if v, ok := fieldValue.(int64); ok {
			if cv, ok := condition.Value.(int64); ok {
				return v > cv
			}
		}
	case "lt":
		if v, ok := fieldValue.(int64); ok {
			if cv, ok := condition.Value.(int64); ok {
				return v < cv
			}
		}
	case "gte":
		if v, ok := fieldValue.(int64); ok {
			if cv, ok := condition.Value.(int64); ok {
				return v >= cv
			}
		}
	case "lte":
		if v, ok := fieldValue.(int64); ok {
			if cv, ok := condition.Value.(int64); ok {
				return v <= cv
			}
		}
	}

	return false
}

func (sp *StandinProcessor) getResponseCode(decision Decision) string {
	switch decision {
	case DecisionApprove:
		return "00" // Approved
	case DecisionDecline:
		return "05" // Do not honor
	case DecisionRefer:
		return "01" // Refer to card issuer
	default:
		return "05"
	}
}

// Reverse reverses a stand-in authorization
func (sp *StandinProcessor) Reverse(authID string) error {
	val, exists := sp.authorizations.Load(authID)
	if !exists {
		return errors.New("authorization not found")
	}

	auth := val.(*Authorization)
	if auth.Reversed {
		return errors.New("authorization already reversed")
	}

	auth.Reversed = true
	auth.ReversedAt = time.Now()

	sp.logger.Info("stand-in authorization reversed",
		zap.String("auth_id", authID))

	if sp.onReversal != nil {
		go sp.onReversal(auth)
	}

	return nil
}

// GetAuthorization retrieves an authorization
func (sp *StandinProcessor) GetAuthorization(authID string) (*Authorization, error) {
	val, exists := sp.authorizations.Load(authID)
	if !exists {
		return nil, errors.New("authorization not found")
	}

	auth := val.(*Authorization)

	// Check if expired
	if time.Now().After(auth.ExpiresAt) {
		return nil, ErrStandinExpired
	}

	return auth, nil
}

// AddRule adds a new rule
func (sp *StandinProcessor) AddRule(rule *Rule) {
	sp.rules = append(sp.rules, rule)
	sp.sortRules()
	sp.logger.Info("stand-in rule added", zap.String("rule", rule.Name))
}

// RemoveRule removes a rule
func (sp *StandinProcessor) RemoveRule(ruleID string) {
	for i, rule := range sp.rules {
		if rule.ID == ruleID {
			sp.rules = append(sp.rules[:i], sp.rules[i+1:]...)
			sp.logger.Info("stand-in rule removed", zap.String("rule_id", ruleID))
			return
		}
	}
}

func (sp *StandinProcessor) sortRules() {
	// Simple bubble sort by priority (higher priority first)
	for i := 0; i < len(sp.rules); i++ {
		for j := i + 1; j < len(sp.rules); j++ {
			if sp.rules[j].Priority > sp.rules[i].Priority {
				sp.rules[i], sp.rules[j] = sp.rules[j], sp.rules[i]
			}
		}
	}
}

func (sp *StandinProcessor) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		sp.cleanup()
	}
}

func (sp *StandinProcessor) cleanup() {
	now := time.Now()
	var toDelete []string

	sp.authorizations.Range(func(key, value interface{}) bool {
		auth := value.(*Authorization)
		if now.After(auth.ExpiresAt) {
			toDelete = append(toDelete, key.(string))
		}
		return true
	})

	for _, id := range toDelete {
		sp.authorizations.Delete(id)
	}

	if len(toDelete) > 0 {
		sp.logger.Debug("cleaned up expired authorizations",
			zap.Int("count", len(toDelete)))
	}
}

// TransactionRequest represents a transaction request
type TransactionRequest struct {
	TransactionID   string
	CardNumber      string
	Amount          int64
	Currency        string
	MerchantID      string
	TerminalID      string
	CardType        string
	TransactionType string
}

func defaultRules() []*Rule {
	return []*Rule{
		{
			ID:       "rule_small_amount",
			Name:     "Approve Small Amounts",
			Priority: 100,
			Conditions: []Condition{
				{Field: "amount", Operator: "lte", Value: int64(2000)}, // $20.00 or less
			},
			Decision:    DecisionApprove,
			MaxAmount:   2000,
			Description: "Automatically approve transactions $20 or less",
			Enabled:     true,
		},
		{
			ID:       "rule_medium_amount",
			Name:     "Approve Medium Amounts",
			Priority: 90,
			Conditions: []Condition{
				{Field: "amount", Operator: "lte", Value: int64(5000)}, // $50.00 or less
			},
			Decision:    DecisionApprove,
			MaxAmount:   5000,
			Description: "Automatically approve transactions $50 or less",
			Enabled:     true,
		},
		{
			ID:          "rule_large_amount",
			Name:        "Refer Large Amounts",
			Priority:    80,
			Conditions:  []Condition{},
			Decision:    DecisionRefer,
			Description: "Refer large transactions for manual review",
			Enabled:     true,
		},
	}
}

func generateID() string {
	// Simple ID generation - in production, use UUID
	return time.Now().Format("20060102150405.000000")
}

// SetOnAuthorization sets authorization callback
func (sp *StandinProcessor) SetOnAuthorization(fn func(*Authorization)) {
	sp.onAuthorization = fn
}

// SetOnReversal sets reversal callback
func (sp *StandinProcessor) SetOnReversal(fn func(*Authorization)) {
	sp.onReversal = fn
}

// GetStats returns stand-in statistics
func (sp *StandinProcessor) GetStats() map[string]interface{} {
	totalAuths := 0
	approvedCount := 0
	declinedCount := 0
	referredCount := 0
	reversedCount := 0

	sp.authorizations.Range(func(key, value interface{}) bool {
		auth := value.(*Authorization)
		totalAuths++
		
		switch auth.Decision {
		case DecisionApprove:
			approvedCount++
		case DecisionDecline:
			declinedCount++
		case DecisionRefer:
			referredCount++
		}
		
		if auth.Reversed {
			reversedCount++
		}
		
		return true
	})

	return map[string]interface{}{
		"enabled":        sp.enabled,
		"total_auths":    totalAuths,
		"approved":       approvedCount,
		"declined":       declinedCount,
		"referred":       referredCount,
		"reversed":       reversedCount,
		"rules_count":    len(sp.rules),
		"max_amount":     sp.maxAmount,
		"expiry_duration": sp.expiryDuration.String(),
	}
}
