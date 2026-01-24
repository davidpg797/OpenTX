package mapper

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/krish567366/OpenTX/pkg/iso8583"
)

// Mapper converts between ISO 8583 messages and canonical transaction model
type Mapper interface {
	// ToCanonical converts ISO 8583 message to canonical format
	ToCanonical(isoMsg *iso8583.Message) (*CanonicalTransaction, error)
	
	// FromCanonical converts canonical message to ISO 8583 format
	FromCanonical(canonical *CanonicalTransaction) (*iso8583.Message, error)
}

// GenericMapper implements bidirectional mapping
type GenericMapper struct {
	variant      string
	mappingRules map[string]FieldMappingRule
}

// FieldMappingRule defines how to map a field
type FieldMappingRule struct {
	ISOField      int
	CanonicalPath string
	Transform     TransformFunc
	ReverseTransform TransformFunc
}

// TransformFunc transforms field value during mapping
type TransformFunc func(value interface{}) (interface{}, error)

// NewGenericMapper creates a new mapper
func NewGenericMapper(variant string) *GenericMapper {
	return &GenericMapper{
		variant:      variant,
		mappingRules: buildDefaultMappingRules(),
	}
}

// ToCanonical converts ISO 8583 to canonical format
func (m *GenericMapper) ToCanonical(isoMsg *iso8583.Message) (*CanonicalTransaction, error) {
	canonical := &CanonicalTransaction{
		SchemaVersion:  "1.0.0",
		MessageID:      uuid.New().String(),
		CorrelationID:  uuid.New().String(),
		Metadata:       &MessageMetadata{},
		State:          &TransactionState{},
	}
	
	// Map MTI
	canonical.Metadata.MTI = isoMsg.MTI
	canonical.Metadata.Network = isoMsg.Variant
	
	// Determine transaction type from MTI
	txnType, err := m.getTransactionTypeFromMTI(isoMsg.MTI)
	if err != nil {
		return nil, err
	}
	
	// Map fields based on transaction type
	switch txnType {
	case "authorization_request":
		authReq, err := m.mapAuthorizationRequest(isoMsg)
		if err != nil {
			return nil, err
		}
		canonical.TransactionType = &CanonicalTransaction_AuthRequest{AuthRequest: authReq}
		
	case "authorization_response":
		authResp, err := m.mapAuthorizationResponse(isoMsg)
		if err != nil {
			return nil, err
		}
		canonical.TransactionType = &CanonicalTransaction_AuthResponse{AuthResponse: authResp}
		
	case "reversal_request":
		revReq, err := m.mapReversalRequest(isoMsg)
		if err != nil {
			return nil, err
		}
		canonical.TransactionType = &CanonicalTransaction_ReversalRequest{ReversalRequest: revReq}
		
	default:
		return nil, fmt.Errorf("unsupported transaction type: %s", txnType)
	}
	
	// Map common metadata fields
	if err := m.mapMetadata(isoMsg, canonical.Metadata); err != nil {
		return nil, err
	}
	
	// Generate idempotency key
	canonical.IdempotencyKey = m.generateIdempotencyKey(isoMsg)
	
	// Initialize state
	canonical.State.Status = TransactionStatus_STATUS_INIT
	canonical.State.CreatedAt = timestampNow()
	canonical.State.UpdatedAt = timestampNow()
	
	return canonical, nil
}

// FromCanonical converts canonical format to ISO 8583
func (m *GenericMapper) FromCanonical(canonical *CanonicalTransaction) (*iso8583.Message, error) {
	isoMsg := iso8583.NewMessage(canonical.Metadata.MTI, canonical.Metadata.Network)
	
	// Map based on transaction type
	switch txn := canonical.TransactionType.(type) {
	case *CanonicalTransaction_AuthRequest:
		if err := m.mapAuthRequestToISO(txn.AuthRequest, canonical.Metadata, isoMsg); err != nil {
			return nil, err
		}
	case *CanonicalTransaction_AuthResponse:
		if err := m.mapAuthResponseToISO(txn.AuthResponse, canonical.Metadata, isoMsg); err != nil {
			return nil, err
		}
	case *CanonicalTransaction_ReversalRequest:
		if err := m.mapReversalRequestToISO(txn.ReversalRequest, canonical.Metadata, isoMsg); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported transaction type")
	}
	
	// Map common metadata
	if err := m.mapMetadataToISO(canonical.Metadata, isoMsg); err != nil {
		return nil, err
	}
	
	return isoMsg, nil
}

