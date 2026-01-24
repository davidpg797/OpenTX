package emv

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// TLVParser parses EMV TLV (Tag-Length-Value) data
type TLVParser struct {
	tags map[string]string // tag -> description
}

// TLV represents a single TLV element
type TLV struct {
	Tag         string
	Length      int
	Value       []byte
	Description string
	Children    []TLV // For constructed tags
}

// NewTLVParser creates a new EMV TLV parser
func NewTLVParser() *TLVParser {
	return &TLVParser{
		tags: getEMVTagDescriptions(),
	}
}

// Parse parses EMV TLV data
func (p *TLVParser) Parse(data []byte) ([]TLV, error) {
	tlvs := make([]TLV, 0)
	buf := bytes.NewBuffer(data)
	
	for buf.Len() > 0 {
		tlv, err := p.parseOne(buf)
		if err != nil {
			return nil, err
		}
		tlvs = append(tlvs, tlv)
	}
	
	return tlvs, nil
}

// parseOne parses a single TLV element
func (p *TLVParser) parseOne(buf *bytes.Buffer) (TLV, error) {
	// Parse tag
	tag, err := p.parseTag(buf)
	if err != nil {
		return TLV{}, err
	}
	
	// Parse length
	length, err := p.parseLength(buf)
	if err != nil {
		return TLV{}, err
	}
	
	// Parse value
	if buf.Len() < length {
		return TLV{}, fmt.Errorf("insufficient data for value: need %d, have %d", length, buf.Len())
	}
	
	value := make([]byte, length)
	if _, err := buf.Read(value); err != nil {
		return TLV{}, err
	}
	
	tlv := TLV{
		Tag:         tag,
		Length:      length,
		Value:       value,
		Description: p.getTagDescription(tag),
	}
	
	// Check if tag is constructed (parse children)
	if p.isConstructed(tag) {
		children, err := p.Parse(value)
		if err == nil {
			tlv.Children = children
		}
	}
	
	return tlv, nil
}

// parseTag parses a BER-TLV tag
func (p *TLVParser) parseTag(buf *bytes.Buffer) (string, error) {
	if buf.Len() == 0 {
		return "", fmt.Errorf("no data for tag")
	}
	
	firstByte, _ := buf.ReadByte()
	tag := []byte{firstByte}
	
	// Check if multi-byte tag (bits 1-5 of first byte are all 1)
	if firstByte&0x1F == 0x1F {
		// Read subsequent bytes
		for {
			if buf.Len() == 0 {
				return "", fmt.Errorf("incomplete multi-byte tag")
			}
			nextByte, _ := buf.ReadByte()
			tag = append(tag, nextByte)
			// Check bit 8 (continuation bit)
			if nextByte&0x80 == 0 {
				break
			}
		}
	}
	
	return hex.EncodeToString(tag), nil
}

// parseLength parses a BER-TLV length
func (p *TLVParser) parseLength(buf *bytes.Buffer) (int, error) {
	if buf.Len() == 0 {
		return 0, fmt.Errorf("no data for length")
	}
	
	firstByte, _ := buf.ReadByte()
	
	// Short form (bit 8 is 0)
	if firstByte&0x80 == 0 {
		return int(firstByte), nil
	}
	
	// Long form (bit 8 is 1)
	numBytes := int(firstByte & 0x7F)
	if numBytes == 0 {
		return 0, fmt.Errorf("indefinite length not supported")
	}
	
	if buf.Len() < numBytes {
		return 0, fmt.Errorf("insufficient data for length bytes")
	}
	
	lengthBytes := make([]byte, numBytes)
	if _, err := buf.Read(lengthBytes); err != nil {
		return 0, err
	}
	
	// Convert to integer
	length := 0
	for _, b := range lengthBytes {
		length = (length << 8) | int(b)
	}
	
	return length, nil
}

// isConstructed checks if a tag is constructed (bit 6 of first byte)
func (p *TLVParser) isConstructed(tag string) bool {
	if len(tag) < 2 {
		return false
	}
	firstByte, _ := hex.DecodeString(tag[0:2])
	return firstByte[0]&0x20 != 0
}

// getTagDescription returns the description for a tag
func (p *TLVParser) getTagDescription(tag string) string {
	if desc, exists := p.tags[tag]; exists {
		return desc
	}
	return "Unknown"
}

// GetValue retrieves a specific tag value from TLV list
func GetValue(tlvs []TLV, tag string) []byte {
	for _, tlv := range tlvs {
		if tlv.Tag == tag {
			return tlv.Value
		}
		if len(tlv.Children) > 0 {
			if value := GetValue(tlv.Children, tag); value != nil {
				return value
			}
		}
	}
	return nil
}

