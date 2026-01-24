package packager

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/krish567366/OpenTX/pkg/iso8583"
)

// GenericPackager implements a configurable ISO 8583 packager
type GenericPackager struct {
	variant      string
	specs        map[int]*iso8583.FieldSpec
	mtiEncoding  iso8583.EncodingType
	mtiLength    int
	headerLength int
	headerValue  []byte
	bitmapEnc    iso8583.EncodingType
}

// NewGenericPackager creates a new generic packager with configuration
func NewGenericPackager(variant string, specs map[int]*iso8583.FieldSpec) *GenericPackager {
	return &GenericPackager{
		variant:      variant,
		specs:        specs,
		mtiEncoding:  iso8583.EncodingASCII,
		mtiLength:    4,
		bitmapEnc:    iso8583.EncodingASCII,
		headerLength: 0,
	}
}

// Pack converts an ISO8583 message to raw bytes
func (p *GenericPackager) Pack(msg *iso8583.Message) ([]byte, error) {
	result := make([]byte, 0, 1024)
	
	// Add header if configured
	if p.headerLength > 0 && len(p.headerValue) > 0 {
		result = append(result, p.headerValue...)
	}
	
	// Pack MTI
	mti, err := p.packMTI(msg.MTI)
	if err != nil {
		return nil, fmt.Errorf("failed to pack MTI: %w", err)
	}
	result = append(result, mti...)
	
	// Pack bitmap
	bitmap, err := p.packBitmap(msg.Bitmap)
	if err != nil {
		return nil, fmt.Errorf("failed to pack bitmap: %w", err)
	}
	result = append(result, bitmap...)
	
	// Pack fields in order
	fields := msg.Bitmap.GetFields()
	for _, fieldNum := range fields {
		if fieldNum == 1 { // Skip bitmap itself
			continue
		}
		
		field, exists := msg.GetField(fieldNum)
		if !exists {
			continue
		}
		
		spec, err := p.GetFieldSpec(fieldNum)
		if err != nil {
			return nil, fmt.Errorf("no spec for field %d: %w", fieldNum, err)
		}
		
		fieldData, err := p.packField(field, spec)
		if err != nil {
			return nil, fmt.Errorf("failed to pack field %d: %w", fieldNum, err)
		}
		result = append(result, fieldData...)
	}
	
	return result, nil
}

// Unpack converts raw bytes to an ISO8583 message
func (p *GenericPackager) Unpack(data []byte) (*iso8583.Message, error) {
	if len(data) < p.mtiLength+16 { // MTI + minimum bitmap
		return nil, fmt.Errorf("insufficient data length")
	}
	
	msg := iso8583.NewMessage("", p.variant)
	msg.RawMessage = data
	offset := 0
	
	// Skip header if present
	if p.headerLength > 0 {
		if len(data) < p.headerLength {
			return nil, fmt.Errorf("insufficient data for header")
		}
		offset += p.headerLength
		msg.HeaderLength = p.headerLength
	}
	
	// Unpack MTI
	mti, err := p.unpackMTI(data[offset : offset+p.mtiLength])
	if err != nil {
		return nil, fmt.Errorf("failed to unpack MTI: %w", err)
	}
	msg.MTI = mti
	offset += p.mtiLength
	
	// Unpack bitmap
	bitmap, bitmapLen, err := iso8583.ParseBitmap(data[offset:], p.bitmapEnc)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack bitmap: %w", err)
	}
	msg.Bitmap = bitmap
	offset += bitmapLen
	
	// Unpack fields in order
	fields := bitmap.GetFields()
	for _, fieldNum := range fields {
		if fieldNum == 1 { // Skip bitmap itself
			continue
		}
		
		spec, err := p.GetFieldSpec(fieldNum)
		if err != nil {
			return nil, fmt.Errorf("no spec for field %d: %w", fieldNum, err)
		}
		
		field, bytesRead, err := p.unpackField(data[offset:], fieldNum, spec)
		if err != nil {
			return nil, fmt.Errorf("failed to unpack field %d at offset %d: %w", fieldNum, offset, err)
		}
		
		msg.Fields[fieldNum] = field
		offset += bytesRead
	}
	
	return msg, nil
}

