package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestFunctionType_Key_Func(t *testing.T) {
	// Backward compatibility: existing key shape for plain func types.
	tests := []struct {
		name string
		f    FunctionType
		want string
	}{
		{"empty func", FunctionType{}, "v_v"},
		{"i32 -> i32", FunctionType{Params: []ValueType{ValueTypeI32}, Results: []ValueType{ValueTypeI32}}, "i32_i32"},
		{"(i32,i64) -> ()", FunctionType{Params: []ValueType{ValueTypeI32, ValueTypeI64}}, "i32i64_v"},
		{"() -> i32 in rec group", FunctionType{Results: []ValueType{ValueTypeI32}, RecGroupSize: 2, RecGroupPosition: 1}, "v_i32|rec1/2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.f.key()
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFunctionType_Key_Struct(t *testing.T) {
	tests := []struct {
		name string
		f    FunctionType
		want string
	}{
		{
			name: "empty struct",
			f:    FunctionType{Form: CompositeFormStruct},
			want: "struct{}",
		},
		{
			name: "struct with one const i32 field",
			f:    FunctionType{Form: CompositeFormStruct, Fields: []FieldType{{ValueType: ValueTypeI32}}},
			want: "struct{i32}",
		},
		{
			name: "struct with mut i64 + const i8",
			f:    FunctionType{Form: CompositeFormStruct, Fields: []FieldType{{ValueType: ValueTypeI64, Mutable: true}, {Packed: PackedTypeI8}}},
			want: "struct{mut i64,i8}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.f.key())
		})
	}
}

func TestFunctionType_Key_Array(t *testing.T) {
	tests := []struct {
		name string
		f    FunctionType
		want string
	}{
		{
			"mut i32 array",
			FunctionType{Form: CompositeFormArray, ArrayField: FieldType{ValueType: ValueTypeI32, Mutable: true}},
			"array(mut i32)",
		},
		{
			"i8-packed array",
			FunctionType{Form: CompositeFormArray, ArrayField: FieldType{Packed: PackedTypeI8}},
			"array(i8)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.f.key())
		})
	}
}

func TestFunctionType_Key_SupertypeAndFinal(t *testing.T) {
	sup := uint32(3)

	f := FunctionType{Form: CompositeFormFunc, SuperTypeIndex: &sup}
	require.Equal(t, "v_v|sup=3", f.key())

	f = FunctionType{Form: CompositeFormFunc, Final: true}
	require.Equal(t, "v_v|final", f.key())

	f = FunctionType{Form: CompositeFormStruct, Fields: []FieldType{{ValueType: ValueTypeI32}}, SuperTypeIndex: &sup, Final: true}
	require.Equal(t, "struct{i32}|sup=3|final", f.key())
}

func TestFunctionType_Key_DistinctForms(t *testing.T) {
	// Same Params/Results bytes should not collide between forms — a struct
	// with one i32 field has a different key than a func ()->i32.
	funcType := FunctionType{Form: CompositeFormFunc, Results: []ValueType{ValueTypeI32}}
	structType := FunctionType{Form: CompositeFormStruct, Fields: []FieldType{{ValueType: ValueTypeI32}}}
	arrayType := FunctionType{Form: CompositeFormArray, ArrayField: FieldType{ValueType: ValueTypeI32}}
	require.NotEqual(t, funcType.key(), structType.key())
	require.NotEqual(t, funcType.key(), arrayType.key())
	require.NotEqual(t, structType.key(), arrayType.key())
}