// ExtractEMVData extracts common EMV fields
func ExtractEMVData(tlvs []TLV) map[string]interface{} {
	data := make(map[string]interface{})
	
	// Application Identifier (AID)
	if aid := GetValue(tlvs, "4f"); aid != nil {
		data["aid"] = hex.EncodeToString(aid)
	}
	
	// Application Cryptogram (9F26)
	if cryptogram := GetValue(tlvs, "9f26"); cryptogram != nil {
		data["cryptogram"] = hex.EncodeToString(cryptogram)
	}
	
	// Terminal Verification Results (TVR - 95)
	if tvr := GetValue(tlvs, "95"); tvr != nil {
		data["tvr"] = hex.EncodeToString(tvr)
	}
	
	// Transaction Status Information (TSI - 9B)
	if tsi := GetValue(tlvs, "9b"); tsi != nil {
		data["tsi"] = hex.EncodeToString(tsi)
	}
	
	// Application Interchange Profile (AIP - 82)
	if aip := GetValue(tlvs, "82"); aip != nil {
		data["aip"] = hex.EncodeToString(aip)
	}
	
	// Application Transaction Counter (ATC - 9F36)
	if atc := GetValue(tlvs, "9f36"); atc != nil {
		data["atc"] = hex.EncodeToString(atc)
	}
	
	// Cryptogram Information Data (CID - 9F27)
	if cid := GetValue(tlvs, "9f27"); cid != nil {
		data["cid"] = hex.EncodeToString(cid)
	}
	
	// Unpredictable Number (9F37)
	if un := GetValue(tlvs, "9f37"); un != nil {
		data["unpredictable_number"] = hex.EncodeToString(un)
	}
	
	// Issuer Application Data (9F10)
	if iad := GetValue(tlvs, "9f10"); iad != nil {
		data["iad"] = hex.EncodeToString(iad)
	}
	
	// CVM Results (9F34)
	if cvmResults := GetValue(tlvs, "9f34"); cvmResults != nil {
		data["cvm_results"] = hex.EncodeToString(cvmResults)
	}
	
	return data
}

// getEMVTagDescriptions returns a map of EMV tag descriptions
func getEMVTagDescriptions() map[string]string {
	return map[string]string{
		"4f":   "Application Identifier (AID)",
		"50":   "Application Label",
		"57":   "Track 2 Equivalent Data",
		"5a":   "Application Primary Account Number (PAN)",
		"5f20": "Cardholder Name",
		"5f24": "Application Expiration Date",
		"5f25": "Application Effective Date",
		"5f28": "Issuer Country Code",
		"5f2a": "Transaction Currency Code",
		"5f2d": "Language Preference",
		"5f34": "Application Primary Account Number (PAN) Sequence Number",
		"82":   "Application Interchange Profile (AIP)",
		"84":   "Dedicated File (DF) Name",
		"87":   "Application Priority Indicator",
		"88":   "Short File Identifier (SFI)",
		"8a":   "Authorization Response Code",
		"8c":   "Card Risk Management Data Object List 1 (CDOL1)",
		"8d":   "Card Risk Management Data Object List 2 (CDOL2)",
		"8e":   "Cardholder Verification Method (CVM) List",
		"8f":   "Certification Authority Public Key Index",
		"90":   "Issuer Public Key Certificate",
		"91":   "Issuer Authentication Data",
		"92":   "Issuer Public Key Remainder",
		"93":   "Signed Static Application Data",
		"94":   "Application File Locator (AFL)",
		"95":   "Terminal Verification Results (TVR)",
		"97":   "Transaction Certificate Data Object List (TDOL)",
		"98":   "Transaction Certificate (TC) Hash Value",
		"99":   "Transaction Personal Identification Number (PIN) Data",
		"9a":   "Transaction Date",
		"9b":   "Transaction Status Information (TSI)",
		"9c":   "Transaction Type",
		"9d":   "Directory Definition File (DDF) Name",
		"9f01": "Acquirer Identifier",
		"9f02": "Amount, Authorized (Numeric)",
		"9f03": "Amount, Other (Numeric)",
		"9f04": "Amount, Other (Binary)",
		"9f05": "Application Discretionary Data",
		"9f06": "Application Identifier (AID) - Terminal",
		"9f07": "Application Usage Control",
		"9f08": "Application Version Number",
		"9f09": "Application Version Number - Terminal",
		"9f0d": "Issuer Action Code - Default",
		"9f0e": "Issuer Action Code - Denial",
		"9f0f": "Issuer Action Code - Online",
		"9f10": "Issuer Application Data",
		"9f11": "Issuer Code Table Index",
		"9f12": "Application Preferred Name",
		"9f13": "Last Online Application Transaction Counter (ATC) Register",
		"9f14": "Lower Consecutive Offline Limit",
		"9f15": "Merchant Category Code",
		"9f16": "Merchant Identifier",
		"9f17": "Personal Identification Number (PIN) Try Counter",
		"9f18": "Issuer Script Identifier",
		"9f1a": "Terminal Country Code",
		"9f1b": "Terminal Floor Limit",
		"9f1c": "Terminal Identification",
		"9f1d": "Terminal Risk Management Data",
		"9f1e": "Interface Device (IFD) Serial Number",
		"9f1f": "Track 1 Discretionary Data",
		"9f21": "Transaction Time",
		"9f22": "Certification Authority Public Key Index - Terminal",
		"9f23": "Upper Consecutive Offline Limit",
		"9f26": "Application Cryptogram",
		"9f27": "Cryptogram Information Data",
		"9f2d": "ICC PIN Encipherment Public Key Certificate",
		"9f2e": "ICC PIN Encipherment Public Key Exponent",
		"9f2f": "ICC PIN Encipherment Public Key Remainder",
		"9f32": "Issuer Public Key Exponent",
		"9f33": "Terminal Capabilities",
		"9f34": "Cardholder Verification Method (CVM) Results",
		"9f35": "Terminal Type",
		"9f36": "Application Transaction Counter (ATC)",
		"9f37": "Unpredictable Number",
		"9f38": "Processing Options Data Object List (PDOL)",
		"9f39": "Point-of-Service (POS) Entry Mode",
		"9f3a": "Amount, Reference Currency",
		"9f3b": "Application Reference Currency",
		"9f3c": "Transaction Reference Currency Code",
		"9f3d": "Transaction Reference Currency Exponent",
		"9f40": "Additional Terminal Capabilities",
		"9f41": "Transaction Sequence Counter",
		"9f42": "Application Currency Code",
		"9f43": "Application Reference Currency Exponent",
		"9f44": "Application Currency Exponent",
		"9f45": "Data Authentication Code",
		"9f46": "ICC Public Key Certificate",
		"9f47": "ICC Public Key Exponent",
		"9f48": "ICC Public Key Remainder",
		"9f49": "Dynamic Data Authentication Data Object List (DDOL)",
		"9f4a": "Static Data Authentication Tag List",
		"9f4b": "Signed Dynamic Application Data",
		"9f4c": "ICC Dynamic Number",
		"9f4d": "Log Entry",
		"9f4e": "Merchant Name and Location",
	}
}

