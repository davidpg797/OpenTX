package iso8583

import (
	"encoding/hex"
	"fmt"
)

// Packager interface for ISO 8583 message packing/unpacking
type Packager interface {
	// Pack converts an ISO8583Message to raw bytes
	Pack(msg *Message) ([]byte, error)
	
	// Unpack converts raw bytes to an ISO8583Message
	Unpack(data []byte) (*Message, error)
	
	// GetVariant returns the variant name (VISA, MC, NPCI, etc.)
	GetVariant() string
	
	// GetFieldSpec returns the specification for a field
	GetFieldSpec(fieldNum int) (*FieldSpec, error)
}

// Message represents a parsed ISO 8583 message
type Message struct {
	MTI           string
	Bitmap        *Bitmap
	Fields        map[int]Field
	Variant       string
	RawMessage    []byte
	HeaderLength  int
	TrailerLength int
}

// Field represents an ISO 8583 data element
type Field struct {
	ID       int
	Name     string
	Type     FieldType
	Value    interface{}
	RawValue []byte
	Length   int
	Spec     *FieldSpec
}

// Bitmap represents ISO 8583 bitmap
type Bitmap struct {
	Primary   []byte
	Secondary []byte
	Fields    map[int]bool
}

// FieldSpec defines the specification for an ISO 8583 field
type FieldSpec struct {
	ID          int
	Name        string
	Type        FieldType
	LengthType  LengthType
	MaxLength   int
	Encoding    EncodingType
	Padding     PaddingType
	Mandatory   bool
	Description string
	Subfields   []SubfieldSpec
}

// SubfieldSpec defines a subfield within a composite field
type SubfieldSpec struct {
	ID         int
	Name       string
	Type       FieldType
	LengthType LengthType
	MaxLength  int
	Encoding   EncodingType
}

// FieldType represents the type of data in a field
type FieldType int

const (
	FieldTypeNumeric FieldType = iota
	FieldTypeAlpha
	FieldTypeAlphanumeric
	FieldTypeBinary
	FieldTypeTrack
	FieldTypeAmount
	FieldTypeDate
	FieldTypeTime
	FieldTypeTLV
)

// LengthType represents how field length is encoded
type LengthType int

const (
	LengthTypeFixed LengthType = iota
	LengthTypeLLVAR
	LengthTypeLLLVAR
	LengthTypeLLLLVAR
	LengthTypeBitmap
)

// EncodingType represents field encoding
type EncodingType int

const (
	EncodingASCII EncodingType = iota
	EncodingBCD
	EncodingBinary
	EncodingEBCDIC
)

// PaddingType represents padding direction
type PaddingType int

const (
	PaddingNone PaddingType = iota
	PaddingLeftZero
	PaddingRightSpace
)

// NewMessage creates a new ISO 8583 message
func NewMessage(mti string, variant string) *Message {
	return &Message{
		MTI:     mti,
		Bitmap:  NewBitmap(),
		Fields:  make(map[int]Field),
		Variant: variant,
	}
}

// SetField sets a field value in the message
func (m *Message) SetField(fieldNum int, value interface{}) error {
	field := Field{
		ID:    fieldNum,
		Value: value,
	}
	m.Fields[fieldNum] = field
	m.Bitmap.Set(fieldNum)
	return nil
}

// GetField retrieves a field value from the message
func (m *Message) GetField(fieldNum int) (Field, bool) {
	field, exists := m.Fields[fieldNum]
	return field, exists
}

// GetFieldAsString retrieves a field value as string
func (m *Message) GetFieldAsString(fieldNum int) string {
	if field, exists := m.Fields[fieldNum]; exists {
		if str, ok := field.Value.(string); ok {
			return str
		}
	}
	return ""
}

// GetFieldAsBytes retrieves a field value as bytes
func (m *Message) GetFieldAsBytes(fieldNum int) []byte {
	if field, exists := m.Fields[fieldNum]; exists {
		if bytes, ok := field.Value.([]byte); ok {
			return bytes
		}
	}
	return nil
}