// GetVariant returns the variant name
func (p *GenericPackager) GetVariant() string {
	return p.variant
}

// GetFieldSpec returns the field specification
func (p *GenericPackager) GetFieldSpec(fieldNum int) (*iso8583.FieldSpec, error) {
	spec, exists := p.specs[fieldNum]
	if !exists {
		return nil, fmt.Errorf("no specification for field %d", fieldNum)
	}
	return spec, nil
}

// packMTI packs the MTI
func (p *GenericPackager) packMTI(mti string) ([]byte, error) {
	if len(mti) != 4 {
		return nil, fmt.Errorf("invalid MTI length: %d", len(mti))
	}
	
	if p.mtiEncoding == iso8583.EncodingASCII {
		return []byte(mti), nil
	}
	
	// BCD encoding
	result := make([]byte, 2)
	for i := 0; i < 4; i += 2 {
		high := mti[i] - '0'
		low := mti[i+1] - '0'
		result[i/2] = (high << 4) | low
	}
	return result, nil
}

// unpackMTI unpacks the MTI
func (p *GenericPackager) unpackMTI(data []byte) (string, error) {
	if p.mtiEncoding == iso8583.EncodingASCII {
		return string(data), nil
	}
	
	// BCD decoding
	if len(data) != 2 {
		return "", fmt.Errorf("invalid BCD MTI length: %d", len(data))
	}
	
	result := make([]byte, 4)
	for i := 0; i < 2; i++ {
		high := (data[i] >> 4) & 0x0F
		low := data[i] & 0x0F
		result[i*2] = '0' + high
		result[i*2+1] = '0' + low
	}
	return string(result), nil
}

// packBitmap packs the bitmap
func (p *GenericPackager) packBitmap(bitmap *iso8583.Bitmap) ([]byte, error) {
	if p.bitmapEnc == iso8583.EncodingASCII {
		return []byte(bitmap.ToHex()), nil
	}
	
	// Binary encoding
	result := make([]byte, 0, 16)
	result = append(result, bitmap.Primary...)
	if bitmap.HasSecondary() {
		result = append(result, bitmap.Secondary...)
	}
	return result, nil
}

// packField packs a single field
func (p *GenericPackager) packField(field iso8583.Field, spec *iso8583.FieldSpec) ([]byte, error) {
	var value []byte
	
	// Convert value to bytes
	switch v := field.Value.(type) {
	case string:
		value = []byte(v)
	case []byte:
		value = v
	case int, int64:
		value = []byte(fmt.Sprintf("%d", v))
	default:
		return nil, fmt.Errorf("unsupported value type for field %d", field.ID)
	}
	
	// Apply padding if needed
	value = p.applyPadding(value, spec)
	
	result := make([]byte, 0)
	
	// Add length indicator for variable length fields
	switch spec.LengthType {
	case iso8583.LengthTypeLLVAR:
		lengthStr := fmt.Sprintf("%02d", len(value))
		result = append(result, []byte(lengthStr)...)
	case iso8583.LengthTypeLLLVAR:
		lengthStr := fmt.Sprintf("%03d", len(value))
		result = append(result, []byte(lengthStr)...)
	case iso8583.LengthTypeLLLLVAR:
		lengthStr := fmt.Sprintf("%04d", len(value))
		result = append(result, []byte(lengthStr)...)
	}
	
	// Encode value
	encoded, err := p.encodeValue(value, spec.Encoding)
	if err != nil {
		return nil, err
	}
	
	result = append(result, encoded...)
	return result, nil
}

