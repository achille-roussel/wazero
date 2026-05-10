package binaryencoding

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// EncodeFunctionType encodes one type-section sub-type entry in the
// WebAssembly binary format. Despite the legacy name, this handles all
// composite forms (func, struct, array) and the sub / sub-final wrappers
// when SuperTypeIndex is set.
//
// Rec-group wrapping (0x4E) is handled by encodeTypeSection, which sees the
// rec-group context across consecutive entries.
//
// See https://webassembly.github.io/spec/core/binary/types.html
func EncodeFunctionType(t *wasm.FunctionType) []byte {
	if t.SuperTypeIndex != nil {
		var prefix byte
		if t.Final {
			prefix = 0x4F // sub final
		} else {
			prefix = 0x50 // sub
		}
		buf := []byte{prefix, 0x01}
		buf = append(buf, leb128.EncodeUint32(*t.SuperTypeIndex)...)
		buf = append(buf, encodeCompositeBody(t)...)
		return buf
	}
	return encodeCompositeBody(t)
}

// encodeCompositeBody emits the composite-type body, starting with the form
// byte (0x60 / 0x5F / 0x5E) and followed by the form-specific data.
func encodeCompositeBody(t *wasm.FunctionType) []byte {
	switch t.Form {
	case wasm.CompositeFormFunc:
		data := append([]byte{0x60}, EncodeValTypes(t.Params)...)
		return append(data, EncodeValTypes(t.Results)...)
	case wasm.CompositeFormStruct:
		return encodeStructBody(t.Fields)
	case wasm.CompositeFormArray:
		return encodeArrayBody(t.ArrayField)
	}
	panic(fmt.Sprintf("unknown composite form: %d", t.Form))
}

func encodeStructBody(fields []wasm.FieldType) []byte {
	buf := []byte{0x5F}
	buf = append(buf, leb128.EncodeUint32(uint32(len(fields)))...)
	for _, f := range fields {
		buf = append(buf, encodeFieldType(f)...)
	}
	return buf
}

func encodeArrayBody(f wasm.FieldType) []byte {
	return append([]byte{0x5E}, encodeFieldType(f)...)
}

func encodeFieldType(f wasm.FieldType) []byte {
	var buf []byte
	switch f.Packed {
	case wasm.PackedTypeI8:
		buf = append(buf, wasm.PackedTypeI8Byte)
	case wasm.PackedTypeI16:
		buf = append(buf, wasm.PackedTypeI16Byte)
	case wasm.PackedTypeNone:
		// Value-type byte. RefInfo (for rich refs) is not yet plumbed
		// through encoding; Phase 4+ will extend this.
		buf = append(buf, f.ValueType)
	}
	if f.Mutable {
		buf = append(buf, 0x01)
	} else {
		buf = append(buf, 0x00)
	}
	return buf
}