// mapAuthorizationRequest maps ISO to canonical auth request
func (m *GenericMapper) mapAuthorizationRequest(isoMsg *iso8583.Message) (*AuthorizationRequest, error) {
	authReq := &AuthorizationRequest{}
	
	// Map amounts
	if amount := isoMsg.GetFieldAsString(4); amount != "" {
		authReq.TransactionAmount = parseAmount(amount, isoMsg.GetFieldAsString(49))
	}
	
	// Map card data
	authReq.Card = &CardData{}
	if pan := isoMsg.GetFieldAsString(2); pan != "" {
		authReq.Card.Identifier = &CardData_Pan{Pan: pan}
	}
	if expiry := isoMsg.GetFieldAsString(14); expiry != "" {
		authReq.Card.ExpiryDate = expiry
	}
	
	// Map terminal data
	authReq.Terminal = &TerminalData{
		TerminalId: isoMsg.GetFieldAsString(41),
		MerchantId: isoMsg.GetFieldAsString(42),
	}
	
	// Map merchant data
	if merchantInfo := isoMsg.GetFieldAsString(43); merchantInfo != "" {
		authReq.Merchant = parseMerchantInfo(merchantInfo)
	}
	
	// Map POS data
	authReq.Pos = &POSData{}
	if posEntry := isoMsg.GetFieldAsString(22); posEntry != "" {
		authReq.Terminal.EntryMode = parsePOSEntryMode(posEntry)
	}
	if posCondition := isoMsg.GetFieldAsString(25); posCondition != "" {
		authReq.Pos.ConditionCode = parsePOSConditionCode(posCondition)
	}
	
	// Map EMV data (DE55)
	if emvData := isoMsg.GetFieldAsBytes(55); len(emvData) > 0 {
		authReq.Emv = parseEMVData(emvData)
	}
	
	// Map acquirer IDs
	authReq.AcquirerId = isoMsg.GetFieldAsString(32)
	authReq.ForwardingInstitutionId = isoMsg.GetFieldAsString(33)
	
	return authReq, nil
}

// mapAuthorizationResponse maps ISO to canonical auth response
func (m *GenericMapper) mapAuthorizationResponse(isoMsg *iso8583.Message) (*AuthorizationResponse, error) {
	authResp := &AuthorizationResponse{}
	
	// Map response code
	if respCode := isoMsg.GetFieldAsString(39); respCode != "" {
		authResp.Response = &ResponseCode{
			Code:        respCode,
			Description: getResponseDescription(respCode),
			Category:    categorizeResponseCode(respCode),
		}
	}
	
	// Map auth ID
	authResp.AuthIdCode = isoMsg.GetFieldAsString(38)
	
	// Map amounts
	if amount := isoMsg.GetFieldAsString(4); amount != "" {
		authResp.ApprovedAmount = parseAmount(amount, isoMsg.GetFieldAsString(49))
	}
	
	// Parse additional amounts (DE54)
	if addAmounts := isoMsg.GetFieldAsString(54); addAmounts != "" {
		authResp.Balances = parseAdditionalAmounts(addAmounts)
	}
	
	return authResp, nil
}

