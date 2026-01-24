package fraud

import (
	"context"
	"fmt"
	"time"

	"github.com/krish567366/OpenTX/pkg/proto/canonical"
	"go.uber.org/zap"
)

// RiskScore represents the fraud risk assessment
type RiskScore struct {
	Score      float64           // 0-100, higher = more risky
	Level      RiskLevel         // LOW, MEDIUM, HIGH, CRITICAL
	Flags      []string          // Specific risk flags triggered
	Reason     string            // Human-readable explanation
	Indicators map[string]float64 // Individual indicator scores
	Timestamp  time.Time
}

// RiskLevel categorizes fraud risk
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "LOW"
	RiskLevelMedium   RiskLevel = "MEDIUM"
	RiskLevelHigh     RiskLevel = "HIGH"
	RiskLevelCritical RiskLevel = "CRITICAL"
)

// Action to take based on risk assessment
type Action string

const (
	ActionApprove  Action = "APPROVE"
	ActionReview   Action = "REVIEW"
	ActionChallenge Action = "CHALLENGE" // Require additional authentication
	ActionBlock    Action = "BLOCK"
)

// FraudDetector interface for fraud detection
type FraudDetector interface {
	Assess(ctx context.Context, txn *canonical.CanonicalTransaction) (*RiskScore, error)
	DetermineAction(score *RiskScore) Action
}

// RuleBasedDetector implements rule-based fraud detection
type RuleBasedDetector struct {
	rules  []Rule
	logger *zap.Logger
}

// Rule defines a fraud detection rule
type Rule interface {
	Evaluate(ctx context.Context, txn *canonical.CanonicalTransaction) (float64, []string, error)
	Name() string
	Weight() float64
}

// NewRuleBasedDetector creates a new rule-based fraud detector
func NewRuleBasedDetector(logger *zap.Logger) *RuleBasedDetector {
	detector := &RuleBasedDetector{
		logger: logger,
		rules:  make([]Rule, 0),
	}

	// Add default rules
	detector.AddRule(&VelocityRule{})
	detector.AddRule(&AmountAnomalyRule{})
	detector.AddRule(&GeographyRule{})
	detector.AddRule(&TimePatternRule{})
	detector.AddRule(&CardTestingRule{})
	detector.AddRule(&BINRule{})

	return detector
}

// AddRule adds a fraud detection rule
func (fd *RuleBasedDetector) AddRule(rule Rule) {
	fd.rules = append(fd.rules, rule)
}

// Assess evaluates transaction for fraud
func (fd *RuleBasedDetector) Assess(ctx context.Context, txn *canonical.CanonicalTransaction) (*RiskScore, error) {
	score := &RiskScore{
		Score:      0.0,
		Flags:      make([]string, 0),
		Indicators: make(map[string]float64),
		Timestamp:  time.Now(),
	}

	var totalWeight float64

	// Evaluate all rules
	for _, rule := range fd.rules {
		ruleScore, flags, err := rule.Evaluate(ctx, txn)
		if err != nil {
			fd.logger.Error("rule evaluation failed",
				zap.String("rule", rule.Name()),
				zap.Error(err))
			continue
		}

		weight := rule.Weight()
		score.Score += ruleScore * weight
		totalWeight += weight
		score.Indicators[rule.Name()] = ruleScore

		if len(flags) > 0 {
			score.Flags = append(score.Flags, flags...)
		}
	}

	// Normalize score
	if totalWeight > 0 {
		score.Score = score.Score / totalWeight
	}

	// Determine risk level
	score.Level = fd.determineRiskLevel(score.Score)
	score.Reason = fd.generateReason(score)

	fd.logger.Info("fraud assessment completed",
		zap.String("transaction_id", txn.MessageId),
		zap.Float64("score", score.Score),
		zap.String("level", string(score.Level)),
		zap.Strings("flags", score.Flags))

	return score, nil
}

