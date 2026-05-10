package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestHeapTypeKindFromBinary(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		wantKind HeapTypeKind
		wantIdx  uint32
		wantOK   bool
	}{
		{"func", -16, HeapTypeKindFunc, 0, true},
		{"extern", -17, HeapTypeKindExtern, 0, true},
		{"exn", -23, HeapTypeKindExn, 0, true},
		{"any", -18, HeapTypeKindAny, 0, true},
		{"eq", -19, HeapTypeKindEq, 0, true},
		{"i31", -20, HeapTypeKindI31, 0, true},
		{"struct", -21, HeapTypeKindStruct, 0, true},
		{"array", -22, HeapTypeKindArray, 0, true},
		{"none", -15, HeapTypeKindBottom, 0, true},
		{"nofunc", -13, HeapTypeKindNoFunc, 0, true},
		{"noextern", -14, HeapTypeKindNoExtern, 0, true},
		{"noexn", -12, HeapTypeKindNoExn, 0, true},
		{"concrete 0", 0, HeapTypeKindConcrete, 0, true},
		{"concrete 7", 7, HeapTypeKindConcrete, 7, true},
		{"concrete max", 1 << 30, HeapTypeKindConcrete, 1 << 30, true},
		{"unknown negative", -100, HeapTypeKindUnknown, 0, false},
		{"unknown -11", -11, HeapTypeKindUnknown, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKind, gotIdx, gotOK := HeapTypeKindFromBinary(tt.input)
			require.Equal(t, tt.wantOK, gotOK)
			require.Equal(t, tt.wantKind, gotKind)
			require.Equal(t, tt.wantIdx, gotIdx)
		})
	}
}

func TestHeapTypeKindFromAbstractByte(t *testing.T) {
	// Every abstract kind should round-trip via shorthand byte.
	abstractKinds := []HeapTypeKind{
		HeapTypeKindNoFunc, HeapTypeKindNoExtern, HeapTypeKindBottom,
		HeapTypeKindFunc, HeapTypeKindExtern, HeapTypeKindAny,
		HeapTypeKindEq, HeapTypeKindI31, HeapTypeKindStruct,
		HeapTypeKindArray, HeapTypeKindExn, HeapTypeKindNoExn,
	}
	for _, k := range abstractKinds {
		t.Run(k.String(), func(t *testing.T) {
			b, ok := k.AbstractShorthandByte()
			require.True(t, ok, "expected shorthand byte for kind %s", k)
			gotKind, gotOK := HeapTypeKindFromAbstractByte(b)
			require.True(t, gotOK)
			require.Equal(t, k, gotKind)
		})
	}

	// Concrete has no shorthand byte.
	_, ok := HeapTypeKindConcrete.AbstractShorthandByte()
	require.False(t, ok)

	// Unknown bytes don't decode.
	_, ok = HeapTypeKindFromAbstractByte(0x00)
	require.False(t, ok)
	_, ok = HeapTypeKindFromAbstractByte(0x60)
	require.False(t, ok)
}

func TestHeapTypeKindString(t *testing.T) {
	tests := []struct {
		kind HeapTypeKind
		want string
	}{
		{HeapTypeKindFunc, "func"},
		{HeapTypeKindExtern, "extern"},
		{HeapTypeKindExn, "exn"},
		{HeapTypeKindAny, "any"},
		{HeapTypeKindEq, "eq"},
		{HeapTypeKindI31, "i31"},
		{HeapTypeKindStruct, "struct"},
		{HeapTypeKindArray, "array"},
		{HeapTypeKindBottom, "none"},
		{HeapTypeKindNoFunc, "nofunc"},
		{HeapTypeKindNoExtern, "noextern"},
		{HeapTypeKindNoExn, "noexn"},
		{HeapTypeKindConcrete, "concrete"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, tt.kind.String())
	}
}

func TestValueTypeRefString(t *testing.T) {
	tests := []struct {
		name string
		ref  ValueTypeRef
		want string
	}{
		{"nullable func", ValueTypeRef{Nullable: true, HeapKind: HeapTypeKindFunc}, "(ref null func)"},
		{"non-null func", ValueTypeRef{HeapKind: HeapTypeKindFunc}, "(ref func)"},
		{"nullable any", ValueTypeRef{Nullable: true, HeapKind: HeapTypeKindAny}, "(ref null any)"},
		{"non-null any", ValueTypeRef{HeapKind: HeapTypeKindAny}, "(ref any)"},
		{"nullable concrete 5", ValueTypeRef{Nullable: true, HeapKind: HeapTypeKindConcrete, TypeIdx: 5}, "(ref null 5)"},
		{"non-null concrete 0", ValueTypeRef{HeapKind: HeapTypeKindConcrete, TypeIdx: 0}, "(ref 0)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.ref.String())
		})
	}
}

