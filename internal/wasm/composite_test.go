package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestCompositeFormString(t *testing.T) {
	require.Equal(t, "func", CompositeFormFunc.String())
	require.Equal(t, "struct", CompositeFormStruct.String())
	require.Equal(t, "array", CompositeFormArray.String())
}

func TestPackedTypeString(t *testing.T) {
	require.Equal(t, "i8", PackedTypeI8.String())
	require.Equal(t, "i16", PackedTypeI16.String())
	require.Equal(t, "<none>", PackedTypeNone.String())
}

func TestFieldTypeString(t *testing.T) {
	tests := []struct {
		name  string
		field FieldType
		want  string
	}{
		{"const i32", FieldType{ValueType: ValueTypeI32}, "i32"},
		{"mut i64", FieldType{ValueType: ValueTypeI64, Mutable: true}, "mut i64"},
		{"const i8", FieldType{Packed: PackedTypeI8}, "i8"},
		{"mut i16", FieldType{Packed: PackedTypeI16, Mutable: true}, "mut i16"},
		{"const (ref null any)", FieldType{ValueType: ValueTypeFuncref, RefInfo: &ValueTypeRef{Nullable: true, HeapKind: HeapTypeKindAny}}, "(ref null any)"},
		{"mut (ref 5)", FieldType{ValueType: ValueTypeFuncref, RefInfo: &ValueTypeRef{HeapKind: HeapTypeKindConcrete, TypeIdx: 5}, Mutable: true}, "mut (ref 5)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.field.String())
		})
	}
}
