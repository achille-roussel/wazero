package wasm

import (
	"encoding/binary"
	"fmt"
	"math"
)

// EncodeFieldValue converts an operand-stack uint64 to the Go-typed value
// stored in WasmStruct.Fields / WasmArray.Elements for the given field
// schema. Refs are stored as their uintptr bit pattern (the caller is
// responsible for keepalive). Packed i8/i16 fields are narrowed to uint8
// / uint16 before storage.
//
// Shared between the interpreter and wazevo so both engines agree on the
// in-memory representation.
func EncodeFieldValue(f FieldType, raw uint64) any {
	if f.Packed == PackedTypeI8 {
		return NarrowI8(int32(uint32(raw)))
	}
	if f.Packed == PackedTypeI16 {
		return NarrowI16(int32(uint32(raw)))
	}
	switch f.ValueType {
	case ValueTypeI32:
		return int32(uint32(raw))
	case ValueTypeI64:
		return int64(raw)
	case ValueTypeF32:
		return math.Float32frombits(uint32(raw))
	case ValueTypeF64:
		return math.Float64frombits(raw)
	}
	if IsRefFieldType(f.ValueType) {
		return uintptr(raw)
	}
	panic(fmt.Sprintf("unsupported struct/array field type %#x", f.ValueType))
}

// FieldReadKind selects between signed-extended and zero-extended reads
// for packed i8/i16 fields. For non-packed fields the kind is ignored.
type FieldReadKind uint8

const (
	// FieldReadDirect is the default kind: signed-narrow for packed
	// fields, value-preserving for non-packed.
	FieldReadDirect FieldReadKind = iota
	// FieldReadSignExtend is used by struct.get_s / array.get_s on
	// packed i8 / i16 fields to sign-extend the read.
	FieldReadSignExtend
	// FieldReadZeroExtend is used by struct.get_u / array.get_u on
	// packed i8 / i16 fields to zero-extend the read.
	FieldReadZeroExtend
)

// DecodeFieldValueRead converts a stored field value back to a uint64
// suitable for placing on the operand stack. The readKind selects
// sign-extended vs zero-extended decoding for packed i8/i16 fields.
func DecodeFieldValueRead(f FieldType, stored any, readKind FieldReadKind) uint64 {
	if f.Packed == PackedTypeI8 {
		v := stored.(uint8)
		if readKind == FieldReadSignExtend {
			return uint64(uint32(SignExtendI8(v)))
		}
		return uint64(ZeroExtendI8(v))
	}
	if f.Packed == PackedTypeI16 {
		v := stored.(uint16)
		if readKind == FieldReadSignExtend {
			return uint64(uint32(SignExtendI16(v)))
		}
		return uint64(ZeroExtendI16(v))
	}
	switch f.ValueType {
	case ValueTypeI32:
		return uint64(uint32(stored.(int32)))
	case ValueTypeI64:
		return uint64(stored.(int64))
	case ValueTypeF32:
		return uint64(math.Float32bits(stored.(float32)))
	case ValueTypeF64:
		return math.Float64bits(stored.(float64))
	}
	if IsRefFieldType(f.ValueType) {
		if stored == nil {
			return 0
		}
		return uint64(stored.(uintptr))
	}
	panic(fmt.Sprintf("unsupported struct/array field type %#x", f.ValueType))
}

// IsRefFieldType reports whether vt is a reference-typed shorthand byte
// (including the funcref sentinel used for concrete refs).
func IsRefFieldType(vt ValueType) bool {
	switch vt {
	case ValueTypeFuncref, ValueTypeExternref, ValueTypeExnref,
		ValueTypeAnyref, ValueTypeEqref, ValueTypeI31ref,
		ValueTypeStructref, ValueTypeArrayref,
		ValueTypeNullref, ValueTypeNoFuncref,
		ValueTypeNoExternref, ValueTypeNoExnref:
		return true
	}
	return false
}

// ArrayDataElemSize returns the byte size of one array element when read
// from a data segment, for the supported numeric / packed element types.
// Returns false for ref-typed elements (which use array.new_elem instead).
func ArrayDataElemSize(f FieldType) (uint32, bool) {
	if f.Packed == PackedTypeI8 {
		return 1, true
	}
	if f.Packed == PackedTypeI16 {
		return 2, true
	}
	switch f.ValueType {
	case ValueTypeI32, ValueTypeF32:
		return 4, true
	case ValueTypeI64, ValueTypeF64:
		return 8, true
	case ValueTypeV128:
		return 16, true
	}
	return 0, false
}

// ReadDataElement decodes a single array element from data starting at
// off, returning the Go-typed value the WasmArray.Elements slice stores.
func ReadDataElement(f FieldType, data []byte, off uint32) any {
	if f.Packed == PackedTypeI8 {
		return uint8(data[off])
	}
	if f.Packed == PackedTypeI16 {
		return binary.LittleEndian.Uint16(data[off:])
	}
	switch f.ValueType {
	case ValueTypeI32:
		return int32(binary.LittleEndian.Uint32(data[off:]))
	case ValueTypeI64:
		return int64(binary.LittleEndian.Uint64(data[off:]))
	case ValueTypeF32:
		return math.Float32frombits(binary.LittleEndian.Uint32(data[off:]))
	case ValueTypeF64:
		return math.Float64frombits(binary.LittleEndian.Uint64(data[off:]))
	}
	panic(fmt.Sprintf("unsupported element type for array.new_data: %#x", f.ValueType))
}