// mapReversalRequest maps ISO to canonical reversal request
func (m *GenericMapper) mapReversalRequest(isoMsg *iso8583.Message) (*ReversalRequest, error) {
	revReq := &ReversalRequest{}
	
	// Parse original data elements (DE90)
	if origData := isoMsg.GetFieldAsString(90); origData != "" {
		if len(origData) >= 42 {
			revReq.OriginalMti = origData[0:4]
			revReq.OriginalStan = origData[4:10]
			revReq.OriginalTransmissionDatetime = origData[10:20]
			revReq.OriginalAcquirerId = origData[20:31]
		}
	}
	
	// Map reversal amount
	if amount := isoMsg.GetFieldAsString(4); amount != "" {
		revReq.ReversalAmount = parseAmount(amount, isoMsg.GetFieldAsString(49))
	}
	
	// Map reason from DE60
	if reasonCode := isoMsg.GetFieldAsString(60); reasonCode != "" {
		revReq.Reason = parseReversalReason(reasonCode)
	}
	
	// Map card and terminal for validation
	revReq.Card = &CardData{}
	if pan := isoMsg.GetFieldAsString(2); pan != "" {
		revReq.Card.Identifier = &CardData_Pan{Pan: pan}
	}
	
	revReq.Terminal = &TerminalData{
		TerminalId: isoMsg.GetFieldAsString(41),
		MerchantId: isoMsg.GetFieldAsString(42),
	}
	
	return revReq, nil
}

// mapMetadata maps common metadata fields
func (m *GenericMapper) mapMetadata(isoMsg *iso8583.Message, metadata *MessageMetadata) error {
	// Processing code
	if procCode := isoMsg.GetFieldAsString(3); procCode != "" {
		metadata.ProcessingCode = parseProcessingCode(procCode)
	}
	
	// STAN
	metadata.Stan = isoMsg.GetFieldAsString(11)
	
	// RRN
	metadata.Rrn = isoMsg.GetFieldAsString(37)
	
	// Transmission datetime
	if transmitDT := isoMsg.GetFieldAsString(7); transmitDT != "" {
		metadata.TransmissionDatetime = parseISO8583DateTime(transmitDT)
	}
	
	// Local transaction datetime
	localDate := isoMsg.GetFieldAsString(13)
	localTime := isoMsg.GetFieldAsString(12)
	if localDate != "" && localTime != "" {
		metadata.LocalTransactionDatetime = parseISO8583LocalDateTime(localDate, localTime)
	}
	
	return nil
}

// Helper functions for mapping

func parseAmount(amountStr, currencyCode string) *Money {
	amount, _ := strconv.ParseInt(strings.TrimSpace(amountStr), 10, 64)
	return &Money{
		Amount:         amount,
		CurrencyCode:   currencyCode,
		DecimalPlaces:  2, // Default to 2
		Sign:           Sign_SIGN_POSITIVE,
	}
}

func parseMerchantInfo(merchantInfo string) *MerchantData {
	// Merchant info format: NAME\CITY
	parts := strings.Split(merchantInfo, "\\")
	merchant := &MerchantData{}
	if len(parts) > 0 {
		merchant.MerchantName = strings.TrimSpace(parts[0])
	}
	if len(parts) > 1 {
		merchant.MerchantCity = strings.TrimSpace(parts[1])
	}
	return merchant
}

func parsePOSEntryMode(entryMode string) POSEntryMode {
	// First 2 digits indicate entry mode
	if len(entryMode) < 2 {
		return POSEntryMode_POS_ENTRY_UNKNOWN
	}
	
	switch entryMode[0:2] {
	case "01", "02":
		return POSEntryMode_POS_ENTRY_MANUAL
	case "05":
		return POSEntryMode_POS_ENTRY_CHIP
	case "07":
		return POSEntryMode_POS_ENTRY_CONTACTLESS
	case "90", "91":
		return POSEntryMode_POS_ENTRY_MAGNETIC_STRIPE
	case "81":
		return POSEntryMode_POS_ENTRY_ECOMMERCE
	default:
		return POSEntryMode_POS_ENTRY_UNKNOWN
	}
}

