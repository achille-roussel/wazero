package binary

import (
	"bytes"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const gcFeatures = api.CoreFeaturesV2 | experimental.CoreFeaturesGC

func TestDecodeTypeSection_RecGroupShorthand(t *testing.T) {
	// Two standalone shorthand func types — pre-GC behavior unchanged.
	// vec(type) length=2; each is 0x60 ([]->[i32]).
	in := []byte{
		0x02,                   // type-section count
		0x60, 0x00, 0x01, 0x7F, // func ()->[i32]
		0x60, 0x00, 0x01, 0x7F, // func ()->[i32]
	}
	r := bytes.NewReader(in)
	got, err := decodeTypeSection(gcFeatures, r)
	require.NoError(t, err)
	require.Equal(t, 2, len(got))
	require.Equal(t, wasm.CompositeFormFunc, got[0].Form)
	require.Equal(t, 0, got[0].RecGroupSize)
	require.Equal(t, []wasm.ValueType{wasm.ValueTypeI32}, got[0].Results)
}

func TestDecodeTypeSection_RecGroup(t *testing.T) {
	// One rec group of two func types.
	// 0x4E rec, count=2, then two 0x60 func types.
	in := []byte{
		0x01,       // type-section count
		0x4E, 0x02, // rec, group-size=2
		0x60, 0x00, 0x01, 0x7F, // func ()->[i32]
		0x60, 0x01, 0x7E, 0x00, // func [i64]->[]
	}
	r := bytes.NewReader(in)
	got, err := decodeTypeSection(gcFeatures, r)
	require.NoError(t, err)
	require.Equal(t, 2, len(got))
	require.Equal(t, 2, got[0].RecGroupSize)
	require.Equal(t, 0, got[0].RecGroupPosition)
	require.Equal(t, 2, got[1].RecGroupSize)
	require.Equal(t, 1, got[1].RecGroupPosition)
	require.Equal(t, []wasm.ValueType{wasm.ValueTypeI32}, got[0].Results)
	require.Equal(t, []wasm.ValueType{wasm.ValueTypeI64}, got[1].Params)
}

func TestDecodeTypeSection_SubFinalNoSupers(t *testing.T) {
	// 0x4F sub final with 0 supertypes wrapping a func type — same as
	// shorthand 0x60, but expressed explicitly.
	in := []byte{
		0x01,       // type-section count
		0x4F, 0x00, // sub final, 0 supertypes
		0x60, 0x00, 0x01, 0x7F, // func ()->[i32]
	}
	r := bytes.NewReader(in)
	got, err := decodeTypeSection(gcFeatures, r)
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, wasm.CompositeFormFunc, got[0].Form)
	require.True(t, got[0].Final)
	require.Nil(t, got[0].SuperTypeIndex)
}

func TestDecodeTypeSection_SubWithSuper(t *testing.T) {
	// 0x50 sub (non-final) with one supertype index 0, struct body with one
	// mutable i32 field.
	in := []byte{
		0x02,             // type-section count
		0x60, 0x00, 0x00, // type 0: func ()->[]
		0x50, 0x01, 0x00, // sub, 1 supertype = type index 0
		0x5F, 0x01, 0x7F, 0x01, // struct { mut i32 }
	}
	r := bytes.NewReader(in)
	got, err := decodeTypeSection(gcFeatures, r)
	require.NoError(t, err)
	require.Equal(t, 2, len(got))
	require.Equal(t, wasm.CompositeFormStruct, got[1].Form)
	require.False(t, got[1].Final)
	require.NotNil(t, got[1].SuperTypeIndex)
	require.Equal(t, uint32(0), *got[1].SuperTypeIndex)
	require.Equal(t, 1, len(got[1].Fields))
	require.Equal(t, wasm.ValueTypeI32, got[1].Fields[0].ValueType)
	require.True(t, got[1].Fields[0].Mutable)
}

func TestDecodeTypeSection_StructShorthand(t *testing.T) {
	// 0x5F struct shorthand with two fields: const i32, mut i64.
	in := []byte{
		0x01,                               // type-section count
		0x5F, 0x02, 0x7F, 0x00, 0x7E, 0x01, // struct { i32, mut i64 }
	}
	r := bytes.NewReader(in)
	got, err := decodeTypeSection(gcFeatures, r)
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, wasm.CompositeFormStruct, got[0].Form)
	require.Equal(t, 2, len(got[0].Fields))
	require.Equal(t, wasm.ValueTypeI32, got[0].Fields[0].ValueType)
	require.False(t, got[0].Fields[0].Mutable)
	require.Equal(t, wasm.ValueTypeI64, got[0].Fields[1].ValueType)
	require.True(t, got[0].Fields[1].Mutable)
}

