package iso20022

import (
	"encoding/xml"
	"errors"
	"fmt"
	"time"
)

// ISO 20022 Message Parser and Generator
// Supports pain.001, pain.002, pacs.008, pacs.002, camt.053, camt.054

// MessageType represents ISO 20022 message type
type MessageType string

const (
	// Payment Initiation (pain)
	MessageTypePain001 MessageType = "pain.001.001.09" // Customer Credit Transfer Initiation
	MessageTypePain002 MessageType = "pain.002.001.10" // Customer Payment Status Report
	MessageTypePain008 MessageType = "pain.008.001.08" // Customer Direct Debit Initiation

	// Payment Clearing and Settlement (pacs)
	MessageTypePacs008 MessageType = "pacs.008.001.08" // Financial Institution Credit Transfer
	MessageTypePacs002 MessageType = "pacs.002.001.10" // Financial Institution Payment Status Report
	MessageTypePacs004 MessageType = "pacs.004.001.09" // Payment Return

	// Cash Management (camt)
	MessageTypeCamt053 MessageType = "camt.053.001.08" // Bank to Customer Statement
	MessageTypeCamt054 MessageType = "camt.054.001.08" // Bank to Customer Debit/Credit Notification
	MessageTypeCamt056 MessageType = "camt.056.001.08" // Financial Institution Cancellation Request
)

// Document represents ISO 20022 document wrapper
type Document struct {
	XMLName xml.Name `xml:"Document"`
	Xmlns   string   `xml:"xmlns,attr"`
	Message interface{}
}

// Pain001 - Customer Credit Transfer Initiation
type Pain001 struct {
	XMLName             xml.Name             `xml:"CstmrCdtTrfInitn"`
	GroupHeader         GroupHeader          `xml:"GrpHdr"`
	PaymentInformation  []PaymentInformation `xml:"PmtInf"`
}

// Pain002 - Customer Payment Status Report
type Pain002 struct {
	XMLName              xml.Name             `xml:"CstmrPmtStsRpt"`
	GroupHeader          GroupHeader          `xml:"GrpHdr"`
	OriginalGroupInfo    OriginalGroupInfo    `xml:"OrgnlGrpInfAndSts"`
	TransactionStatus    []TransactionStatus  `xml:"TxInfAndSts,omitempty"`
}

// Pacs008 - Financial Institution Credit Transfer
type Pacs008 struct {
	XMLName             xml.Name             `xml:"FIToFICstmrCdtTrf"`
	GroupHeader         GroupHeader          `xml:"GrpHdr"`
	CreditTransferInfo  []CreditTransferInfo `xml:"CdtTrfTxInf"`
}

// Pacs002 - Financial Institution Payment Status Report
type Pacs002 struct {
	XMLName              xml.Name             `xml:"FIToFIPmtStsRpt"`
	GroupHeader          GroupHeader          `xml:"GrpHdr"`
	TransactionStatus    []TransactionStatus  `xml:"TxInfAndSts"`
	OriginalGroupInfo    *OriginalGroupInfo   `xml:"OrgnlGrpInfAndSts,omitempty"`
}

// Camt053 - Bank to Customer Statement
type Camt053 struct {
	XMLName   xml.Name    `xml:"BkToCstmrStmt"`
	GroupHeader GroupHeader `xml:"GrpHdr"`
	Statement []Statement `xml:"Stmt"`
}

// Camt054 - Bank to Customer Debit/Credit Notification
type Camt054 struct {
	XMLName      xml.Name       `xml:"BkToCstmrDbtCdtNtfctn"`
	GroupHeader  GroupHeader    `xml:"GrpHdr"`
	Notification []Notification `xml:"Ntfctn"`
}

// GroupHeader - Common group header
type GroupHeader struct {
	MessageID             string    `xml:"MsgId"`
	CreationDateTime      time.Time `xml:"CreDtTm"`
	NumberOfTransactions  string    `xml:"NbOfTxs,omitempty"`
	ControlSum            string    `xml:"CtrlSum,omitempty"`
	InitiatingParty       Party     `xml:"InitgPty,omitempty"`
	ForwardingAgent       Agent     `xml:"FwdgAgt,omitempty"`
	InstructingAgent      Agent     `xml:"InstgAgt,omitempty"`
	InstructedAgent       Agent     `xml:"InstdAgt,omitempty"`
}