func parsePOSConditionCode(condCode string) POSConditionCode {
	switch condCode {
	case "00":
		return POSConditionCode_POS_CONDITION_NORMAL
	case "01":
		return POSConditionCode_POS_CONDITION_CUSTOMER_NOT_PRESENT
	case "08":
		return POSConditionCode_POS_CONDITION_MAIL_ORDER
	case "59":
		return POSConditionCode_POS_CONDITION_ECOMMERCE
	default:
		return POSConditionCode_POS_CONDITION_UNKNOWN
	}
}

func parseProcessingCode(procCode string) *ProcessingCode {
	if len(procCode) != 6 {
		return &ProcessingCode{Code: procCode}
	}
	
	return &ProcessingCode{
		Code:            procCode,
		TransactionType: parseTransactionType(procCode[0:2]),
		FromAccount:     parseAccountType(procCode[2:4]),
		ToAccount:       parseAccountType(procCode[4:6]),
	}
}

func parseTransactionType(code string) TransactionType {
	switch code {
	case "00":
		return TransactionType_TXN_PURCHASE
	case "01":
		return TransactionType_TXN_CASH_WITHDRAWAL
	case "20":
		return TransactionType_TXN_REFUND
	case "30":
		return TransactionType_TXN_BALANCE_INQUIRY
	default:
		return TransactionType_TXN_UNKNOWN
	}
}

func parseAccountType(code string) AccountType {
	switch code {
	case "00":
		return AccountType_ACCOUNT_DEFAULT
	case "10":
		return AccountType_ACCOUNT_SAVINGS
	case "20":
		return AccountType_ACCOUNT_CHECKING
	case "30":
		return AccountType_ACCOUNT_CREDIT
	default:
		return AccountType_ACCOUNT_UNKNOWN
	}
}

func parseReversalReason(reasonCode string) ReversalReason {
	switch reasonCode {
	case "4021":
		return ReversalReason_REVERSAL_TIMEOUT
	case "4022":
		return ReversalReason_REVERSAL_CUSTOMER_CANCELLATION
	default:
		return ReversalReason_REVERSAL_UNKNOWN
	}
}

func getResponseDescription(code string) string {
	descriptions := map[string]string{
		"00": "Approved",
		"01": "Refer to card issuer",
		"05": "Do not honor",
		"14": "Invalid card number",
		"51": "Insufficient funds",
		"54": "Expired card",
		"55": "Incorrect PIN",
		"91": "Issuer unavailable",
	}
	if desc, exists := descriptions[code]; exists {
		return desc
	}
	return "Unknown"
}

func categorizeResponseCode(code string) ResponseCategory {
	if code == "00" {
		return ResponseCategory_RESPONSE_APPROVED
	}
	if code == "01" {
		return ResponseCategory_RESPONSE_REFERRAL
	}
	if code[0] == '0' || code[0] == '1' {
		return ResponseCategory_RESPONSE_APPROVED
	}
	return ResponseCategory_RESPONSE_DECLINED
}

func parseAdditionalAmounts(data string) []*BalanceInfo {
	// Additional amounts format: Account Type, Amount Type, Currency, Amount
	// Each record is 20 bytes
	balances := make([]*BalanceInfo, 0)
	
	for i := 0; i < len(data); i += 20 {
		if i+20 > len(data) {
			break
		}
		
		record := data[i : i+20]
		balances = append(balances, &BalanceInfo{
			AccountType: parseAccountType(record[0:2]),
			AmountType:  parseAmountType(record[2:4]),
			CurrencyCode: record[4:7],
			Amount:      parseAmount(record[8:20], record[4:7]),
		})
	}
	
	return balances
}

func parseAmountType(code string) AmountType {
	switch code {
	case "01":
		return AmountType_AMOUNT_TYPE_LEDGER
	case "02":
		return AmountType_AMOUNT_TYPE_AVAILABLE
	case "03":
		return AmountType_AMOUNT_TYPE_CREDIT_LIMIT
	default:
		return AmountType_AMOUNT_TYPE_UNKNOWN
	}
}

