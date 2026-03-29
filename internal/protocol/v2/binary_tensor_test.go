/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v2

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- DatatypeSize --------------------------------------------------------

func TestDatatypeSize(t *testing.T) {
	cases := []struct {
		dt   string
		want int
	}{
		{DatatypeBOOL, 1},
		{DatatypeUINT8, 1},
		{DatatypeINT8, 1},
		{DatatypeUINT16, 2},
		{DatatypeINT16, 2},
		{DatatypeFP16, 2},
		{DatatypeBF16, 2},
		{DatatypeUINT32, 4},
		{DatatypeINT32, 4},
		{DatatypeFP32, 4},
		{DatatypeUINT64, 8},
		{DatatypeINT64, 8},
		{DatatypeFP64, 8},
		{DatatypeBYTES, 0},
		{"UNKNOWN", 0},
	}
	for _, tc := range cases {
		t.Run(tc.dt, func(t *testing.T) {
			assert.Equal(t, tc.want, DatatypeSize(tc.dt))
		})
	}
}

// ---- NewBinaryEncoder ----------------------------------------------------

func TestNewBinaryEncoder_NotNil(t *testing.T) {
	assert.NotNil(t, NewBinaryEncoder())
}

// ---- EncodeTensor — wrong type errors ------------------------------------

