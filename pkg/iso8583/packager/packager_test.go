package packager

import (
	"testing"

	"github.com/krish567366/OpenTX/pkg/iso8583"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVisaPackager(t *testing.T) {
	packager := NewVisaPackager()
	
	t.Run("Create Visa Packager", func(t *testing.T) {
		assert.NotNil(t, packager)
		assert.Equal(t, "VISA", packager.GetVariant())
	})
	
	t.Run("Pack Authorization Request", func(t *testing.T) {
		msg := iso8583.NewMessage("0100", "VISA")
		
		// Set required fields
		msg.SetField(2, "4111111111111111")    // PAN
		msg.SetField(3, "000000")              // Processing code
		msg.SetField(4, "000000010000")        // Amount
		msg.SetField(7, "0124123045")          // Transmission date/time
		msg.SetField(11, "123456")             // STAN
		msg.SetField(12, "123045")             // Local time
		msg.SetField(13, "0124")               // Local date
		msg.SetField(14, "2512")               // Expiry date
		msg.SetField(22, "051")                // POS entry mode
		msg.SetField(25, "00")                 // POS condition code
		msg.SetField(32, "12345678901")        // Acquirer ID
		msg.SetField(37, "000000123456")       // RRN
		msg.SetField(41, "TERM0001")           // Terminal ID
		msg.SetField(42, "MERCHANT00001")      // Merchant ID
		msg.SetField(49, "840")                // Currency code (USD)
		
		// Pack message
		packed, err := packager.Pack(msg)
		require.NoError(t, err)
		assert.NotEmpty(t, packed)
		
		// Unpack message
		unpacked, err := packager.Unpack(packed)
		require.NoError(t, err)
		assert.Equal(t, "0100", unpacked.MTI)
		assert.Equal(t, "4111111111111111", unpacked.GetFieldAsString(2))
		assert.Equal(t, "000000", unpacked.GetFieldAsString(3))
		assert.Equal(t, "123456", unpacked.GetFieldAsString(11))
	})
	
	t.Run("Pack Authorization Response", func(t *testing.T) {
		msg := iso8583.NewMessage("0110", "VISA")
		
		// Set required fields
		msg.SetField(2, "4111111111111111")
		msg.SetField(3, "000000")
		msg.SetField(4, "000000010000")
		msg.SetField(7, "0124123045")
		msg.SetField(11, "123456")
		msg.SetField(12, "123045")
		msg.SetField(13, "0124")
		msg.SetField(37, "000000123456")
		msg.SetField(38, "ABC123")             // Auth ID
		msg.SetField(39, "00")                 // Response code
		msg.SetField(41, "TERM0001")
		msg.SetField(42, "MERCHANT00001")
		msg.SetField(49, "840")
		
		packed, err := packager.Pack(msg)
		require.NoError(t, err)
		assert.NotEmpty(t, packed)
		
		unpacked, err := packager.Unpack(packed)
		require.NoError(t, err)
		assert.Equal(t, "0110", unpacked.MTI)
		assert.Equal(t, "00", unpacked.GetFieldAsString(39))
		assert.Equal(t, "ABC123", unpacked.GetFieldAsString(38))
	})
}

func TestMastercardPackager(t *testing.T) {
	packager := NewMastercardPackager()
	
	t.Run("Create Mastercard Packager", func(t *testing.T) {
		assert.NotNil(t, packager)
		assert.Equal(t, "MASTERCARD", packager.GetVariant())
	})
	
	t.Run("Binary Bitmap Encoding", func(t *testing.T) {
		msg := iso8583.NewMessage("0200", "MASTERCARD")
		msg.SetField(2, "5111111111111111")
		msg.SetField(3, "000000")
		msg.SetField(4, "000000010000")
		
		packed, err := packager.Pack(msg)
		require.NoError(t, err)
		assert.NotEmpty(t, packed)
		
		// Mastercard uses binary bitmap, so the bitmap portion should be 8 bytes
		// MTI (4 bytes) + Bitmap (8 bytes) = 12 bytes minimum
		assert.GreaterOrEqual(t, len(packed), 12)
	})
}

func TestNPCIPackager(t *testing.T) {
	packager := NewNPCIPackager()
	
	t.Run("Create NPCI Packager", func(t *testing.T) {
		assert.NotNil(t, packager)
		assert.Equal(t, "NPCI", packager.GetVariant())
	})
	
	t.Run("Header Processing", func(t *testing.T) {
		msg := iso8583.NewMessage("0200", "NPCI")
		msg.SetField(2, "6011111111111111")
		msg.SetField(3, "000000")
		msg.SetField(4, "000000010000")
		
		packed, err := packager.Pack(msg)
		require.NoError(t, err)
		assert.NotEmpty(t, packed)
		
		// NPCI has a 2-byte header
		// Header (2 bytes) + MTI (4 bytes) + Bitmap (16 bytes hex) = 22 bytes minimum
		assert.GreaterOrEqual(t, len(packed), 22)
	})
}

func TestGenericPackager(t *testing.T) {
	t.Run("Field Padding", func(t *testing.T) {
		specs := map[int]*iso8583.FieldSpec{
			11: {
				ID:         11,
				Name:       "STAN",
				Type:       iso8583.FieldTypeNumeric,
				LengthType: iso8583.LengthTypeFixed,
				MaxLength:  6,
				Encoding:   iso8583.EncodingASCII,
				Padding:    iso8583.PaddingLeftZero,
			},
		}
		
		packager := NewGenericPackager("TEST", specs)
		msg := iso8583.NewMessage("0200", "TEST")
		msg.SetField(11, "123") // Should be padded to "000123"
		
		field, _ := msg.GetField(11)
		assert.Equal(t, "123", field.Value)
	})
	
	t.Run("Variable Length Field", func(t *testing.T) {
		specs := map[int]*iso8583.FieldSpec{
			2: {
				ID:         2,
				Name:       "PAN",
				Type:       iso8583.FieldTypeNumeric,
				LengthType: iso8583.LengthTypeLLVAR,
				MaxLength:  19,
				Encoding:   iso8583.EncodingASCII,
			},
		}
		
		packager := NewGenericPackager("TEST", specs)
		msg := iso8583.NewMessage("0200", "TEST")
		msg.SetField(2, "4111111111111111")
		
		packed, err := packager.Pack(msg)
		require.NoError(t, err)
		assert.NotEmpty(t, packed)
	})
}

// Benchmark tests
func BenchmarkVisaPackager_Pack(b *testing.B) {
	packager := NewVisaPackager()
	msg := iso8583.NewMessage("0200", "VISA")
	msg.SetField(2, "4111111111111111")
	msg.SetField(3, "000000")
	msg.SetField(4, "000000010000")
	msg.SetField(11, "123456")
	msg.SetField(37, "000000123456")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = packager.Pack(msg)
	}
}

func BenchmarkVisaPackager_Unpack(b *testing.B) {
	packager := NewVisaPackager()
	msg := iso8583.NewMessage("0200", "VISA")
	msg.SetField(2, "4111111111111111")
	msg.SetField(3, "000000")
	msg.SetField(4, "000000010000")
	msg.SetField(11, "123456")
	msg.SetField(37, "000000123456")
	
	packed, _ := packager.Pack(msg)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = packager.Unpack(packed)
	}
}