// unpackField unpacks a single field
func (p *GenericPackager) unpackField(data []byte, fieldNum int, spec *iso8583.FieldSpec) (iso8583.Field, int, error) {
	field := iso8583.Field{
		ID:   fieldNum,
		Name: spec.Name,
		Type: spec.Type,
		Spec: spec,
	}
	
	offset := 0
	length := spec.MaxLength
	
	// Read length for variable length fields
	switch spec.LengthType {
	case iso8583.LengthTypeLLVAR:
		if len(data) < 2 {
			return field, 0, fmt.Errorf("insufficient data for LL length")
		}
		lengthStr := string(data[0:2])
		var err error
		length, err = strconv.Atoi(lengthStr)
		if err != nil {
			return field, 0, fmt.Errorf("invalid LL length: %s", lengthStr)
		}
		offset = 2
	case iso8583.LengthTypeLLLVAR:
		if len(data) < 3 {
			return field, 0, fmt.Errorf("insufficient data for LLL length")
		}
		lengthStr := string(data[0:3])
		var err error
		length, err = strconv.Atoi(lengthStr)
		if err != nil {
			return field, 0, fmt.Errorf("invalid LLL length: %s", lengthStr)
		}
		offset = 3
	case iso8583.LengthTypeLLLLVAR:
		if len(data) < 4 {
			return field, 0, fmt.Errorf("insufficient data for LLLL length")
		}
		lengthStr := string(data[0:4])
		var err error
		length, err = strconv.Atoi(lengthStr)
		if err != nil {
			return field, 0, fmt.Errorf("invalid LLLL length: %s", lengthStr)
		}
		offset = 4
	}
	
	// Check if enough data is available
	if len(data) < offset+length {
		return field, 0, fmt.Errorf("insufficient data for field value: need %d, have %d", offset+length, len(data))
	}
	
	// Decode value
	value, err := p.decodeValue(data[offset:offset+length], spec.Encoding, spec.Type)
	if err != nil {
		return field, 0, err
	}
	
	field.Value = value
	field.RawValue = data[offset : offset+length]
	field.Length = length
	
	return field, offset + length, nil
}

// applyPadding applies padding to a value
func (p *GenericPackager) applyPadding(value []byte, spec *iso8583.FieldSpec) []byte {
	if spec.LengthType != iso8583.LengthTypeFixed {
		return value
	}
	
	if len(value) >= spec.MaxLength {
		return value[:spec.MaxLength]
	}
	
	padLen := spec.MaxLength - len(value)
	switch spec.Padding {
	case iso8583.PaddingLeftZero:
		return append([]byte(strings.Repeat("0", padLen)), value...)
	case iso8583.PaddingRightSpace:
		return append(value, []byte(strings.Repeat(" ", padLen))...)
	}
	
	return value
}

// encodeValue encodes a value according to encoding type
func (p *GenericPackager) encodeValue(value []byte, encoding iso8583.EncodingType) ([]byte, error) {
	switch encoding {
	case iso8583.EncodingASCII:
		return value, nil
	case iso8583.EncodingBinary:
		return value, nil
	case iso8583.EncodingBCD:
		// Convert ASCII digits to BCD
		result := make([]byte, (len(value)+1)/2)
		for i := 0; i < len(value); i += 2 {
			high := value[i] - '0'
			low := byte(0)
			if i+1 < len(value) {
				low = value[i+1] - '0'
			}
			result[i/2] = (high << 4) | low
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported encoding: %v", encoding)
	}
}

// decodeValue decodes a value according to encoding type
func (p *GenericPackager) decodeValue(data []byte, encoding iso8583.EncodingType, fieldType iso8583.FieldType) (interface{}, error) {
	switch encoding {
	case iso8583.EncodingASCII:
		if fieldType == iso8583.FieldTypeBinary {
			return data, nil
		}
		return string(data), nil
	case iso8583.EncodingBinary:
		return data, nil
	case iso8583.EncodingBCD:
		// Convert BCD to ASCII digits
		result := make([]byte, len(data)*2)
		for i, b := range data {
			high := (b >> 4) & 0x0F
			low := b & 0x0F
			result[i*2] = '0' + high
			result[i*2+1] = '0' + low
		}
		return string(result), nil
	default:
		return nil, fmt.Errorf("unsupported encoding: %v", encoding)
	}
}

// SetHeader configures message header
func (p *GenericPackager) SetHeader(length int, value []byte) {
	p.headerLength = length
	p.headerValue = value
}

// SetMTIEncoding configures MTI encoding
func (p *GenericPackager) SetMTIEncoding(encoding iso8583.EncodingType, length int) {
	p.mtiEncoding = encoding
	p.mtiLength = length
}

// SetBitmapEncoding configures bitmap encoding
func (p *GenericPackager) SetBitmapEncoding(encoding iso8583.EncodingType) {
	p.bitmapEnc = encoding
}