// PaymentInformation - Payment instruction
type PaymentInformation struct {
	PaymentInfoID          string               `xml:"PmtInfId"`
	PaymentMethod          string               `xml:"PmtMtd"`
	BatchBooking           bool                 `xml:"BtchBookg,omitempty"`
	NumberOfTransactions   string               `xml:"NbOfTxs,omitempty"`
	ControlSum             string               `xml:"CtrlSum,omitempty"`
	PaymentTypeInfo        PaymentTypeInfo      `xml:"PmtTpInf,omitempty"`
	RequestedExecutionDate string               `xml:"ReqdExctnDt"`
	Debtor                 Party                `xml:"Dbtr"`
	DebtorAccount          Account              `xml:"DbtrAcct"`
	DebtorAgent            Agent                `xml:"DbtrAgt"`
	CreditTransferTxInfo   []CreditTransferInfo `xml:"CdtTrfTxInf"`
}

// CreditTransferInfo - Credit transfer transaction
type CreditTransferInfo struct {
	PaymentID              PaymentID            `xml:"PmtId"`
	PaymentTypeInfo        PaymentTypeInfo      `xml:"PmtTpInf,omitempty"`
	Amount                 ActiveOrHistoricAmount `xml:"Amt"`
	ChargeBearer           string               `xml:"ChrgBr,omitempty"`
	UltimateDebtor         *Party               `xml:"UltmtDbtr,omitempty"`
	IntermediaryAgent      *Agent               `xml:"IntrmyAgt1,omitempty"`
	CreditorAgent          Agent                `xml:"CdtrAgt"`
	Creditor               Party                `xml:"Cdtr"`
	CreditorAccount        Account              `xml:"CdtrAcct"`
	UltimateCreditor       *Party               `xml:"UltmtCdtr,omitempty"`
	Purpose                *Purpose             `xml:"Purp,omitempty"`
	RemittanceInfo         *RemittanceInfo      `xml:"RmtInf,omitempty"`
	SupplementaryData      []SupplementaryData  `xml:"SplmtryData,omitempty"`
}

// TransactionStatus - Transaction status information
type TransactionStatus struct {
	StatusID               string              `xml:"StsId,omitempty"`
	OriginalInstructionID  string              `xml:"OrgnlInstrId,omitempty"`
	OriginalEndToEndID     string              `xml:"OrgnlEndToEndId,omitempty"`
	OriginalTxID           string              `xml:"OrgnlTxId,omitempty"`
	TransactionStatus      string              `xml:"TxSts"`
	StatusReasonInfo       []StatusReasonInfo  `xml:"StsRsnInf,omitempty"`
	ChargesInfo            []ChargesInfo       `xml:"ChrgsInf,omitempty"`
	AcceptanceDateTime     *time.Time          `xml:"AccptncDtTm,omitempty"`
	OriginalTxRef          *OriginalTxRef      `xml:"OrgnlTxRef,omitempty"`
}

// Statement - Account statement
type Statement struct {
	StatementID        string              `xml:"Id"`
	ElectronicSeqNum   string              `xml:"ElctrncSeqNb,omitempty"`
	CreationDateTime   time.Time           `xml:"CreDtTm"`
	FromToDate         DateTimePeriod      `xml:"FrToDt,omitempty"`
	Account            Account             `xml:"Acct"`
	Balance            []Balance           `xml:"Bal"`
	Entry              []Entry             `xml:"Ntry,omitempty"`
	AdditionalInfo     string              `xml:"AddtlStmtInf,omitempty"`
}

// Notification - Debit/Credit notification
type Notification struct {
	NotificationID     string         `xml:"Id"`
	CreationDateTime   time.Time      `xml:"CreDtTm"`
	Account            Account        `xml:"Acct"`
	Entry              []Entry        `xml:"Ntry"`
	AdditionalInfo     string         `xml:"AddtlNtfctnInf,omitempty"`
}

