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

func TestCanonicalTypeKey_NoSuperType(t *testing.T) {
	// Without a SuperTypeIndex the canonical key matches the plain key()
	// modulo the legacy cached-string form (which key() also produces).
	f := FunctionType{Form: CompositeFormFunc, Results: []ValueType{ValueTypeI32}}
	require.Equal(t, "v_i32", canonicalTypeKey(&f, 0))
	require.Equal(t, "v_i32", canonicalTypeKey(&f, 17))
}

func TestCanonicalTypeKey_IntraGroupSupertype(t *testing.T) {
	sup := uint32(5) // module-level position 5
	// Type at module position 6 with rec-group position 1 and size 2.
	// The rec group spans module positions [5..6], so SuperTypeIndex 5 is
	// the entry at relative rec-position 0 within the SAME group.
	f := FunctionType{
		Form:             CompositeFormStruct,
		Fields:           []FieldType{{ValueType: ValueTypeI32}},
		SuperTypeIndex:   &sup,
		RecGroupSize:     2,
		RecGroupPosition: 1,
	}
	got := canonicalTypeKey(&f, 6)
	require.Equal(t, "struct{i32}|sup=rec.0|rec1/2", got)

	// Same type at a different module position (rec group starting at 100)
	// should yield the IDENTICAL canonical key — that's the iso-recursive
	// invariant.
	supShifted := uint32(100)
	f2 := FunctionType{
		Form:             CompositeFormStruct,
		Fields:           []FieldType{{ValueType: ValueTypeI32}},
		SuperTypeIndex:   &supShifted,
		RecGroupSize:     2,
		RecGroupPosition: 1,
	}
	got2 := canonicalTypeKey(&f2, 101)
	require.Equal(t, got, got2)
}

func TestCanonicalTypeKey_ExtraGroupSupertype(t *testing.T) {
	sup := uint32(2)
	// Type at module position 6 with rec-group position 1 and size 2.
	// SuperTypeIndex 2 is outside the rec group [5..6].
	f := FunctionType{
		Form:             CompositeFormFunc,
		Params:           []ValueType{ValueTypeI32},
		SuperTypeIndex:   &sup,
		RecGroupSize:     2,
		RecGroupPosition: 1,
	}
	got := canonicalTypeKey(&f, 6)
	require.Equal(t, "i32_v|sup=abs.2|rec1/2", got)

	// Note: this is the conservative encoding; a future Phase 4 commit
	// will resolve abs.2 to the supertype's TypeID for true cross-module
	// canonicalization through subtype chains.
}

func TestCanonicalTypeKey_StandaloneWithSupertype(t *testing.T) {
	sup := uint32(0)
	// Standalone (non-rec-group) type at module position 1 with supertype 0.
	// The "group" is implicitly size 1 starting at the type's own position,
	// so SuperTypeIndex 0 is outside it.
	f := FunctionType{
		Form:           CompositeFormStruct,
		Fields:         []FieldType{{ValueType: ValueTypeI32}},
		SuperTypeIndex: &sup,
	}
	got := canonicalTypeKey(&f, 1)
	require.Equal(t, "struct{i32}|sup=abs.0", got)
}

func TestCanonicalTypeKey_RecGroupSelfReference(t *testing.T) {
	// A self-recursive type: position 0 in a size-1 rec group with supertype
	// pointing to itself. Should canonicalize as rec.0 (intra-group).
	sup := uint32(3)
	f := FunctionType{
		Form:             CompositeFormStruct,
		Fields:           []FieldType{{ValueType: ValueTypeI32}},
		SuperTypeIndex:   &sup,
		RecGroupSize:     1,
		RecGroupPosition: 0,
	}
	// RecGroupSize is 1, so the |rec0/1 suffix isn't appended (it's added
	// only when RecGroupSize > 1). But the iso-recursive logic still
	// recognises that sup is within the size-1 group at module position 3
	// itself (treated as a single-type group). Note the function uses
	// groupSize=max(RecGroupSize, 1) in this calculation.
	got := canonicalTypeKey(&f, 3)
	require.Equal(t, "struct{i32}|sup=rec.0", got)
}