func parseISO8583DateTime(datetime string) *Timestamp {
	// Format: MMDDhhmmss
	if len(datetime) != 10 {
		return timestampNow()
	}
	
	month, _ := strconv.Atoi(datetime[0:2])
	day, _ := strconv.Atoi(datetime[2:4])
	hour, _ := strconv.Atoi(datetime[4:6])
	minute, _ := strconv.Atoi(datetime[6:8])
	second, _ := strconv.Atoi(datetime[8:10])
	
	now := time.Now()
	t := time.Date(now.Year(), time.Month(month), day, hour, minute, second, 0, time.UTC)
	
	return &Timestamp{
		Seconds: t.Unix(),
		Nanos:   0,
	}
}

func parseISO8583LocalDateTime(date, timeStr string) *Timestamp {
	// Date: MMDD, Time: hhmmss
	if len(date) != 4 || len(timeStr) != 6 {
		return timestampNow()
	}
	
	month, _ := strconv.Atoi(date[0:2])
	day, _ := strconv.Atoi(date[2:4])
	hour, _ := strconv.Atoi(timeStr[0:2])
	minute, _ := strconv.Atoi(timeStr[2:4])
	second, _ := strconv.Atoi(timeStr[4:6])
	
	now := time.Now()
	t := time.Date(now.Year(), time.Month(month), day, hour, minute, second, 0, time.Local)
	
	return &Timestamp{
		Seconds: t.Unix(),
		Nanos:   0,
	}
}

func parseEMVData(data []byte) *EMVData {
	// This would use a full TLV parser - simplified here
	return &EMVData{
		RawEmvData: data,
	}
}

func timestampNow() *Timestamp {
	now := time.Now()
	return &Timestamp{
		Seconds: now.Unix(),
		Nanos:   int32(now.Nanosecond()),
	}
}

func (m *GenericMapper) getTransactionTypeFromMTI(mti string) (string, error) {
	if len(mti) != 4 {
		return "", fmt.Errorf("invalid MTI length")
	}
	
	msgClass := mti[2:3]
	
	switch mti[0:2] {
	case "01":
		if msgClass == "0" {
			return "authorization_request", nil
		}
		return "authorization_response", nil
	case "04":
		if msgClass == "0" {
			return "reversal_request", nil
		}
		return "reversal_response", nil
	case "02":
		if msgClass == "0" {
			return "advice_request", nil
		}
		return "advice_response", nil
	default:
		return "", fmt.Errorf("unsupported MTI: %s", mti)
	}
}

func (m *GenericMapper) generateIdempotencyKey(isoMsg *iso8583.Message) string {
	// Combine network, STAN, RRN, amount for uniqueness
	return fmt.Sprintf("%s:%s:%s:%s",
		isoMsg.Variant,
		isoMsg.GetFieldAsString(11), // STAN
		isoMsg.GetFieldAsString(37), // RRN
		isoMsg.GetFieldAsString(4),  // Amount
	)
}

func buildDefaultMappingRules() map[string]FieldMappingRule {
	return map[string]FieldMappingRule{
		"pan":        {ISOField: 2, CanonicalPath: "card.pan"},
		"proc_code":  {ISOField: 3, CanonicalPath: "metadata.processing_code"},
		"amount":     {ISOField: 4, CanonicalPath: "transaction_amount"},
		"stan":       {ISOField: 11, CanonicalPath: "metadata.stan"},
		"expiry":     {ISOField: 14, CanonicalPath: "card.expiry_date"},
		"rrn":        {ISOField: 37, CanonicalPath: "metadata.rrn"},
		"response":   {ISOField: 39, CanonicalPath: "response.code"},
		"terminal":   {ISOField: 41, CanonicalPath: "terminal.terminal_id"},
		"merchant":   {ISOField: 42, CanonicalPath: "terminal.merchant_id"},
	}
}

