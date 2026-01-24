package iso8583

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBitmap(t *testing.T) {
	t.Run("Set and IsSet", func(t *testing.T) {
		bitmap := NewBitmap()
		
		// Set some fields
		bitmap.Set(2)
		bitmap.Set(3)
		bitmap.Set(4)
		bitmap.Set(11)
		bitmap.Set(37)
		bitmap.Set(39)
		
		// Check if fields are set
		assert.True(t, bitmap.IsSet(2))
		assert.True(t, bitmap.IsSet(3))
		assert.True(t, bitmap.IsSet(4))
		assert.True(t, bitmap.IsSet(11))
		assert.True(t, bitmap.IsSet(37))
		assert.True(t, bitmap.IsSet(39))
		
		// Check fields that are not set
		assert.False(t, bitmap.IsSet(1))
		assert.False(t, bitmap.IsSet(5))
		assert.False(t, bitmap.IsSet(100))
	})
	
	t.Run("Secondary Bitmap", func(t *testing.T) {
		bitmap := NewBitmap()
		
		// Set field in secondary bitmap
		bitmap.Set(65)
		bitmap.Set(90)
		bitmap.Set(100)
		
		// Check secondary bitmap is enabled
		assert.True(t, bitmap.HasSecondary())
		assert.True(t, bitmap.IsSet(1)) // Bit 1 should be set for secondary bitmap
		
		// Check fields in secondary bitmap
		assert.True(t, bitmap.IsSet(65))
		assert.True(t, bitmap.IsSet(90))
		assert.True(t, bitmap.IsSet(100))
	})
	
	t.Run("ToHex", func(t *testing.T) {
		bitmap := NewBitmap()
		bitmap.Set(2)
		bitmap.Set(3)
		bitmap.Set(4)
		
		hexStr := bitmap.ToHex()
		assert.NotEmpty(t, hexStr)
		assert.Equal(t, 16, len(hexStr)) // 8 bytes = 16 hex chars
	})
}

func TestParseBitmap(t *testing.T) {
	t.Run("Parse ASCII Bitmap", func(t *testing.T) {
		// Bitmap with fields 2, 3, 4, 11, 37, 39
		hexBitmap := "7000000000000000" // Example bitmap
		data := []byte(hexBitmap)
		
		bitmap, bytesRead, err := ParseBitmap(data, EncodingASCII)
		require.NoError(t, err)
		assert.Equal(t, 16, bytesRead)
		assert.NotNil(t, bitmap)
	})
	
	t.Run("Parse Binary Bitmap", func(t *testing.T) {
		// Binary bitmap
		binaryBitmap, _ := hex.DecodeString("7000000000000000")
		
		bitmap, bytesRead, err := ParseBitmap(binaryBitmap, EncodingBinary)
		require.NoError(t, err)
		assert.Equal(t, 8, bytesRead)
		assert.NotNil(t, bitmap)
	})
}

func TestMessage(t *testing.T) {
	t.Run("Create Message", func(t *testing.T) {
		msg := NewMessage("0200", "VISA")
		
		assert.Equal(t, "0200", msg.MTI)
		assert.Equal(t, "VISA", msg.Variant)
		assert.NotNil(t, msg.Bitmap)
		assert.NotNil(t, msg.Fields)
	})
	
	t.Run("Set and Get Field", func(t *testing.T) {
		msg := NewMessage("0200", "VISA")
		
		err := msg.SetField(2, "1234567890123456")
		require.NoError(t, err)
		
		field, exists := msg.GetField(2)
		assert.True(t, exists)
		assert.Equal(t, "1234567890123456", field.Value)
	})
	
	t.Run("GetFieldAsString", func(t *testing.T) {
		msg := NewMessage("0200", "VISA")
		msg.SetField(11, "123456")
		
		value := msg.GetFieldAsString(11)
		assert.Equal(t, "123456", value)
	})
	
	t.Run("HasField", func(t *testing.T) {
		msg := NewMessage("0200", "VISA")
		msg.SetField(2, "1234567890123456")
		
		assert.True(t, msg.HasField(2))
		assert.False(t, msg.HasField(3))
	})
}

func TestFieldSpec(t *testing.T) {
	t.Run("Create Field Spec", func(t *testing.T) {
		spec := &FieldSpec{
			ID:         2,
			Name:       "Primary Account Number",
			Type:       FieldTypeNumeric,
			LengthType: LengthTypeLLVAR,
			MaxLength:  19,
			Encoding:   EncodingASCII,
		}
		
		assert.Equal(t, 2, spec.ID)
		assert.Equal(t, "Primary Account Number", spec.Name)
		assert.Equal(t, FieldTypeNumeric, spec.Type)
		assert.Equal(t, LengthTypeLLVAR, spec.LengthType)
		assert.Equal(t, 19, spec.MaxLength)
		assert.Equal(t, EncodingASCII, spec.Encoding)
	})
}

// Integration test
func TestMessagePackUnpack(t *testing.T) {
	t.Run("Pack and Unpack Message", func(t *testing.T) {
		// This test would require a packager implementation
		// Skipping for now as it needs the full packager setup
		t.Skip("Requires full packager implementation")
	})
}

// Benchmark tests
func BenchmarkBitmapSet(b *testing.B) {
	bitmap := NewBitmap()
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		bitmap.Set(2 + (i % 62))
	}
}

func BenchmarkBitmapIsSet(b *testing.B) {
	bitmap := NewBitmap()
	for i := 2; i <= 64; i++ {
		bitmap.Set(i)
	}
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		bitmap.IsSet(2 + (i % 62))
	}
}