func TestValueTypeRefShorthandByte(t *testing.T) {
	// Nullable abstract references have shorthand bytes.
	r := ValueTypeRef{Nullable: true, HeapKind: HeapTypeKindFunc}
	b, ok := r.ShorthandByte()
	require.True(t, ok)
	require.Equal(t, byte(0x70), b)

	r = ValueTypeRef{Nullable: true, HeapKind: HeapTypeKindAny}
	b, ok = r.ShorthandByte()
	require.True(t, ok)
	require.Equal(t, byte(0x6E), b)

	// Non-nullable abstract references do NOT have a single-byte shorthand;
	// they require the 0x64 prefix encoding.
	r = ValueTypeRef{Nullable: false, HeapKind: HeapTypeKindFunc}
	_, ok = r.ShorthandByte()
	require.False(t, ok)

	// Concrete references never have a single-byte shorthand.
	r = ValueTypeRef{Nullable: true, HeapKind: HeapTypeKindConcrete, TypeIdx: 0}
	_, ok = r.ShorthandByte()
	require.False(t, ok)
}

func TestValueTypeRefIsAbstract(t *testing.T) {
	require.True(t, ValueTypeRef{HeapKind: HeapTypeKindFunc}.IsAbstract())
	require.True(t, ValueTypeRef{HeapKind: HeapTypeKindAny}.IsAbstract())
	require.False(t, ValueTypeRef{HeapKind: HeapTypeKindConcrete, TypeIdx: 3}.IsAbstract())
	require.False(t, ValueTypeRef{HeapKind: HeapTypeKindUnknown}.IsAbstract())
}

// abstractKinds is the canonical list of abstract heap-type kinds used by
// the subtype matrix tests below. Order matters only for readability.
var abstractKinds = []HeapTypeKind{
	HeapTypeKindAny, HeapTypeKindEq,
	HeapTypeKindI31, HeapTypeKindStruct, HeapTypeKindArray,
	HeapTypeKindBottom,
	HeapTypeKindFunc, HeapTypeKindNoFunc,
	HeapTypeKindExtern, HeapTypeKindNoExtern,
	HeapTypeKindExn, HeapTypeKindNoExn,
}

func TestIsAbstractSubtypeOf_Reflexive(t *testing.T) {
	for _, k := range abstractKinds {
		t.Run(k.String(), func(t *testing.T) {
			require.True(t, k.IsAbstractSubtypeOf(k))
		})
	}
}

func TestIsAbstractSubtypeOf_AnyHierarchy(t *testing.T) {
	// Every kind in the any hierarchy must be a subtype of `any`.
	anyHierarchy := []HeapTypeKind{
		HeapTypeKindEq, HeapTypeKindI31, HeapTypeKindStruct,
		HeapTypeKindArray, HeapTypeKindBottom,
	}
	for _, k := range anyHierarchy {
		require.True(t, k.IsAbstractSubtypeOf(HeapTypeKindAny),
			"%s should be <: any", k)
	}

	// Strict subtypes of `eq`: i31, struct, array (and bottom).
	for _, k := range []HeapTypeKind{HeapTypeKindI31, HeapTypeKindStruct, HeapTypeKindArray, HeapTypeKindBottom} {
		require.True(t, k.IsAbstractSubtypeOf(HeapTypeKindEq), "%s should be <: eq", k)
	}

	// i31, struct, array are NOT subtypes of each other.
	siblings := []HeapTypeKind{HeapTypeKindI31, HeapTypeKindStruct, HeapTypeKindArray}
	for _, a := range siblings {
		for _, b := range siblings {
			if a == b {
				continue
			}
			require.False(t, a.IsAbstractSubtypeOf(b), "%s should NOT be <: %s", a, b)
		}
	}

	// `any` is not a subtype of anything strictly below it.
	for _, k := range []HeapTypeKind{HeapTypeKindEq, HeapTypeKindI31, HeapTypeKindStruct, HeapTypeKindArray, HeapTypeKindBottom} {
		require.False(t, HeapTypeKindAny.IsAbstractSubtypeOf(k),
			"any should NOT be <: %s", k)
	}
}