// Reverse mapping functions (FromCanonical implementations)

func (m *GenericMapper) mapAuthRequestToISO(authReq *AuthorizationRequest, metadata *MessageMetadata, isoMsg *iso8583.Message) error {
	// Map PAN
	if authReq.Card != nil {
		if pan, ok := authReq.Card.Identifier.(*CardData_Pan); ok {
			isoMsg.SetField(2, pan.Pan)
		}
		if authReq.Card.ExpiryDate != "" {
			isoMsg.SetField(14, authReq.Card.ExpiryDate)
		}
	}
	
	// Map processing code
	if metadata.ProcessingCode != nil {
		isoMsg.SetField(3, metadata.ProcessingCode.Code)
	}
	
	// Map amount
	if authReq.TransactionAmount != nil {
		amountStr := fmt.Sprintf("%012d", authReq.TransactionAmount.Amount)
		isoMsg.SetField(4, amountStr)
		isoMsg.SetField(49, authReq.TransactionAmount.CurrencyCode)
	}
	
	// Map terminal
	if authReq.Terminal != nil {
		isoMsg.SetField(41, authReq.Terminal.TerminalId)
		isoMsg.SetField(42, authReq.Terminal.MerchantId)
	}
	
	// Map merchant
	if authReq.Merchant != nil {
		merchantInfo := fmt.Sprintf("%s\\%s", authReq.Merchant.MerchantName, authReq.Merchant.MerchantCity)
		isoMsg.SetField(43, merchantInfo)
	}
	
	return nil
}

func (m *GenericMapper) mapAuthResponseToISO(authResp *AuthorizationResponse, metadata *MessageMetadata, isoMsg *iso8583.Message) error {
	// Map response code
	if authResp.Response != nil {
		isoMsg.SetField(39, authResp.Response.Code)
	}
	
	// Map auth ID
	if authResp.AuthIdCode != "" {
		isoMsg.SetField(38, authResp.AuthIdCode)
	}
	
	// Map amount
	if authResp.ApprovedAmount != nil {
		amountStr := fmt.Sprintf("%012d", authResp.ApprovedAmount.Amount)
		isoMsg.SetField(4, amountStr)
	}
	
	return nil
}

func (m *GenericMapper) mapReversalRequestToISO(revReq *ReversalRequest, metadata *MessageMetadata, isoMsg *iso8583.Message) error {
	// Build DE90 (Original Data Elements)
	origData := fmt.Sprintf("%s%s%s%s",
		revReq.OriginalMti,
		revReq.OriginalStan,
		revReq.OriginalTransmissionDatetime,
		revReq.OriginalAcquirerId,
	)
	isoMsg.SetField(90, origData)
	
	// Map amount
	if revReq.ReversalAmount != nil {
		amountStr := fmt.Sprintf("%012d", revReq.ReversalAmount.Amount)
		isoMsg.SetField(4, amountStr)
	}
	
	return nil
}

func (m *GenericMapper) mapMetadataToISO(metadata *MessageMetadata, isoMsg *iso8583.Message) error {
	// Map STAN
	if metadata.Stan != "" {
		isoMsg.SetField(11, metadata.Stan)
	}
	
	// Map RRN
	if metadata.Rrn != "" {
		isoMsg.SetField(37, metadata.Rrn)
	}
	
	// Map transmission datetime
	if metadata.TransmissionDatetime != nil {
		t := time.Unix(metadata.TransmissionDatetime.Seconds, 0)
		datetime := t.Format("0102150405") // MMDDhhmmss
		isoMsg.SetField(7, datetime)
	}
	
	// Map local transaction datetime
	if metadata.LocalTransactionDatetime != nil {
		t := time.Unix(metadata.LocalTransactionDatetime.Seconds, 0)
		isoMsg.SetField(12, t.Format("150405")) // hhmmss
		isoMsg.SetField(13, t.Format("0102"))   // MMDD
	}
	
	return nil
}
