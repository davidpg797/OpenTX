package packager

import (
	"github.com/krish567366/OpenTX/pkg/iso8583"
)

// MastercardPackager implements Mastercard-specific ISO 8583 packaging
type MastercardPackager struct {
	*GenericPackager
}

// NewMastercardPackager creates a new Mastercard packager
func NewMastercardPackager() *MastercardPackager {
	specs := buildMastercardFieldSpecs()
	generic := NewGenericPackager("MASTERCARD", specs)
	
	// Mastercard uses ASCII for MTI, binary for bitmap
	generic.SetMTIEncoding(iso8583.EncodingASCII, 4)
	generic.SetBitmapEncoding(iso8583.EncodingBinary)
	
	return &MastercardPackager{
		GenericPackager: generic,
	}
}

func buildMastercardFieldSpecs() map[int]*iso8583.FieldSpec {
	// Mastercard has similar but slightly different field specs
	specs := buildVisaFieldSpecs() // Start with Visa as base
	
	// Override/add Mastercard-specific fields
	specs[15] = &iso8583.FieldSpec{
		ID:         15,
		Name:       "Settlement Date",
		Type:       iso8583.FieldTypeDate,
		LengthType: iso8583.LengthTypeFixed,
		MaxLength:  4,
		Encoding:   iso8583.EncodingASCII,
	}
	
	specs[48] = &iso8583.FieldSpec{
		ID:         48,
		Name:       "Additional Data",
		Type:       iso8583.FieldTypeAlphanumeric,
		LengthType: iso8583.LengthTypeLLLVAR,
		MaxLength:  999,
		Encoding:   iso8583.EncodingASCII,
	}
	
	return specs
}

// NPCIPackager implements NPCI/RuPay-specific ISO 8583 packaging
type NPCIPackager struct {
	*GenericPackager
}

// NewNPCIPackager creates a new NPCI/RuPay packager
func NewNPCIPackager() *NPCIPackager {
	specs := buildNPCIFieldSpecs()
	generic := NewGenericPackager("NPCI", specs)
	
	// NPCI uses ASCII for both MTI and bitmap
	generic.SetMTIEncoding(iso8583.EncodingASCII, 4)
	generic.SetBitmapEncoding(iso8583.EncodingASCII)
	
	// NPCI has specific header requirements
	header := []byte{0x60, 0x00} // Example header
	generic.SetHeader(2, header)
	
	return &NPCIPackager{
		GenericPackager: generic,
	}
}

func buildNPCIFieldSpecs() map[int]*iso8583.FieldSpec {
	// NPCI uses ISO 8583:1993 with custom extensions
	specs := buildVisaFieldSpecs()
	
	// Add NPCI-specific fields
	specs[34] = &iso8583.FieldSpec{
		ID:         34,
		Name:       "Extended Primary Account Number",
		Type:       iso8583.FieldTypeAlphanumeric,
		LengthType: iso8583.LengthTypeLLVAR,
		MaxLength:  28,
		Encoding:   iso8583.EncodingASCII,
	}
	
	specs[44] = &iso8583.FieldSpec{
		ID:         44,
		Name:       "Additional Response Data",
		Type:       iso8583.FieldTypeAlphanumeric,
		LengthType: iso8583.LengthTypeLLVAR,
		MaxLength:  25,
		Encoding:   iso8583.EncodingASCII,
	}
	
	return specs
}