func TestIsAbstractSubtypeOf_FuncHierarchy(t *testing.T) {
	// nofunc <: func, but func is not <: anything below it.
	require.True(t, HeapTypeKindNoFunc.IsAbstractSubtypeOf(HeapTypeKindFunc))
	require.False(t, HeapTypeKindFunc.IsAbstractSubtypeOf(HeapTypeKindNoFunc))
}

func TestIsAbstractSubtypeOf_ExternHierarchy(t *testing.T) {
	require.True(t, HeapTypeKindNoExtern.IsAbstractSubtypeOf(HeapTypeKindExtern))
	require.False(t, HeapTypeKindExtern.IsAbstractSubtypeOf(HeapTypeKindNoExtern))
}

func TestIsAbstractSubtypeOf_ExnHierarchy(t *testing.T) {
	require.True(t, HeapTypeKindNoExn.IsAbstractSubtypeOf(HeapTypeKindExn))
	require.False(t, HeapTypeKindExn.IsAbstractSubtypeOf(HeapTypeKindNoExn))
}

func TestIsAbstractSubtypeOf_DisjointHierarchies(t *testing.T) {
	// The four hierarchies (any, func, extern, exn) are disjoint: no
	// cross-hierarchy subtyping. Spot-check across-hierarchy pairs.
	pairs := [][2]HeapTypeKind{
		{HeapTypeKindAny, HeapTypeKindFunc},
		{HeapTypeKindFunc, HeapTypeKindAny},
		{HeapTypeKindEq, HeapTypeKindFunc},
		{HeapTypeKindStruct, HeapTypeKindExtern},
		{HeapTypeKindI31, HeapTypeKindExn},
		{HeapTypeKindBottom, HeapTypeKindFunc}, // any-hierarchy bottom != func
		{HeapTypeKindNoFunc, HeapTypeKindAny},  // func-hierarchy bottom != any
		{HeapTypeKindNoExtern, HeapTypeKindExn},
		{HeapTypeKindNoExn, HeapTypeKindAny},
		{HeapTypeKindArray, HeapTypeKindExn},
	}
	for _, p := range pairs {
		require.False(t, p[0].IsAbstractSubtypeOf(p[1]),
			"%s should NOT be <: %s (disjoint hierarchies)", p[0], p[1])
	}
}

func TestIsAbstractSubtypeOf_ConcreteAndUnknown(t *testing.T) {
	// Concrete on either side is rejected at this layer; module context
	// is required for proper concrete-ref subtyping.
	require.False(t, HeapTypeKindConcrete.IsAbstractSubtypeOf(HeapTypeKindAny))
	require.False(t, HeapTypeKindAny.IsAbstractSubtypeOf(HeapTypeKindConcrete))
	require.False(t, HeapTypeKindConcrete.IsAbstractSubtypeOf(HeapTypeKindConcrete))

	// Unknown on either side never matches.
	require.False(t, HeapTypeKindUnknown.IsAbstractSubtypeOf(HeapTypeKindAny))
	require.False(t, HeapTypeKindAny.IsAbstractSubtypeOf(HeapTypeKindUnknown))
}

func TestIsAbstractSubtypeOf_BottomKindsAcrossHierarchy(t *testing.T) {
	// Each bottom kind is a subtype of every other kind in its hierarchy.
	for _, k := range []HeapTypeKind{HeapTypeKindAny, HeapTypeKindEq, HeapTypeKindI31, HeapTypeKindStruct, HeapTypeKindArray} {
		require.True(t, HeapTypeKindBottom.IsAbstractSubtypeOf(k),
			"none should be <: %s", k)
	}
	require.True(t, HeapTypeKindNoFunc.IsAbstractSubtypeOf(HeapTypeKindFunc))
	require.True(t, HeapTypeKindNoExtern.IsAbstractSubtypeOf(HeapTypeKindExtern))
	require.True(t, HeapTypeKindNoExn.IsAbstractSubtypeOf(HeapTypeKindExn))

	// Bottom kinds are NOT subtypes of each other across hierarchies.
	require.False(t, HeapTypeKindBottom.IsAbstractSubtypeOf(HeapTypeKindNoFunc))
	require.False(t, HeapTypeKindNoFunc.IsAbstractSubtypeOf(HeapTypeKindBottom))
	require.False(t, HeapTypeKindNoExn.IsAbstractSubtypeOf(HeapTypeKindNoFunc))
}