// DetermineAction determines action based on risk score
func (fd *RuleBasedDetector) DetermineAction(score *RiskScore) Action {
	switch score.Level {
	case RiskLevelLow:
		return ActionApprove
	case RiskLevelMedium:
		return ActionReview
	case RiskLevelHigh:
		return ActionChallenge
	case RiskLevelCritical:
		return ActionBlock
	default:
		return ActionReview
	}
}

func (fd *RuleBasedDetector) determineRiskLevel(score float64) RiskLevel {
	switch {
	case score >= 75:
		return RiskLevelCritical
	case score >= 50:
		return RiskLevelHigh
	case score >= 25:
		return RiskLevelMedium
	default:
		return RiskLevelLow
	}
}

func (fd *RuleBasedDetector) generateReason(score *RiskScore) string {
	if len(score.Flags) == 0 {
		return "No fraud indicators detected"
	}
	return fmt.Sprintf("Fraud indicators: %v", score.Flags)
}

// VelocityRule checks for high transaction velocity
type VelocityRule struct{}

func (r *VelocityRule) Name() string { return "velocity" }
func (r *VelocityRule) Weight() float64 { return 1.5 }

func (r *VelocityRule) Evaluate(ctx context.Context, txn *canonical.CanonicalTransaction) (float64, []string, error) {
	// TODO: Implement velocity checking
	// - Count transactions from same card in last 1 hour, 24 hours
	// - Check transactions from same merchant
	// - Detect rapid successive transactions
	
	var score float64
	var flags []string

	// Placeholder logic
	cardData := txn.GetCardData()
	if cardData != nil {
		// High velocity detection would go here
		// For now, return low risk
		score = 0.0
	}

	return score, flags, nil
}

// AmountAnomalyRule checks for unusual transaction amounts
type AmountAnomalyRule struct{}

func (r *AmountAnomalyRule) Name() string { return "amount_anomaly" }
func (r *AmountAnomalyRule) Weight() float64 { return 1.2 }

func (r *AmountAnomalyRule) Evaluate(ctx context.Context, txn *canonical.CanonicalTransaction) (float64, []string, error) {
	var score float64
	var flags []string

	amount := txn.GetMoney().GetAmount()

	// Check for round amounts (potential testing)
	if amount%100000 == 0 { // Round to major currency units
		score += 20
		flags = append(flags, "round_amount")
	}

	// Check for unusually high amounts
	if amount > 50000000 { // $500,000
		score += 40
		flags = append(flags, "high_amount")
	}

	// Check for very small amounts (potential testing)
	if amount < 100 { // $1.00
		score += 15
		flags = append(flags, "low_amount_testing")
	}

	return score, flags, nil
}

// GeographyRule checks for geographical anomalies
type GeographyRule struct{}

func (r *GeographyRule) Name() string { return "geography" }
func (r *GeographyRule) Weight() float64 { return 1.3 }

func (r *GeographyRule) Evaluate(ctx context.Context, txn *canonical.CanonicalTransaction) (float64, []string, error) {
	var score float64
	var flags []string

	// TODO: Implement geographical checks
	// - Check if terminal country matches card issuer country
	// - Detect rapid geographical changes
	// - Flag high-risk countries
	
	terminal := txn.GetTerminalData()
	if terminal != nil {
		terminalCountry := terminal.GetCountryCode()
		
		// Example: Flag certain high-risk countries
		highRiskCountries := map[string]bool{
			// This is just an example, not actual fraud data
		}
		
		if highRiskCountries[terminalCountry] {
			score += 30
			flags = append(flags, "high_risk_country")
		}
	}

	return score, flags, nil
}

// TimePatternRule checks for suspicious time patterns
type TimePatternRule struct{}

func (r *TimePatternRule) Name() string { return "time_pattern" }
func (r *TimePatternRule) Weight() float64 { return 0.8 }

func (r *TimePatternRule) Evaluate(ctx context.Context, txn *canonical.CanonicalTransaction) (float64, []string, error) {
	var score float64
	var flags []string

	txnTime := txn.GetTransactionDatetime().AsTime()
	hour := txnTime.Hour()

	// Flag unusual hours (2 AM - 5 AM local time)
	if hour >= 2 && hour < 5 {
		score += 15
		flags = append(flags, "unusual_hour")
	}

	// Check if transaction is on holiday/weekend
	if txnTime.Weekday() == time.Saturday || txnTime.Weekday() == time.Sunday {
		score += 5
		flags = append(flags, "weekend_transaction")
	}

	return score, flags, nil
}

