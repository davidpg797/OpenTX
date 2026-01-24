package testutil

import (
	"math/rand"
	"time"

	"github.com/krish567366/OpenTX/pkg/iso8583"
	"github.com/krish567366/OpenTX/pkg/proto/canonical"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestDataGenerator generates synthetic test data
type TestDataGenerator struct {
	rand *rand.Rand
}

// NewTestDataGenerator creates a new test data generator
func NewTestDataGenerator() *TestDataGenerator {
	return &TestDataGenerator{
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// GenerateCard generates a test card number
func (g *TestDataGenerator) GenerateCard(brand canonical.CardBrand) *canonical.CardData {
	var pan string
	
	switch brand {
	case canonical.CardBrand_CARD_BRAND_VISA:
		pan = g.generateVisaCard()
	case canonical.CardBrand_CARD_BRAND_MASTERCARD:
		pan = g.generateMastercardCard()
	case canonical.CardBrand_CARD_BRAND_AMEX:
		pan = g.generateAmexCard()
	default:
		pan = g.generateVisaCard()
	}

	return &canonical.CardData{
		Pan:            pan,
		ExpirationDate: g.generateExpiry(),
		CardBrand:      brand,
		CardType:       canonical.CardType_CARD_TYPE_CREDIT,
	}
}

func (g *TestDataGenerator) generateVisaCard() string {
	// Visa test cards start with 4
	return "4" + g.generateDigits(15)
}

func (g *TestDataGenerator) generateMastercardCard() string {
	// Mastercard test cards start with 5
	return "5" + g.generateDigits(15)
}

func (g *TestDataGenerator) generateAmexCard() string {
	// Amex test cards start with 3
	return "3" + g.generateDigits(14)
}

func (g *TestDataGenerator) generateDigits(n int) string {
	digits := make([]byte, n)
	for i := 0; i < n; i++ {
		digits[i] = byte('0' + g.rand.Intn(10))
	}
	return string(digits)
}

func (g *TestDataGenerator) generateExpiry() string {
	// Generate expiry 1-5 years in future
	year := time.Now().Year() + 1 + g.rand.Intn(5)
	month := 1 + g.rand.Intn(12)
	return time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).Format("0601")
}

// GenerateTransaction generates a test canonical transaction
func (g *TestDataGenerator) GenerateTransaction() *canonical.CanonicalTransaction {
	now := time.Now()
	
	return &canonical.CanonicalTransaction{
		MessageId:          g.generateMessageID(),
		MessageType:        canonical.MessageType_MESSAGE_TYPE_AUTHORIZATION_REQUEST,
		TransactionType:    canonical.TransactionType_TRANSACTION_TYPE_PURCHASE,
		TransactionDatetime: timestamppb.New(now),
		
		Money: &canonical.Money{
			Amount:   int64(g.rand.Intn(100000) + 100), // $1 to $1000
			Currency: "USD",
		},
		CurrencyCode: "840",
		
		CardData: g.GenerateCard(canonical.CardBrand_CARD_BRAND_VISA),
		
		MerchantData: &canonical.MerchantData{
			MerchantId:           g.generateMerchantID(),
			MerchantName:         "Test Merchant",
			MerchantCategoryCode: "5411",
			MerchantCity:         "San Francisco",
			MerchantCountry:      "USA",
		},
		
		TerminalData: &canonical.TerminalData{
			TerminalId:  g.generateTerminalID(),
			CountryCode: "840",
		},
		
		Stan: g.generateSTAN(),
		Rrn:  g.generateRRN(),
		
		PosData: &canonical.POSData{
			EntryMode:     canonical.POSEntryMode_POS_ENTRY_MODE_CHIP,
			ConditionCode: canonical.POSConditionCode_POS_CONDITION_NORMAL,
		},
	}
}

// GenerateISO8583Message generates a test ISO 8583 message
func (g *TestDataGenerator) GenerateISO8583Message() *iso8583.Message {
	msg := iso8583.NewMessage()
	msg.MTI = "0100" // Authorization request
	
	// Add required fields
	msg.SetField(2, &iso8583.Field{Value: g.generateVisaCard()})     // PAN
	msg.SetField(3, &iso8583.Field{Value: "000000"})                  // Processing code
	msg.SetField(4, &iso8583.Field{Value: "000000010000"})            // Amount
	msg.SetField(7, &iso8583.Field{Value: time.Now().Format("0102150405")}) // Transmission date/time
	msg.SetField(11, &iso8583.Field{Value: g.generateSTAN()})         // STAN
	msg.SetField(12, &iso8583.Field{Value: time.Now().Format("150405")})    // Local time
	msg.SetField(13, &iso8583.Field{Value: time.Now().Format("0102")})      // Local date
	msg.SetField(22, &iso8583.Field{Value: "051"})                    // POS entry mode
	msg.SetField(25, &iso8583.Field{Value: "00"})                     // POS condition code
	msg.SetField(41, &iso8583.Field{Value: g.generateTerminalID()})   // Terminal ID
	msg.SetField(42, &iso8583.Field{Value: g.generateMerchantID()})   // Merchant ID
	msg.SetField(49, &iso8583.Field{Value: "840"})                    // Currency code
	
	return msg
}

func (g *TestDataGenerator) generateMessageID() string {
	return "MSG" + g.generateDigits(10)
}

func (g *TestDataGenerator) generateSTAN() string {
	return g.generateDigits(6)
}

func (g *TestDataGenerator) generateRRN() string {
	return g.generateDigits(12)
}

func (g *TestDataGenerator) generateTerminalID() string {
	return "TERM" + g.generateDigits(4)
}

func (g *TestDataGenerator) generateMerchantID() string {
	return "MERCH" + g.generateDigits(6)
}

// Test Scenarios
var (
	// ApprovedCard always gets approved
	ApprovedCard = "4111111111111111"
	
	// DeclinedCard always gets declined
	DeclinedCard = "4000000000000002"
	
	// TimeoutCard causes timeout
	TimeoutCard = "4000000000000119"
	
	// InsufficientFundsCard gets declined with code 51
	InsufficientFundsCard = "4000000000000101"
	
	// ExpiredCard gets declined with code 54
	ExpiredCard = "4000000000000069"
	
	// FraudCard gets declined with fraud flag
	FraudCard = "4100000000000019"
)

// GenerateApprovedTransaction generates a transaction that will be approved
func (g *TestDataGenerator) GenerateApprovedTransaction() *canonical.CanonicalTransaction {
	txn := g.GenerateTransaction()
	txn.CardData.Pan = ApprovedCard
	return txn
}

// GenerateDeclinedTransaction generates a transaction that will be declined
func (g *TestDataGenerator) GenerateDeclinedTransaction() *canonical.CanonicalTransaction {
	txn := g.GenerateTransaction()
	txn.CardData.Pan = DeclinedCard
	return txn
}

// GenerateHighValueTransaction generates a high-value transaction
func (g *TestDataGenerator) GenerateHighValueTransaction() *canonical.CanonicalTransaction {
	txn := g.GenerateTransaction()
	txn.Money.Amount = 100000000 // $1,000,000
	return txn
}

// GenerateInternationalTransaction generates a cross-border transaction
func (g *TestDataGenerator) GenerateInternationalTransaction() *canonical.CanonicalTransaction {
	txn := g.GenerateTransaction()
	txn.MerchantData.MerchantCountry = "GBR"
	txn.TerminalData.CountryCode = "826"
	return txn
}

// GenerateBatchTransactions generates multiple transactions
func (g *TestDataGenerator) GenerateBatchTransactions(count int) []*canonical.CanonicalTransaction {
	transactions := make([]*canonical.CanonicalTransaction, count)
	for i := 0; i < count; i++ {
		transactions[i] = g.GenerateTransaction()
	}
	return transactions
}

// GenerateEMVData generates test EMV data
func (g *TestDataGenerator) GenerateEMVData() *canonical.EMVData {
	return &canonical.EMVData{
		ApplicationId:    []byte{0xA0, 0x00, 0x00, 0x00, 0x03, 0x10, 0x10},
		ApplicationLabel: "VISA CREDIT",
		Cryptogram:       g.generateRandomBytes(8),
		CryptogramType:   canonical.CryptogramType_CRYPTOGRAM_TYPE_ARQC,
		Atc:              uint32(g.rand.Intn(65536)),
		Cvr:              g.generateRandomBytes(2),
		IssuerApplicationData: g.generateRandomBytes(16),
	}
}

func (g *TestDataGenerator) generateRandomBytes(n int) []byte {
	bytes := make([]byte, n)
	g.rand.Read(bytes)
	return bytes
}

// MockNetworkSimulator simulates network responses
type MockNetworkSimulator struct {
	responses map[string]string // PAN -> Response code
}

// NewMockNetworkSimulator creates a mock network
func NewMockNetworkSimulator() *MockNetworkSimulator {
	return &MockNetworkSimulator{
		responses: map[string]string{
			ApprovedCard:          "00", // Approved
			DeclinedCard:          "05", // Do not honor
			InsufficientFundsCard: "51", // Insufficient funds
			ExpiredCard:           "54", // Expired card
			FraudCard:             "59", // Suspected fraud
		},
	}
}

// GetResponse returns mock response for a PAN
func (m *MockNetworkSimulator) GetResponse(pan string) string {
	if resp, exists := m.responses[pan]; exists {
		return resp
	}
	return "00" // Default to approved
}

// AddResponse adds a custom response
func (m *MockNetworkSimulator) AddResponse(pan string, responseCode string) {
	m.responses[pan] = responseCode
}
