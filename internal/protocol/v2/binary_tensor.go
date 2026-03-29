/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package v2

import (
	"encoding/binary"
	"fmt"
	"math"
)

// BinaryEncoder encodes tensor data to binary wire format per the
// Binary Tensor Data Extension specification.
//
// Wire format: little-endian, row-major, no stride or padding.
// - BOOL: 1 byte (1=true, 0=false)
// - BYTES: 4-byte uint32 length prefix + raw bytes
// - FP16/BF16: 2 bytes native representation
// - All others: native little-endian size
type BinaryEncoder struct{}

// NewBinaryEncoder creates a new BinaryEncoder.
func NewBinaryEncoder() *BinaryEncoder {
	return &BinaryEncoder{}
}

// EncodeTensor encodes tensor data to binary format.
// data must be a flat slice of the appropriate Go type for the datatype.
func (e *BinaryEncoder) EncodeTensor(datatype string, data any) ([]byte, error) {
	switch datatype {
	case DatatypeBOOL:
		return e.encodeBool(data)
	case DatatypeUINT8:
		return e.encodeUint8(data)
	case DatatypeUINT16:
		return e.encodeUint16(data)
	case DatatypeUINT32:
		return e.encodeUint32(data)
	case DatatypeUINT64:
		return e.encodeUint64(data)
	case DatatypeINT8:
		return e.encodeInt8(data)
	case DatatypeINT16:
		return e.encodeInt16(data)
	case DatatypeINT32:
		return e.encodeInt32(data)
	case DatatypeINT64:
		return e.encodeInt64(data)
	case DatatypeFP16:
		return e.encodeFP16(data)
	case DatatypeFP32:
		return e.encodeFP32(data)
	case DatatypeFP64:
		return e.encodeFP64(data)
	case DatatypeBYTES:
		return e.encodeBytes(data)
	default:
		return nil, fmt.Errorf("unsupported datatype: %s", datatype)
	}
}

// DecodeTensor decodes binary data back to a Go slice.
func (e *BinaryEncoder) DecodeTensor(datatype string, raw []byte, shape []int64) (any, error) {
	numElements := int64(1)
	for _, dim := range shape {
		numElements *= dim
	}

	switch datatype {
	case DatatypeBOOL:
		return e.decodeBool(raw, numElements)
	case DatatypeUINT8:
		return e.decodeUint8(raw, numElements)
	case DatatypeUINT16:
		return e.decodeUint16(raw, numElements)
	case DatatypeUINT32:
		return e.decodeUint32(raw, numElements)
	case DatatypeUINT64:
		return e.decodeUint64(raw, numElements)
	case DatatypeINT8:
		return e.decodeInt8(raw, numElements)
	case DatatypeINT16:
		return e.decodeInt16(raw, numElements)
	case DatatypeINT32:
		return e.decodeInt32(raw, numElements)
	case DatatypeINT64:
		return e.decodeInt64(raw, numElements)
	case DatatypeFP16:
		return e.decodeFP16(raw, numElements)
	case DatatypeFP32:
		return e.decodeFP32(raw, numElements)
	case DatatypeFP64:
		return e.decodeFP64(raw, numElements)
	case DatatypeBYTES:
		return e.decodeBytes(raw, numElements)
	default:
		return nil, fmt.Errorf("unsupported datatype: %s", datatype)
	}
}

// TensorBinarySize computes the byte size of a tensor in binary format.
func (e *BinaryEncoder) TensorBinarySize(datatype string, data any) (int64, error) {
	encoded, err := e.EncodeTensor(datatype, data)
	if err != nil {
		return 0, err
	}
	return int64(len(encoded)), nil
}

// ----- Encode helpers -----

func (e *BinaryEncoder) encodeBool(data any) ([]byte, error) {
	vals, ok := data.([]bool)
	if !ok {
		return nil, fmt.Errorf("expected []bool, got %T", data)
	}
	buf := make([]byte, len(vals))
	for i, v := range vals {
		if v {
			buf[i] = 1
		}
	}
	return buf, nil
}

func (e *BinaryEncoder) encodeUint8(data any) ([]byte, error) {
	vals, ok := data.([]uint8)
	if !ok {
		return nil, fmt.Errorf("expected []uint8, got %T", data)
	}
	return vals, nil
}

func (e *BinaryEncoder) encodeUint16(data any) ([]byte, error) {
	vals, ok := data.([]uint16)
	if !ok {
		return nil, fmt.Errorf("expected []uint16, got %T", data)
	}
	buf := make([]byte, len(vals)*2)
	for i, v := range vals {
		binary.LittleEndian.PutUint16(buf[i*2:], v)
	}
	return buf, nil
}