// Entry - Statement/Notification entry
type Entry struct {
	EntryReference     string              `xml:"NtryRef,omitempty"`
	Amount             ActiveOrHistoricAmount `xml:"Amt"`
	CreditDebitInd     string              `xml:"CdtDbtInd"`
	Status             string              `xml:"Sts"`
	BookingDate        *DateAndDateTime    `xml:"BookgDt,omitempty"`
	ValueDate          *DateAndDateTime    `xml:"ValDt,omitempty"`
	AccountServicerRef string              `xml:"AcctSvcrRef,omitempty"`
	BankTransactionCode BankTransactionCode `xml:"BkTxCd"`
	EntryDetails       []EntryDetails      `xml:"NtryDtls,omitempty"`
	AdditionalInfo     string              `xml:"AddtlNtryInf,omitempty"`
}

// EntryDetails - Entry transaction details
type EntryDetails struct {
	TransactionDetails []TransactionDetails `xml:"TxDtls,omitempty"`
}

// TransactionDetails - Detailed transaction information
type TransactionDetails struct {
	References         TransactionReferences `xml:"Refs,omitempty"`
	Amount             *ActiveOrHistoricAmount `xml:"Amt,omitempty"`
	CreditDebitInd     string                `xml:"CdtDbtInd,omitempty"`
	RelatedParties     *RelatedParties       `xml:"RltdPties,omitempty"`
	RelatedAgents      *RelatedAgents        `xml:"RltdAgts,omitempty"`
	Purpose            *Purpose              `xml:"Purp,omitempty"`
	RemittanceInfo     *RemittanceInfo       `xml:"RmtInf,omitempty"`
	ReturnInfo         *ReturnInfo           `xml:"RtrInf,omitempty"`
}

// Party - Party identification
type Party struct {
	Name                string               `xml:"Nm,omitempty"`
	PostalAddress       *PostalAddress       `xml:"PstlAdr,omitempty"`
	Identification      *PartyIdentification `xml:"Id,omitempty"`
	CountryOfResidence  string               `xml:"CtryOfRes,omitempty"`
	ContactDetails      *ContactDetails      `xml:"CtctDtls,omitempty"`
}

// Agent - Financial institution
type Agent struct {
	FinancialInstitutionID FinancialInstitutionID `xml:"FinInstnId"`
	BranchID               *BranchID              `xml:"BrnchId,omitempty"`
}

// Account - Account identification
type Account struct {
	Identification AccountIdentification `xml:"Id"`
	Type           *AccountType          `xml:"Tp,omitempty"`
	Currency       string                `xml:"Ccy,omitempty"`
	Name           string                `xml:"Nm,omitempty"`
	Owner          *Party                `xml:"Ownr,omitempty"`
	Servicer       *Agent                `xml:"Svcr,omitempty"`
}

// AccountIdentification - Account ID
type AccountIdentification struct {
	IBAN  string `xml:"IBAN,omitempty"`
	Other *OtherAccountID `xml:"Othr,omitempty"`
}

// OtherAccountID - Other account identification
type OtherAccountID struct {
	Identification string `xml:"Id"`
	SchemeName     string `xml:"SchmeNm,omitempty"`
	Issuer         string `xml:"Issr,omitempty"`
}

// FinancialInstitutionID - Financial institution identification
type FinancialInstitutionID struct {
	BICFI          string         `xml:"BICFI,omitempty"`
	ClearingSystemID *ClearingSystemID `xml:"ClrSysMmbId,omitempty"`
	Name           string         `xml:"Nm,omitempty"`
	PostalAddress  *PostalAddress `xml:"PstlAdr,omitempty"`
	Other          *GenericID     `xml:"Othr,omitempty"`
}

// ClearingSystemID - Clearing system member identification
type ClearingSystemID struct {
	ClearingSystemID string `xml:"ClrSysId,omitempty"`
	MemberID         string `xml:"MmbId"`
}

// PaymentID - Payment identification
type PaymentID struct {
	InstructionID    string `xml:"InstrId,omitempty"`
	EndToEndID       string `xml:"EndToEndId"`
	TransactionID    string `xml:"TxId,omitempty"`
	UETR             string `xml:"UETR,omitempty"`
}

// ActiveOrHistoricAmount - Amount with currency
type ActiveOrHistoricAmount struct {
	Value    string `xml:",chardata"`
	Currency string `xml:"Ccy,attr"`
}

// PaymentTypeInfo - Payment type information
type PaymentTypeInfo struct {
	InstructionPriority string          `xml:"InstrPrty,omitempty"`
	ServiceLevel        *ServiceLevel   `xml:"SvcLvl,omitempty"`
	LocalInstrument     *LocalInstrument `xml:"LclInstrm,omitempty"`
	CategoryPurpose     *CategoryPurpose `xml:"CtgyPurp,omitempty"`
}