// HasField checks if a field is present
func (m *Message) HasField(fieldNum int) bool {
	return m.Bitmap.IsSet(fieldNum)
}

// NewBitmap creates a new bitmap
func NewBitmap() *Bitmap {
	return &Bitmap{
		Primary:   make([]byte, 8),
		Secondary: make([]byte, 8),
		Fields:    make(map[int]bool),
	}
}

// Set marks a field as present in the bitmap
func (b *Bitmap) Set(fieldNum int) {
	if fieldNum < 1 || fieldNum > 192 {
		return
	}
	
	b.Fields[fieldNum] = true
	
	if fieldNum <= 64 {
		byteIdx := (fieldNum - 1) / 8
		bitIdx := 7 - ((fieldNum - 1) % 8)
		b.Primary[byteIdx] |= 1 << bitIdx
	} else {
		// Set bit 1 in primary bitmap to indicate secondary bitmap presence
		b.Primary[0] |= 0x80
		byteIdx := (fieldNum - 65) / 8
		bitIdx := 7 - ((fieldNum - 65) % 8)
		b.Secondary[byteIdx] |= 1 << bitIdx
	}
}

// IsSet checks if a field is present in the bitmap
func (b *Bitmap) IsSet(fieldNum int) bool {
	return b.Fields[fieldNum]
}

// GetFields returns all set field numbers
func (b *Bitmap) GetFields() []int {
	fields := make([]int, 0, len(b.Fields))
	for fieldNum := range b.Fields {
		fields = append(fields, fieldNum)
	}
	return fields
}

// ToHex returns hex representation of bitmap
func (b *Bitmap) ToHex() string {
	if b.HasSecondary() {
		return hex.EncodeToString(b.Primary) + hex.EncodeToString(b.Secondary)
	}
	return hex.EncodeToString(b.Primary)
}

// HasSecondary checks if secondary bitmap is present
func (b *Bitmap) HasSecondary() bool {
	return (b.Primary[0] & 0x80) != 0
}

// ParseBitmap parses bitmap from bytes
func ParseBitmap(data []byte, encoding EncodingType) (*Bitmap, int, error) {
	bitmap := NewBitmap()
	bytesRead := 0
	
	var primary []byte
	if encoding == EncodingASCII {
		if len(data) < 16 {
			return nil, 0, fmt.Errorf("insufficient data for ASCII bitmap")
		}
		primary = make([]byte, 8)
		hex.Decode(primary, data[0:16])
		bytesRead = 16
	} else {
		if len(data) < 8 {
			return nil, 0, fmt.Errorf("insufficient data for binary bitmap")
		}
		primary = data[0:8]
		bytesRead = 8
	}
	
	copy(bitmap.Primary, primary)
	
	// Parse primary bitmap
	for i := 0; i < 64; i++ {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		if (primary[byteIdx] & (1 << bitIdx)) != 0 {
			bitmap.Fields[i+1] = true
		}
	}
	
	// Check for secondary bitmap
	if (primary[0] & 0x80) != 0 {
		var secondary []byte
		if encoding == EncodingASCII {
			if len(data) < 32 {
				return nil, 0, fmt.Errorf("insufficient data for secondary bitmap")
			}
			secondary = make([]byte, 8)
			hex.Decode(secondary, data[16:32])
			bytesRead = 32
		} else {
			if len(data) < 16 {
				return nil, 0, fmt.Errorf("insufficient data for secondary bitmap")
			}
			secondary = data[8:16]
			bytesRead = 16
		}
		
		copy(bitmap.Secondary, secondary)
		
		// Parse secondary bitmap
		for i := 0; i < 64; i++ {
			byteIdx := i / 8
			bitIdx := 7 - (i % 8)
			if (secondary[byteIdx] & (1 << bitIdx)) != 0 {
				bitmap.Fields[i+65] = true
			}
		}
	}
	
	return bitmap, bytesRead, nil
}

// String returns a string representation of the message
func (m *Message) String() string {
	return fmt.Sprintf("MTI: %s, Variant: %s, Fields: %v", m.MTI, m.Variant, m.Bitmap.GetFields())
}