func (e *BinaryEncoder) encodeUint32(data any) ([]byte, error) {
	vals, ok := data.([]uint32)
	if !ok {
		return nil, fmt.Errorf("expected []uint32, got %T", data)
	}
	buf := make([]byte, len(vals)*4)
	for i, v := range vals {
		binary.LittleEndian.PutUint32(buf[i*4:], v)
	}
	return buf, nil
}

func (e *BinaryEncoder) encodeUint64(data any) ([]byte, error) {
	vals, ok := data.([]uint64)
	if !ok {
		return nil, fmt.Errorf("expected []uint64, got %T", data)
	}
	buf := make([]byte, len(vals)*8)
	for i, v := range vals {
		binary.LittleEndian.PutUint64(buf[i*8:], v)
	}
	return buf, nil
}

func (e *BinaryEncoder) encodeInt8(data any) ([]byte, error) {
	vals, ok := data.([]int8)
	if !ok {
		return nil, fmt.Errorf("expected []int8, got %T", data)
	}
	buf := make([]byte, len(vals))
	for i, v := range vals {
		buf[i] = byte(v)
	}
	return buf, nil
}

func (e *BinaryEncoder) encodeInt16(data any) ([]byte, error) {
	vals, ok := data.([]int16)
	if !ok {
		return nil, fmt.Errorf("expected []int16, got %T", data)
	}
	buf := make([]byte, len(vals)*2)
	for i, v := range vals {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(v))
	}
	return buf, nil
}

func (e *BinaryEncoder) encodeInt32(data any) ([]byte, error) {
	vals, ok := data.([]int32)
	if !ok {
		return nil, fmt.Errorf("expected []int32, got %T", data)
	}
	buf := make([]byte, len(vals)*4)
	for i, v := range vals {
		binary.LittleEndian.PutUint32(buf[i*4:], uint32(v))
	}
	return buf, nil
}

func (e *BinaryEncoder) encodeInt64(data any) ([]byte, error) {
	vals, ok := data.([]int64)
	if !ok {
		return nil, fmt.Errorf("expected []int64, got %T", data)
	}
	buf := make([]byte, len(vals)*8)
	for i, v := range vals {
		binary.LittleEndian.PutUint64(buf[i*8:], uint64(v))
	}
	return buf, nil
}

func (e *BinaryEncoder) encodeFP16(data any) ([]byte, error) {
	// FP16: accept []uint16 raw half-float bits
	vals, ok := data.([]uint16)
	if !ok {
		return nil, fmt.Errorf("expected []uint16 (FP16 bits), got %T", data)
	}
	buf := make([]byte, len(vals)*2)
	for i, v := range vals {
		binary.LittleEndian.PutUint16(buf[i*2:], v)
	}
	return buf, nil
}