// DecodeTVR decodes Terminal Verification Results
func DecodeTVR(tvr []byte) map[string]bool {
	if len(tvr) != 5 {
		return nil
	}
	
	return map[string]bool{
		"offline_data_auth_not_performed":     tvr[0]&0x80 != 0,
		"sda_failed":                          tvr[0]&0x40 != 0,
		"icc_data_missing":                    tvr[0]&0x20 != 0,
		"card_on_exception_file":              tvr[0]&0x10 != 0,
		"dda_failed":                          tvr[0]&0x08 != 0,
		"cda_failed":                          tvr[0]&0x04 != 0,
		"icc_and_terminal_versions_different": tvr[1]&0x80 != 0,
		"expired_application":                 tvr[1]&0x40 != 0,
		"application_not_yet_effective":       tvr[1]&0x20 != 0,
		"requested_service_not_allowed":       tvr[1]&0x10 != 0,
		"new_card":                            tvr[1]&0x08 != 0,
		"cardholder_verification_failed":      tvr[2]&0x80 != 0,
		"unrecognised_cvm":                    tvr[2]&0x40 != 0,
		"pin_try_limit_exceeded":              tvr[2]&0x20 != 0,
		"pin_entry_required_not_entered":      tvr[2]&0x10 != 0,
		"pin_entry_required_pad_not_present":  tvr[2]&0x08 != 0,
		"online_pin_entered":                  tvr[2]&0x02 != 0,
		"transaction_exceeds_floor_limit":     tvr[3]&0x80 != 0,
		"lower_consecutive_offline_exceeded":  tvr[3]&0x40 != 0,
		"upper_consecutive_offline_exceeded":  tvr[3]&0x20 != 0,
		"transaction_selected_randomly":       tvr[3]&0x10 != 0,
		"merchant_forced_online":              tvr[3]&0x08 != 0,
		"default_tdol_used":                   tvr[4]&0x80 != 0,
		"issuer_authentication_failed":        tvr[4]&0x40 != 0,
		"script_processing_failed":            tvr[4]&0x20 != 0,
	}
}

// DecodeTSI decodes Transaction Status Information
func DecodeTSI(tsi []byte) map[string]bool {
	if len(tsi) != 2 {
		return nil
	}
	
	return map[string]bool{
		"offline_data_auth_performed": tsi[0]&0x80 != 0,
		"cardholder_verification_performed": tsi[0]&0x40 != 0,
		"card_risk_management_performed": tsi[0]&0x20 != 0,
		"issuer_authentication_performed": tsi[0]&0x10 != 0,
		"terminal_risk_management_performed": tsi[0]&0x08 != 0,
		"script_processing_performed": tsi[0]&0x04 != 0,
	}
}
