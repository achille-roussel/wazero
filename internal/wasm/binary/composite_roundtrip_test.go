package binary

import (
	"bytes"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// TestEncodeDecodeRoundTrip_Composite verifies that the test-only encoder
// in internal/testing/binaryencoding and the production decoder in
// internal/wasm/binary agree on the wire format for all composite forms
// (func, struct, array), the sub / sub-final wrappers, and rec groups.
func TestEncodeDecodeRoundTrip_Composite(t *testing.T) {
	supIdx := uint32(0)

	tests := []struct {
		name string
		in   []wasm.FunctionType
	}{
		{
			name: "func shorthand",
			in: []wasm.FunctionType{
				{Form: wasm.CompositeFormFunc, Params: []wasm.ValueType{wasm.ValueTypeI32}, Results: []wasm.ValueType{wasm.ValueTypeI64}},
			},
		},
		{
			name: "struct shorthand const+mut",
			in: []wasm.FunctionType{
				{
					Form: wasm.CompositeFormStruct,
					Fields: []wasm.FieldType{
						{ValueType: wasm.ValueTypeI32},
						{ValueType: wasm.ValueTypeI64, Mutable: true},
					},
				},
			},
		},
		{
			name: "struct shorthand packed",
			in: []wasm.FunctionType{
				{
					Form: wasm.CompositeFormStruct,
					Fields: []wasm.FieldType{
						{Packed: wasm.PackedTypeI8, Mutable: true},
						{Packed: wasm.PackedTypeI16},
					},
				},
			},
		},
		{
			name: "struct empty",
			in: []wasm.FunctionType{
				{Form: wasm.CompositeFormStruct},
			},
		},
		{
			name: "array shorthand",
			in: []wasm.FunctionType{
				{Form: wasm.CompositeFormArray, ArrayField: wasm.FieldType{ValueType: wasm.ValueTypeI32, Mutable: true}},
			},
		},
		{
			name: "array packed",
			in: []wasm.FunctionType{
				{Form: wasm.CompositeFormArray, ArrayField: wasm.FieldType{Packed: wasm.PackedTypeI8}},
			},
		},
		{
			name: "sub final with supertype",
			in: []wasm.FunctionType{
				{Form: wasm.CompositeFormFunc, Params: []wasm.ValueType{wasm.ValueTypeI32}},
				{
					Form:           wasm.CompositeFormFunc,
					Params:         []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32},
					SuperTypeIndex: &supIdx,
					Final:          true,
				},
			},
		},
		{
			name: "sub (non-final) struct with supertype",
			in: []wasm.FunctionType{
				{Form: wasm.CompositeFormStruct, Fields: []wasm.FieldType{{ValueType: wasm.ValueTypeI32}}},
				{
					Form:           wasm.CompositeFormStruct,
					Fields:         []wasm.FieldType{{ValueType: wasm.ValueTypeI32}, {ValueType: wasm.ValueTypeI64}},
					SuperTypeIndex: &supIdx,
					Final:          false,
				},
			},
		},
		{
			name: "rec group of two structs",
			in: []wasm.FunctionType{
				{
					Form:             wasm.CompositeFormStruct,
					Fields:           []wasm.FieldType{{ValueType: wasm.ValueTypeI32}},
					RecGroupSize:     2,
					RecGroupPosition: 0,
				},
				{
					Form:             wasm.CompositeFormStruct,
					Fields:           []wasm.FieldType{{ValueType: wasm.ValueTypeI64}},
					RecGroupSize:     2,
					RecGroupPosition: 1,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode the type section manually (we can't call the unexported
			// encodeTypeSection, but we can manually build the same shape
			// the decoder consumes: the section vec count + encoded types).
			//
			// We emit just the type section payload (no section ID / size
			// prefix) and call decodeTypeSection directly.
			payload := encodeTypeSectionPayload(t, tt.in)
			r := bytes.NewReader(payload)
			got, err := decodeTypeSection(gcFeatures, r)
			require.NoError(t, err)
			require.Equal(t, len(tt.in), len(got))
			for i, want := range tt.in {
				// Clear the cached string before comparison since the encoder
				// won't populate that field on the input fixture.
				want.String() // populate
				got[i].String()
				require.Equal(t, want.Form, got[i].Form, "type[%d].Form", i)
				require.Equal(t, want.Params, got[i].Params, "type[%d].Params", i)
				require.Equal(t, want.Results, got[i].Results, "type[%d].Results", i)
				require.Equal(t, want.Fields, got[i].Fields, "type[%d].Fields", i)
				require.Equal(t, want.ArrayField, got[i].ArrayField, "type[%d].ArrayField", i)
				require.Equal(t, want.Final, got[i].Final, "type[%d].Final", i)
				require.Equal(t, want.RecGroupSize, got[i].RecGroupSize, "type[%d].RecGroupSize", i)
				require.Equal(t, want.RecGroupPosition, got[i].RecGroupPosition, "type[%d].RecGroupPosition", i)
				if want.SuperTypeIndex == nil {
					require.Nil(t, got[i].SuperTypeIndex, "type[%d].SuperTypeIndex", i)
				} else {
					require.NotNil(t, got[i].SuperTypeIndex)
					require.Equal(t, *want.SuperTypeIndex, *got[i].SuperTypeIndex)
				}
			}
		})
	}
}

// encodeTypeSectionPayload reproduces what binaryencoding.encodeTypeSection
// emits, minus the section ID and size-prefix bytes — yielding just the
// vec count plus the encoded entries that decodeTypeSection expects.
func encodeTypeSectionPayload(t *testing.T, types []wasm.FunctionType) []byte {
	t.Helper()
	entryCount := uint32(0)
	for i := 0; i < len(types); {
		if types[i].RecGroupSize > 1 {
			entryCount++
			i += types[i].RecGroupSize
		} else {
			entryCount++
			i++
		}
	}
	out := []byte{byte(entryCount)} // works for entryCount < 128
	for i := 0; i < len(types); {
		if types[i].RecGroupSize > 1 {
			out = append(out, 0x4E)
			out = append(out, byte(types[i].RecGroupSize))
			for j := 0; j < types[i].RecGroupSize; j++ {
				out = append(out, binaryencoding.EncodeFunctionType(&types[i+j])...)
			}
			i += types[i].RecGroupSize
		} else {
			out = append(out, binaryencoding.EncodeFunctionType(&types[i])...)
			i++
		}
	}
	return out
}