// RemittanceInfo - Remittance information
type RemittanceInfo struct {
	Unstructured []string              `xml:"Ustrd,omitempty"`
	Structured   []StructuredRemittance `xml:"Strd,omitempty"`
}

// StructuredRemittance - Structured remittance information
type StructuredRemittance struct {
	ReferredDocInfo []ReferredDocInfo `xml:"RfrdDocInf,omitempty"`
	AdditionalInfo  string            `xml:"AddtlRmtInf,omitempty"`
}

// ReferredDocInfo - Referred document information
type ReferredDocInfo struct {
	Type   *DocumentType `xml:"Tp,omitempty"`
	Number string        `xml:"Nb,omitempty"`
	Date   string        `xml:"RltdDt,omitempty"`
}

// Balance - Account balance
type Balance struct {
	Type             BalanceType            `xml:"Tp"`
	Amount           ActiveOrHistoricAmount `xml:"Amt"`
	CreditDebitInd   string                 `xml:"CdtDbtInd"`
	Date             DateAndDateTime        `xml:"Dt"`
	Availability     []Availability         `xml:"Avlbty,omitempty"`
}

// BalanceType - Balance type
type BalanceType struct {
	CodeOrProprietary CodeOrProprietary `xml:"CdOrPrtry"`
}

// CodeOrProprietary - Code or proprietary format
type CodeOrProprietary struct {
	Code        string `xml:"Cd,omitempty"`
	Proprietary string `xml:"Prtry,omitempty"`
}

// DateAndDateTime - Date and optional time
type DateAndDateTime struct {
	Date     string     `xml:"Dt,omitempty"`
	DateTime *time.Time `xml:"DtTm,omitempty"`
}

// DateTimePeriod - Date/time period
type DateTimePeriod struct {
	FromDateTime time.Time `xml:"FrDtTm"`
	ToDateTime   time.Time `xml:"ToDtTm"`
}

// BankTransactionCode - Bank transaction code
type BankTransactionCode struct {
	Domain    *BankTransactionCodeDomain `xml:"Domn,omitempty"`
	Proprietary string                   `xml:"Prtry,omitempty"`
}

// BankTransactionCodeDomain - Transaction code domain
type BankTransactionCodeDomain struct {
	Code   string                  `xml:"Cd"`
	Family *BankTransactionCodeFamily `xml:"Fmly"`
}

// BankTransactionCodeFamily - Transaction code family
type BankTransactionCodeFamily struct {
	Code        string `xml:"Cd"`
	SubFamilyCode string `xml:"SubFmlyCd"`
}

// Supporting types
type PostalAddress struct {
	AddressType      string   `xml:"AdrTp,omitempty"`
	AddressLine      []string `xml:"AdrLine,omitempty"`
	StreetName       string   `xml:"StrtNm,omitempty"`
	BuildingNumber   string   `xml:"BldgNb,omitempty"`
	PostCode         string   `xml:"PstCd,omitempty"`
	TownName         string   `xml:"TwnNm,omitempty"`
	Country          string   `xml:"Ctry,omitempty"`
}

type PartyIdentification struct {
	OrganisationID *OrganisationID `xml:"OrgId,omitempty"`
	PrivateID      *PrivateID      `xml:"PrvtId,omitempty"`
}

type OrganisationID struct {
	BICOrBEI string       `xml:"BICOrBEI,omitempty"`
	Other    []GenericID  `xml:"Othr,omitempty"`
}

type PrivateID struct {
	DateAndPlaceOfBirth *DateAndPlaceOfBirth `xml:"DtAndPlcOfBirth,omitempty"`
	Other               []GenericID          `xml:"Othr,omitempty"`
}

type DateAndPlaceOfBirth struct {
	BirthDate      string `xml:"BirthDt"`
	CityOfBirth    string `xml:"CityOfBirth"`
	CountryOfBirth string `xml:"CtryOfBirth"`
}

type GenericID struct {
	Identification string `xml:"Id"`
	SchemeName     string `xml:"SchmeNm,omitempty"`
	Issuer         string `xml:"Issr,omitempty"`
}