// CardTestingRule detects card testing patterns
type CardTestingRule struct{}

func (r *CardTestingRule) Name() string { return "card_testing" }
func (r *CardTestingRule) Weight() float64 { return 2.0 }

func (r *CardTestingRule) Evaluate(ctx context.Context, txn *canonical.CanonicalTransaction) (float64, []string, error) {
	var score float64
	var flags []string

	// TODO: Implement card testing detection
	// - Multiple small transactions in quick succession
	// - Sequential card numbers being tested
	// - Multiple failed authorization attempts
	
	amount := txn.GetMoney().GetAmount()
	
	// Very small amount could be testing
	if amount < 200 { // Less than $2.00
		score += 25
		flags = append(flags, "potential_card_testing")
	}

	return score, flags, nil
}

// BINRule checks Bank Identification Number patterns
type BINRule struct{}

func (r *BINRule) Name() string { return "bin_check" }
func (r *BINRule) Weight() float64 { return 1.0 }

func (r *BINRule) Evaluate(ctx context.Context, txn *canonical.CanonicalTransaction) (float64, []string, error) {
	var score float64
	var flags []string

	cardData := txn.GetCardData()
	if cardData != nil {
		// TODO: Implement BIN database lookup
		// - Check if BIN is valid
		// - Check if BIN is from high-risk issuer
		// - Validate card type matches transaction type
		
		// Example: Check for prepaid cards (higher risk)
		cardType := cardData.GetCardType()
		if cardType == canonical.CardType_CARD_TYPE_PREPAID {
			score += 20
			flags = append(flags, "prepaid_card")
		}
	}

	return score, flags, nil
}

// FraudCache caches fraud assessments to avoid re-evaluation
type FraudCache struct {
	// In production, use Redis
	cache  map[string]*RiskScore
	logger *zap.Logger
}

// NewFraudCache creates a new fraud cache
func NewFraudCache(logger *zap.Logger) *FraudCache {
	return &FraudCache{
		cache:  make(map[string]*RiskScore),
		logger: logger,
	}
}

// Get retrieves cached fraud score
func (fc *FraudCache) Get(key string) (*RiskScore, bool) {
	score, exists := fc.cache[key]
	return score, exists
}

// Set stores fraud score in cache
func (fc *FraudCache) Set(key string, score *RiskScore, ttl time.Duration) {
	fc.cache[key] = score
	// TODO: Implement TTL with Redis
}

// MLBasedDetector is a placeholder for ML-based fraud detection
type MLBasedDetector struct {
	modelPath string
	logger    *zap.Logger
}

// NewMLBasedDetector creates ML-based detector
func NewMLBasedDetector(modelPath string, logger *zap.Logger) *MLBasedDetector {
	return &MLBasedDetector{
		modelPath: modelPath,
		logger:    logger,
	}
}

// Assess uses ML model for fraud detection
func (md *MLBasedDetector) Assess(ctx context.Context, txn *canonical.CanonicalTransaction) (*RiskScore, error) {
	// TODO: Implement ML model inference
	// - Load trained model
	// - Extract features from transaction
	// - Run inference
	// - Return probability score
	
	md.logger.Info("ML fraud detection not yet implemented",
		zap.String("transaction_id", txn.MessageId))
	
	return &RiskScore{
		Score:     0.0,
		Level:     RiskLevelLow,
		Reason:    "ML detection pending",
		Timestamp: time.Now(),
	}, nil
}

// DetermineAction determines action for ML-based score
func (md *MLBasedDetector) DetermineAction(score *RiskScore) Action {
	if score.Score >= 0.75 {
		return ActionBlock
	} else if score.Score >= 0.50 {
		return ActionChallenge
	} else if score.Score >= 0.25 {
		return ActionReview
	}
	return ActionApprove
}
