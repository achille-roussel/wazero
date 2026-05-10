package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestIsValueTypeSubtypeOf_Numeric(t *testing.T) {
	require.True(t, IsValueTypeSubtypeOf(ValueTypeI32, nil, ValueTypeI32, nil))
	require.False(t, IsValueTypeSubtypeOf(ValueTypeI32, nil, ValueTypeI64, nil))
	require.False(t, IsValueTypeSubtypeOf(ValueTypeI64, nil, ValueTypeI32, nil))
	require.True(t, IsValueTypeSubtypeOf(ValueTypeV128, nil, ValueTypeV128, nil))
}

func TestIsValueTypeSubtypeOf_AbstractRefs_Nullability(t *testing.T) {
	nullableFunc := (*ValueTypeRef)(nil) // nil = shorthand reads as nullable
	nonNullFunc := &ValueTypeRef{Nullable: false, HeapKind: HeapTypeKindFunc}

	// Non-nullable <: nullable.
	require.True(t, IsValueTypeSubtypeOf(ValueTypeFuncref, nonNullFunc, ValueTypeFuncref, nullableFunc))
	// Nullable NOT <: non-nullable.
	require.False(t, IsValueTypeSubtypeOf(ValueTypeFuncref, nullableFunc, ValueTypeFuncref, nonNullFunc))
	// Reflexive.
	require.True(t, IsValueTypeSubtypeOf(ValueTypeFuncref, nullableFunc, ValueTypeFuncref, nullableFunc))
	require.True(t, IsValueTypeSubtypeOf(ValueTypeFuncref, nonNullFunc, ValueTypeFuncref, nonNullFunc))
}

func TestIsValueTypeSubtypeOf_AbstractHierarchy(t *testing.T) {
	// nofunc <: func.
	noFunc := &ValueTypeRef{Nullable: true, HeapKind: HeapTypeKindNoFunc}
	fn := (*ValueTypeRef)(nil) // shorthand funcref byte
	require.True(t, IsValueTypeSubtypeOf(ValueTypeNoFuncref, noFunc, ValueTypeFuncref, fn))

	// Hierarchies disjoint: any not <: func.
	any := (*ValueTypeRef)(nil)
	require.False(t, IsValueTypeSubtypeOf(ValueTypeAnyref, any, ValueTypeFuncref, fn))
	require.False(t, IsValueTypeSubtypeOf(ValueTypeFuncref, fn, ValueTypeAnyref, any))

	// i31 <: eq <: any.
	i31 := (*ValueTypeRef)(nil)
	eq := (*ValueTypeRef)(nil)
	require.True(t, IsValueTypeSubtypeOf(ValueTypeI31ref, i31, ValueTypeEqref, eq))
	require.True(t, IsValueTypeSubtypeOf(ValueTypeI31ref, i31, ValueTypeAnyref, any))
	require.True(t, IsValueTypeSubtypeOf(ValueTypeEqref, eq, ValueTypeAnyref, any))

	// any not <: eq.
	require.False(t, IsValueTypeSubtypeOf(ValueTypeAnyref, any, ValueTypeEqref, eq))
}

func TestIsValueTypeSubtypeOf_ConcreteRefs(t *testing.T) {
	c5 := &ValueTypeRef{Nullable: true, HeapKind: HeapTypeKindConcrete, TypeIdx: 5}
	c5dup := &ValueTypeRef{Nullable: true, HeapKind: HeapTypeKindConcrete, TypeIdx: 5}
	c7 := &ValueTypeRef{Nullable: true, HeapKind: HeapTypeKindConcrete, TypeIdx: 7}

	// Same concrete index: subtype.
	require.True(t, IsValueTypeSubtypeOf(ValueTypeFuncref, c5, ValueTypeFuncref, c5dup))
	// Different concrete indices: conservatively not subtypes (Phase 5
	// will consult Store.IsSubtype to check declared chains).
	require.False(t, IsValueTypeSubtypeOf(ValueTypeFuncref, c5, ValueTypeFuncref, c7))
}

func TestIsValueTypeSubtypeOf_ConcreteVsAbstract(t *testing.T) {
	c5 := &ValueTypeRef{Nullable: true, HeapKind: HeapTypeKindConcrete, TypeIdx: 5}
	any := (*ValueTypeRef)(nil)
	eq := (*ValueTypeRef)(nil)

	// Concrete <: any always.
	require.True(t, IsValueTypeSubtypeOf(ValueTypeFuncref, c5, ValueTypeAnyref, any))
	// Concrete <: eq is conservatively false without module context.
	require.False(t, IsValueTypeSubtypeOf(ValueTypeFuncref, c5, ValueTypeEqref, eq))
	// Abstract <: concrete is always false (no extension downward).
	require.False(t, IsValueTypeSubtypeOf(ValueTypeAnyref, any, ValueTypeFuncref, c5))
}

func TestIsValueTypeSubtypeOf_RichNonNullExn(t *testing.T) {
	// The catch_ref / catch_all_ref case from try_table: a non-nullable
	// exnref delivered by the catch must satisfy a non-nullable exn
	// expected by the target block.
	nonNullExn := &ValueTypeRef{Nullable: false, HeapKind: HeapTypeKindExn}
	require.True(t, IsValueTypeSubtypeOf(ValueTypeExnref, nonNullExn, ValueTypeExnref, nonNullExn))

	// And non-nullable exn is a subtype of nullable exn.
	nullExn := (*ValueTypeRef)(nil)
	require.True(t, IsValueTypeSubtypeOf(ValueTypeExnref, nonNullExn, ValueTypeExnref, nullExn))

	// Nullable exn is NOT a subtype of non-nullable exn (catches the
	// previously-skipped try_table assert_invalid).
	require.False(t, IsValueTypeSubtypeOf(ValueTypeExnref, nullExn, ValueTypeExnref, nonNullExn))
}