func TestEncodeTensor_UnsupportedDatatype(t *testing.T) {
	e := NewBinaryEncoder()
	_, err := e.EncodeTensor("FLOAT128", []float64{1.0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported datatype")
}

func TestEncodeTensor_WrongGoType(t *testing.T) {
	e := NewBinaryEncoder()
	// BOOL expects []bool, not []int
	_, err := e.EncodeTensor(DatatypeBOOL, []int{1, 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected []bool")
}

// ---- DecodeTensor — unsupported type -------------------------------------

func TestDecodeTensor_UnsupportedDatatype(t *testing.T) {
	e := NewBinaryEncoder()
	_, err := e.DecodeTensor("FLOAT128", []byte{0, 0}, []int64{1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported datatype")
}

// ---- BOOL round-trip -----------------------------------------------------

func TestBOOL_RoundTrip(t *testing.T) {
	e := NewBinaryEncoder()
	input := []bool{true, false, true, true, false}
	encoded, err := e.EncodeTensor(DatatypeBOOL, input)
	require.NoError(t, err)
	assert.Equal(t, 5, len(encoded))

	decoded, err := e.DecodeTensor(DatatypeBOOL, encoded, []int64{5})
	require.NoError(t, err)
	assert.Equal(t, input, decoded)
}

func TestBOOL_Decode_TruncatedBuffer(t *testing.T) {
	e := NewBinaryEncoder()
	_, err := e.DecodeTensor(DatatypeBOOL, []byte{1}, []int64{5}) // need 5, have 1
	require.Error(t, err)
}

// ---- UINT8 round-trip ----------------------------------------------------

func TestUINT8_RoundTrip(t *testing.T) {
	e := NewBinaryEncoder()
	input := []uint8{0, 127, 255}
	encoded, err := e.EncodeTensor(DatatypeUINT8, input)
	require.NoError(t, err)
	assert.Equal(t, []byte{0, 127, 255}, encoded)

	decoded, err := e.DecodeTensor(DatatypeUINT8, encoded, []int64{3})
	require.NoError(t, err)
	assert.Equal(t, input, decoded)
}

// ---- UINT16 round-trip ---------------------------------------------------

func TestUINT16_RoundTrip(t *testing.T) {
	e := NewBinaryEncoder()
	input := []uint16{0, 1000, 65535}
	encoded, err := e.EncodeTensor(DatatypeUINT16, input)
	require.NoError(t, err)
	assert.Equal(t, 6, len(encoded))

	decoded, err := e.DecodeTensor(DatatypeUINT16, encoded, []int64{3})
	require.NoError(t, err)
	assert.Equal(t, input, decoded)
}

// ---- UINT32 round-trip ---------------------------------------------------

func TestUINT32_RoundTrip(t *testing.T) {
	e := NewBinaryEncoder()
	input := []uint32{0, 1<<16 + 1, math.MaxUint32}
	encoded, err := e.EncodeTensor(DatatypeUINT32, input)
	require.NoError(t, err)
	assert.Equal(t, 12, len(encoded))

	decoded, err := e.DecodeTensor(DatatypeUINT32, encoded, []int64{3})
	require.NoError(t, err)
	assert.Equal(t, input, decoded)
}

// ---- UINT64 round-trip ---------------------------------------------------

func TestUINT64_RoundTrip(t *testing.T) {
	e := NewBinaryEncoder()
	input := []uint64{0, math.MaxUint32 + 1, math.MaxUint64}
	encoded, err := e.EncodeTensor(DatatypeUINT64, input)
	require.NoError(t, err)
	assert.Equal(t, 24, len(encoded))

	decoded, err := e.DecodeTensor(DatatypeUINT64, encoded, []int64{3})
	require.NoError(t, err)
	assert.Equal(t, input, decoded)
}

// ---- INT8 round-trip -----------------------------------------------------

func TestINT8_RoundTrip(t *testing.T) {
	e := NewBinaryEncoder()
	input := []int8{-128, 0, 127}
	encoded, err := e.EncodeTensor(DatatypeINT8, input)
	require.NoError(t, err)
	assert.Equal(t, 3, len(encoded))

	decoded, err := e.DecodeTensor(DatatypeINT8, encoded, []int64{3})
	require.NoError(t, err)
	assert.Equal(t, input, decoded)
}

// ---- INT16 round-trip ----------------------------------------------------

func TestINT16_RoundTrip(t *testing.T) {
	e := NewBinaryEncoder()
	input := []int16{-32768, 0, 32767}
	encoded, err := e.EncodeTensor(DatatypeINT16, input)
	require.NoError(t, err)
	assert.Equal(t, 6, len(encoded))

	decoded, err := e.DecodeTensor(DatatypeINT16, encoded, []int64{3})
	require.NoError(t, err)
	assert.Equal(t, input, decoded)
}

// ---- INT32 round-trip ----------------------------------------------------

func TestINT32_RoundTrip(t *testing.T) {
	e := NewBinaryEncoder()
	input := []int32{math.MinInt32, 0, math.MaxInt32}
	encoded, err := e.EncodeTensor(DatatypeINT32, input)
	require.NoError(t, err)
	assert.Equal(t, 12, len(encoded))

	decoded, err := e.DecodeTensor(DatatypeINT32, encoded, []int64{3})
	require.NoError(t, err)
	assert.Equal(t, input, decoded)
}

// ---- INT64 round-trip ----------------------------------------------------

func TestINT64_RoundTrip(t *testing.T) {
	e := NewBinaryEncoder()
	input := []int64{math.MinInt64, 0, math.MaxInt64}
	encoded, err := e.EncodeTensor(DatatypeINT64, input)
	require.NoError(t, err)
	assert.Equal(t, 24, len(encoded))

	decoded, err := e.DecodeTensor(DatatypeINT64, encoded, []int64{3})
	require.NoError(t, err)
	assert.Equal(t, input, decoded)
}

// ---- FP16 round-trip (raw uint16 bits) -----------------------------------

func TestFP16_RoundTrip(t *testing.T) {
	e := NewBinaryEncoder()
	// Store half-float bits as uint16 (e.g., 1.0 in FP16 = 0x3C00)
	input := []uint16{0x3C00, 0x0000, 0x7BFF}
	encoded, err := e.EncodeTensor(DatatypeFP16, input)
	require.NoError(t, err)
	assert.Equal(t, 6, len(encoded))

	decoded, err := e.DecodeTensor(DatatypeFP16, encoded, []int64{3})
	require.NoError(t, err)
	assert.Equal(t, input, decoded)
}

// ---- FP32 round-trip -----------------------------------------------------

func TestFP32_RoundTrip(t *testing.T) {
	e := NewBinaryEncoder()
	input := []float32{-1.5, 0.0, 3.14159}
	encoded, err := e.EncodeTensor(DatatypeFP32, input)
	require.NoError(t, err)
	assert.Equal(t, 12, len(encoded))

	decoded, err := e.DecodeTensor(DatatypeFP32, encoded, []int64{3})
	require.NoError(t, err)
	result := decoded.([]float32)
	assert.InDelta(t, input[0], result[0], 1e-6)
	assert.InDelta(t, input[1], result[1], 1e-6)
	assert.InDelta(t, input[2], result[2], 1e-4)
}

func TestFP32_NaNInf(t *testing.T) {
	e := NewBinaryEncoder()
	input := []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))}
	encoded, err := e.EncodeTensor(DatatypeFP32, input)
	require.NoError(t, err)

	decoded, err := e.DecodeTensor(DatatypeFP32, encoded, []int64{3})
	require.NoError(t, err)
	result := decoded.([]float32)
	assert.True(t, math.IsNaN(float64(result[0])))
	assert.True(t, math.IsInf(float64(result[1]), 1))
	assert.True(t, math.IsInf(float64(result[2]), -1))
}

// ---- FP64 round-trip -----------------------------------------------------

func TestFP64_RoundTrip(t *testing.T) {
	e := NewBinaryEncoder()
	input := []float64{-1.5e308, 0.0, 1.5e308}
	encoded, err := e.EncodeTensor(DatatypeFP64, input)
	require.NoError(t, err)
	assert.Equal(t, 24, len(encoded))

	decoded, err := e.DecodeTensor(DatatypeFP64, encoded, []int64{3})
	require.NoError(t, err)
	assert.InDeltaSlice(t, input, decoded, 1e300)
}

// ---- BYTES round-trip ----------------------------------------------------

func TestBYTES_RoundTrip(t *testing.T) {
	e := NewBinaryEncoder()
	input := [][]byte{
		[]byte("hello"),
		[]byte("world"),
		{}, // empty byte slice
		{0x00, 0xFF},
	}
	encoded, err := e.EncodeTensor(DatatypeBYTES, input)
	require.NoError(t, err)
	// Each element: 4-byte length prefix + data
	// 4+5 + 4+5 + 4+0 + 4+2 = 9+9+4+6 = 28
	assert.Equal(t, 28, len(encoded))

	decoded, err := e.DecodeTensor(DatatypeBYTES, encoded, []int64{4})
	require.NoError(t, err)
	assert.Equal(t, input, decoded)
}

func TestBYTES_EmptySlice(t *testing.T) {
	e := NewBinaryEncoder()
	input := [][]byte{}
	encoded, err := e.EncodeTensor(DatatypeBYTES, input)
	require.NoError(t, err)
	assert.Empty(t, encoded)
}

func TestBYTES_Decode_TruncatedLengthPrefix(t *testing.T) {
	e := NewBinaryEncoder()
	// Only 3 bytes — not enough for a 4-byte length prefix
	_, err := e.DecodeTensor(DatatypeBYTES, []byte{0, 0, 0}, []int64{1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected end of BYTES data")
}

func TestBYTES_Decode_TruncatedPayload(t *testing.T) {
	e := NewBinaryEncoder()
	// Length prefix says 10 bytes, but only 2 bytes follow
	buf := []byte{10, 0, 0, 0, 'a', 'b'}
	_, err := e.DecodeTensor(DatatypeBYTES, buf, []int64{1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected end of BYTES data")
}

// ---- TensorBinarySize ---------------------------------------------------

func TestTensorBinarySize(t *testing.T) {
	e := NewBinaryEncoder()
	size, err := e.TensorBinarySize(DatatypeINT32, []int32{1, 2, 3})
	require.NoError(t, err)
	assert.Equal(t, int64(12), size)
}

func TestTensorBinarySize_Error(t *testing.T) {
	e := NewBinaryEncoder()
	_, err := e.TensorBinarySize("BAD", nil)
	require.Error(t, err)
}

// ---- Decode truncated buffers for integer types --------------------------

func TestUINT16_Decode_TruncatedBuffer(t *testing.T) {
	e := NewBinaryEncoder()
	_, err := e.DecodeTensor(DatatypeUINT16, []byte{0x01}, []int64{2}) // need 4, have 1
	require.Error(t, err)
}

func TestINT32_Decode_TruncatedBuffer(t *testing.T) {
	e := NewBinaryEncoder()
	_, err := e.DecodeTensor(DatatypeINT32, []byte{0, 0}, []int64{2}) // need 8, have 2
	require.Error(t, err)
}

func TestFP64_Decode_TruncatedBuffer(t *testing.T) {
	e := NewBinaryEncoder()
	_, err := e.DecodeTensor(DatatypeFP64, []byte{0, 0, 0, 0}, []int64{2}) // need 16, have 4
	require.Error(t, err)
}

// ---- Wrong Go types for encoding ----------------------------------------

func TestEncodeUINT8_WrongType(t *testing.T) {
	e := NewBinaryEncoder()
	_, err := e.EncodeTensor(DatatypeUINT8, []int{1, 2})
	require.Error(t, err)
}

func TestEncodeINT32_WrongType(t *testing.T) {
	e := NewBinaryEncoder()
	_, err := e.EncodeTensor(DatatypeINT32, []int{1, 2})
	require.Error(t, err)
}

func TestEncodeFP32_WrongType(t *testing.T) {
	e := NewBinaryEncoder()
	_, err := e.EncodeTensor(DatatypeFP32, []float64{1.0})
	require.Error(t, err)
}

func TestEncodeBYTES_WrongType(t *testing.T) {
	e := NewBinaryEncoder()
	_, err := e.EncodeTensor(DatatypeBYTES, []string{"hello"})
	require.Error(t, err)
}

// ---- Multi-dim shape (volume > 1 dim) ------------------------------------

func TestINT32_MultiDimShape(t *testing.T) {
	e := NewBinaryEncoder()
	// 2x3 matrix = 6 elements
	input := []int32{1, 2, 3, 4, 5, 6}
	encoded, err := e.EncodeTensor(DatatypeINT32, input)
	require.NoError(t, err)
	assert.Equal(t, 24, len(encoded))

	decoded, err := e.DecodeTensor(DatatypeINT32, encoded, []int64{2, 3})
	require.NoError(t, err)
	assert.Equal(t, input, decoded)
}