func (e *BinaryEncoder) encodeFP32(data any) ([]byte, error) {
	vals, ok := data.([]float32)
	if !ok {
		return nil, fmt.Errorf("expected []float32, got %T", data)
	}
	buf := make([]byte, len(vals)*4)
	for i, v := range vals {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf, nil
}

func (e *BinaryEncoder) encodeFP64(data any) ([]byte, error) {
	vals, ok := data.([]float64)
	if !ok {
		return nil, fmt.Errorf("expected []float64, got %T", data)
	}
	buf := make([]byte, len(vals)*8)
	for i, v := range vals {
		binary.LittleEndian.PutUint64(buf[i*8:], math.Float64bits(v))
	}
	return buf, nil
}

func (e *BinaryEncoder) encodeBytes(data any) ([]byte, error) {
	vals, ok := data.([][]byte)
	if !ok {
		return nil, fmt.Errorf("expected [][]byte, got %T", data)
	}
	var buf []byte
	for _, v := range vals {
		// 4-byte uint32 length prefix + raw bytes
		lenBuf := make([]byte, 4)
		binary.LittleEndian.PutUint32(lenBuf, uint32(len(v)))
		buf = append(buf, lenBuf...)
		buf = append(buf, v...)
	}
	return buf, nil
}

// ----- Decode helpers -----

func (e *BinaryEncoder) decodeBool(raw []byte, n int64) ([]bool, error) {
	if int64(len(raw)) < n {
		return nil, fmt.Errorf("expected %d bytes for BOOL, got %d", n, len(raw))
	}
	vals := make([]bool, n)
	for i := int64(0); i < n; i++ {
		vals[i] = raw[i] != 0
	}
	return vals, nil
}

func (e *BinaryEncoder) decodeUint8(raw []byte, n int64) ([]uint8, error) {
	if int64(len(raw)) < n {
		return nil, fmt.Errorf("expected %d bytes for UINT8, got %d", n, len(raw))
	}
	return raw[:n], nil
}

func (e *BinaryEncoder) decodeUint16(raw []byte, n int64) ([]uint16, error) {
	expected := n * 2
	if int64(len(raw)) < expected {
		return nil, fmt.Errorf("expected %d bytes for UINT16, got %d", expected, len(raw))
	}
	vals := make([]uint16, n)
	for i := int64(0); i < n; i++ {
		vals[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	return vals, nil
}

func (e *BinaryEncoder) decodeUint32(raw []byte, n int64) ([]uint32, error) {
	expected := n * 4
	if int64(len(raw)) < expected {
		return nil, fmt.Errorf("expected %d bytes for UINT32, got %d", expected, len(raw))
	}
	vals := make([]uint32, n)
	for i := int64(0); i < n; i++ {
		vals[i] = binary.LittleEndian.Uint32(raw[i*4:])
	}
	return vals, nil
}

func (e *BinaryEncoder) decodeUint64(raw []byte, n int64) ([]uint64, error) {
	expected := n * 8
	if int64(len(raw)) < expected {
		return nil, fmt.Errorf("expected %d bytes for UINT64, got %d", expected, len(raw))
	}
	vals := make([]uint64, n)
	for i := int64(0); i < n; i++ {
		vals[i] = binary.LittleEndian.Uint64(raw[i*8:])
	}
	return vals, nil
}

func (e *BinaryEncoder) decodeInt8(raw []byte, n int64) ([]int8, error) {
	if int64(len(raw)) < n {
		return nil, fmt.Errorf("expected %d bytes for INT8, got %d", n, len(raw))
	}
	vals := make([]int8, n)
	for i := int64(0); i < n; i++ {
		vals[i] = int8(raw[i])
	}
	return vals, nil
}

func (e *BinaryEncoder) decodeInt16(raw []byte, n int64) ([]int16, error) {
	expected := n * 2
	if int64(len(raw)) < expected {
		return nil, fmt.Errorf("expected %d bytes for INT16, got %d", expected, len(raw))
	}
	vals := make([]int16, n)
	for i := int64(0); i < n; i++ {
		vals[i] = int16(binary.LittleEndian.Uint16(raw[i*2:]))
	}
	return vals, nil
}

func (e *BinaryEncoder) decodeInt32(raw []byte, n int64) ([]int32, error) {
	expected := n * 4
	if int64(len(raw)) < expected {
		return nil, fmt.Errorf("expected %d bytes for INT32, got %d", expected, len(raw))
	}
	vals := make([]int32, n)
	for i := int64(0); i < n; i++ {
		vals[i] = int32(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return vals, nil
}

func (e *BinaryEncoder) decodeInt64(raw []byte, n int64) ([]int64, error) {
	expected := n * 8
	if int64(len(raw)) < expected {
		return nil, fmt.Errorf("expected %d bytes for INT64, got %d", expected, len(raw))
	}
	vals := make([]int64, n)
	for i := int64(0); i < n; i++ {
		vals[i] = int64(binary.LittleEndian.Uint64(raw[i*8:]))
	}
	return vals, nil
}

func (e *BinaryEncoder) decodeFP16(raw []byte, n int64) ([]uint16, error) {
	expected := n * 2
	if int64(len(raw)) < expected {
		return nil, fmt.Errorf("expected %d bytes for FP16, got %d", expected, len(raw))
	}
	vals := make([]uint16, n)
	for i := int64(0); i < n; i++ {
		vals[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	return vals, nil
}

func (e *BinaryEncoder) decodeFP32(raw []byte, n int64) ([]float32, error) {
	expected := n * 4
	if int64(len(raw)) < expected {
		return nil, fmt.Errorf("expected %d bytes for FP32, got %d", expected, len(raw))
	}
	vals := make([]float32, n)
	for i := int64(0); i < n; i++ {
		vals[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return vals, nil
}

func (e *BinaryEncoder) decodeFP64(raw []byte, n int64) ([]float64, error) {
	expected := n * 8
	if int64(len(raw)) < expected {
		return nil, fmt.Errorf("expected %d bytes for FP64, got %d", expected, len(raw))
	}
	vals := make([]float64, n)
	for i := int64(0); i < n; i++ {
		vals[i] = math.Float64frombits(binary.LittleEndian.Uint64(raw[i*8:]))
	}
	return vals, nil
}

func (e *BinaryEncoder) decodeBytes(raw []byte, n int64) ([][]byte, error) {
	vals := make([][]byte, 0, n)
	offset := 0
	for i := int64(0); i < n; i++ {
		if offset+4 > len(raw) {
			return nil, fmt.Errorf("unexpected end of BYTES data at element %d", i)
		}
		length := int(binary.LittleEndian.Uint32(raw[offset:]))
		offset += 4
		if offset+length > len(raw) {
			return nil, fmt.Errorf("unexpected end of BYTES data at element %d, need %d bytes", i, length)
		}
		val := make([]byte, length)
		copy(val, raw[offset:offset+length])
		vals = append(vals, val)
		offset += length
	}
	return vals, nil
}