type ContactDetails struct {
	NamePrefix  string `xml:"NmPrfx,omitempty"`
	Name        string `xml:"Nm,omitempty"`
	PhoneNumber string `xml:"PhneNb,omitempty"`
	MobileNumber string `xml:"MobNb,omitempty"`
	EmailAddress string `xml:"EmailAdr,omitempty"`
}

type AccountType struct {
	Code        string `xml:"Cd,omitempty"`
	Proprietary string `xml:"Prtry,omitempty"`
}

type BranchID struct {
	Identification string         `xml:"Id,omitempty"`
	Name           string         `xml:"Nm,omitempty"`
	PostalAddress  *PostalAddress `xml:"PstlAdr,omitempty"`
}

type ServiceLevel struct {
	Code        string `xml:"Cd,omitempty"`
	Proprietary string `xml:"Prtry,omitempty"`
}

type LocalInstrument struct {
	Code        string `xml:"Cd,omitempty"`
	Proprietary string `xml:"Prtry,omitempty"`
}

type CategoryPurpose struct {
	Code        string `xml:"Cd,omitempty"`
	Proprietary string `xml:"Prtry,omitempty"`
}

type Purpose struct {
	Code        string `xml:"Cd,omitempty"`
	Proprietary string `xml:"Prtry,omitempty"`
}

type DocumentType struct {
	CodeOrProprietary CodeOrProprietary `xml:"CdOrPrtry"`
}

type OriginalGroupInfo struct {
	OriginalMessageID   string    `xml:"OrgnlMsgId"`
	OriginalMessageNameID string  `xml:"OrgnlMsgNmId"`
	GroupStatus         string    `xml:"GrpSts,omitempty"`
	StatusReasonInfo    []StatusReasonInfo `xml:"StsRsnInf,omitempty"`
}

type StatusReasonInfo struct {
	Originator  *Party  `xml:"Orgtr,omitempty"`
	Reason      *Reason `xml:"Rsn,omitempty"`
	AdditionalInfo []string `xml:"AddtlInf,omitempty"`
}

type Reason struct {
	Code        string `xml:"Cd,omitempty"`
	Proprietary string `xml:"Prtry,omitempty"`
}

type ChargesInfo struct {
	Amount        ActiveOrHistoricAmount `xml:"Amt"`
	Agent         Agent                  `xml:"Agt"`
	Type          *ChargeType            `xml:"Tp,omitempty"`
}

type ChargeType struct {
	Code        string `xml:"Cd,omitempty"`
	Proprietary string `xml:"Prtry,omitempty"`
}

type OriginalTxRef struct {
	Amount              *ActiveOrHistoricAmount `xml:"Amt,omitempty"`
	RequestedExecution  *DateAndDateTime        `xml:"ReqdExctnDt,omitempty"`
	CreditorAgent       *Agent                  `xml:"CdtrAgt,omitempty"`
	Creditor            *Party                  `xml:"Cdtr,omitempty"`
	CreditorAccount     *Account                `xml:"CdtrAcct,omitempty"`
	DebtorAgent         *Agent                  `xml:"DbtrAgt,omitempty"`
	Debtor              *Party                  `xml:"Dbtr,omitempty"`
	DebtorAccount       *Account                `xml:"DbtrAcct,omitempty"`
}

type TransactionReferences struct {
	MessageID         string `xml:"MsgId,omitempty"`
	AccountServicerRef string `xml:"AcctSvcrRef,omitempty"`
	PaymentInfoID     string `xml:"PmtInfId,omitempty"`
	InstructionID     string `xml:"InstrId,omitempty"`
	EndToEndID        string `xml:"EndToEndId,omitempty"`
	TransactionID     string `xml:"TxId,omitempty"`
	MandateID         string `xml:"MndtId,omitempty"`
	ChequeNumber      string `xml:"ChqNb,omitempty"`
}

type RelatedParties struct {
	Debtor           *Party   `xml:"Dbtr,omitempty"`
	DebtorAccount    *Account `xml:"DbtrAcct,omitempty"`
	UltimateDebtor   *Party   `xml:"UltmtDbtr,omitempty"`
	Creditor         *Party   `xml:"Cdtr,omitempty"`
	CreditorAccount  *Account `xml:"CdtrAcct,omitempty"`
	UltimateCreditor *Party   `xml:"UltmtCdtr,omitempty"`
}