func TestDecodeTypeSection_StructWithPackedFields(t *testing.T) {
	// Struct with one i8 mutable field and one i16 const field.
	in := []byte{
		0x01,                               // type-section count
		0x5F, 0x02, 0x78, 0x01, 0x77, 0x00, // struct { mut i8, i16 }
	}
	r := bytes.NewReader(in)
	got, err := decodeTypeSection(gcFeatures, r)
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, wasm.PackedTypeI8, got[0].Fields[0].Packed)
	require.True(t, got[0].Fields[0].Mutable)
	require.Equal(t, wasm.PackedTypeI16, got[0].Fields[1].Packed)
	require.False(t, got[0].Fields[1].Mutable)
}

func TestDecodeTypeSection_EmptyStruct(t *testing.T) {
	in := []byte{
		0x01,       // type-section count
		0x5F, 0x00, // struct { }
	}
	r := bytes.NewReader(in)
	got, err := decodeTypeSection(gcFeatures, r)
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, wasm.CompositeFormStruct, got[0].Form)
	require.Nil(t, got[0].Fields)
}

func TestDecodeTypeSection_ArrayShorthand(t *testing.T) {
	// 0x5E array shorthand with mutable i32 element.
	in := []byte{
		0x01,             // type-section count
		0x5E, 0x7F, 0x01, // array (mut i32)
	}
	r := bytes.NewReader(in)
	got, err := decodeTypeSection(gcFeatures, r)
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, wasm.CompositeFormArray, got[0].Form)
	require.Equal(t, wasm.ValueTypeI32, got[0].ArrayField.ValueType)
	require.True(t, got[0].ArrayField.Mutable)
}

func TestDecodeTypeSection_ArrayPacked(t *testing.T) {
	// Array of const-i8.
	in := []byte{
		0x01,             // type-section count
		0x5E, 0x78, 0x00, // array (i8)
	}
	r := bytes.NewReader(in)
	got, err := decodeTypeSection(gcFeatures, r)
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, wasm.CompositeFormArray, got[0].Form)
	require.Equal(t, wasm.PackedTypeI8, got[0].ArrayField.Packed)
	require.False(t, got[0].ArrayField.Mutable)
}

func TestDecodeTypeSection_MixedRecGroup(t *testing.T) {
	// One rec group containing a struct and an array.
	in := []byte{
		0x01,       // type-section count
		0x4E, 0x02, // rec, group-size=2
		0x5F, 0x01, 0x7F, 0x01, // struct { mut i32 }
		0x5E, 0x7E, 0x00, // array (i64)
	}
	r := bytes.NewReader(in)
	got, err := decodeTypeSection(gcFeatures, r)
	require.NoError(t, err)
	require.Equal(t, 2, len(got))
	require.Equal(t, wasm.CompositeFormStruct, got[0].Form)
	require.Equal(t, wasm.CompositeFormArray, got[1].Form)
	require.Equal(t, 2, got[0].RecGroupSize)
	require.Equal(t, 0, got[0].RecGroupPosition)
	require.Equal(t, 2, got[1].RecGroupSize)
	require.Equal(t, 1, got[1].RecGroupPosition)
}

func TestDecodeTypeSection_TooManySupers(t *testing.T) {
	// Sub form with 2 supertype indices — MVP allows at most 1.
	in := []byte{
		0x01,                   // type-section count
		0x50, 0x02, 0x00, 0x01, // sub, 2 supertypes
		0x60, 0x00, 0x00, // func ()->[]
	}
	r := bytes.NewReader(in)
	_, err := decodeTypeSection(gcFeatures, r)
	require.Error(t, err)
}

func TestDecodeTypeSection_InvalidLeadingByte(t *testing.T) {
	in := []byte{
		0x01, // type-section count
		0x00, // invalid sub-type form
	}
	r := bytes.NewReader(in)
	_, err := decodeTypeSection(gcFeatures, r)
	require.Error(t, err)
}

func TestDecodeTypeSection_InvalidMutability(t *testing.T) {
	in := []byte{
		0x01,                   // type-section count
		0x5F, 0x01, 0x7F, 0x02, // struct { i32 with bad mutability byte 0x02 }
	}
	r := bytes.NewReader(in)
	_, err := decodeTypeSection(gcFeatures, r)
	require.Error(t, err)
}