type RelatedAgents struct {
	DebtorAgent       *Agent `xml:"DbtrAgt,omitempty"`
	CreditorAgent     *Agent `xml:"CdtrAgt,omitempty"`
	IntermediaryAgent *Agent `xml:"IntrmyAgt,omitempty"`
}

type ReturnInfo struct {
	OriginalBankTxCode *BankTransactionCode `xml:"OrgnlBkTxCd,omitempty"`
	Originator         *Party               `xml:"Orgtr,omitempty"`
	Reason             *Reason              `xml:"Rsn,omitempty"`
	AdditionalInfo     []string             `xml:"AddtlInf,omitempty"`
}

type Availability struct {
	Date   DateAndDateTime        `xml:"Dt"`
	Amount ActiveOrHistoricAmount `xml:"Amt"`
}

type SupplementaryData struct {
	PlaceAndName string      `xml:"PlcAndNm,omitempty"`
	Envelope     interface{} `xml:"Envlp"`
}

// Parser handles ISO 20022 message parsing
type Parser struct {
	strict bool
}

// NewParser creates a new ISO 20022 parser
func NewParser(strict bool) *Parser {
	return &Parser{strict: strict}
}

// Parse parses ISO 20022 XML message
func (p *Parser) Parse(data []byte, msgType MessageType) (interface{}, error) {
	var doc Document

	switch msgType {
	case MessageTypePain001:
		doc.Message = &Pain001{}
	case MessageTypePain002:
		doc.Message = &Pain002{}
	case MessageTypePacs008:
		doc.Message = &Pacs008{}
	case MessageTypePacs002:
		doc.Message = &Pacs002{}
	case MessageTypeCamt053:
		doc.Message = &Camt053{}
	case MessageTypeCamt054:
		doc.Message = &Camt054{}
	default:
		return nil, fmt.Errorf("unsupported message type: %s", msgType)
	}

	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse XML: %w", err)
	}

	return doc.Message, nil
}

// Generate generates ISO 20022 XML message
func (p *Parser) Generate(message interface{}, msgType MessageType) ([]byte, error) {
	if message == nil {
		return nil, errors.New("message is nil")
	}

	doc := Document{
		Xmlns:   p.getNamespace(msgType),
		Message: message,
	}

	data, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to generate XML: %w", err)
	}

	return append([]byte(xml.Header), data...), nil
}

// getNamespace returns namespace for message type
func (p *Parser) getNamespace(msgType MessageType) string {
	switch msgType {
	case MessageTypePain001, MessageTypePain002, MessageTypePain008:
		return "urn:iso:std:iso:20022:tech:xsd:pain.001.001.09"
	case MessageTypePacs008, MessageTypePacs002, MessageTypePacs004:
		return "urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"
	case MessageTypeCamt053, MessageTypeCamt054, MessageTypeCamt056:
		return "urn:iso:std:iso:20022:tech:xsd:camt.053.001.08"
	default:
		return "urn:iso:std:iso:20022:tech:xsd"
	}
}

// Validate validates ISO 20022 message
func (p *Parser) Validate(message interface{}) error {
	if message == nil {
		return errors.New("message is nil")
	}

	// Basic validation (extend as needed)
	switch msg := message.(type) {
	case *Pain001:
		return p.validatePain001(msg)
	case *Pacs008:
		return p.validatePacs008(msg)
	case *Camt053:
		return p.validateCamt053(msg)
	default:
		return nil
	}
}

func (p *Parser) validatePain001(msg *Pain001) error {
	if msg.GroupHeader.MessageID == "" {
		return errors.New("message ID is required")
	}
	if len(msg.PaymentInformation) == 0 {
		return errors.New("at least one payment information is required")
	}
	return nil
}

func (p *Parser) validatePacs008(msg *Pacs008) error {
	if msg.GroupHeader.MessageID == "" {
		return errors.New("message ID is required")
	}
	if len(msg.CreditTransferInfo) == 0 {
		return errors.New("at least one credit transfer is required")
	}
	return nil
}

func (p *Parser) validateCamt053(msg *Camt053) error {
	if msg.GroupHeader.MessageID == "" {
		return errors.New("message ID is required")
	}
	if len(msg.Statement) == 0 {
		return errors.New("at least one statement is required")
	}
	return nil
}
