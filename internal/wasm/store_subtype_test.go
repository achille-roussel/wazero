package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// newSubtypeTestStore constructs a fresh Store with no engine for the
// subtype-display tests below.
func newSubtypeTestStore() *Store {
	return NewStore(0, nil)
}

func TestIsSubtype_NoSupertypes(t *testing.T) {
	// Single standalone type, no supertype: it's a subtype only of itself.
	s := newSubtypeTestStore()
	ids, err := s.GetFunctionTypeIDs([]FunctionType{
		{Form: CompositeFormFunc, Params: []ValueType{ValueTypeI32}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(ids))

	require.True(t, s.IsSubtype(ids[0], ids[0]))
	// An unregistered ID is not a subtype of anything.
	require.False(t, s.IsSubtype(FunctionTypeID(99), ids[0]))
	require.False(t, s.IsSubtype(ids[0], FunctionTypeID(99)))
}

func TestIsSubtype_LinearChain(t *testing.T) {
	// Three types in a linear supertype chain:
	//   t0  (no supertype)
	//   t1  super = 0
	//   t2  super = 1
	// Depths: 0, 1, 2.
	s := newSubtypeTestStore()
	zero := uint32(0)
	one := uint32(1)
	ts := []FunctionType{
		{Form: CompositeFormStruct},
		{Form: CompositeFormStruct, SuperTypeIndex: &zero},
		{Form: CompositeFormStruct, SuperTypeIndex: &one},
	}
	ids, err := s.GetFunctionTypeIDs(ts)
	require.NoError(t, err)
	require.Equal(t, 3, len(ids))

	// Reflexive.
	require.True(t, s.IsSubtype(ids[0], ids[0]))
	require.True(t, s.IsSubtype(ids[1], ids[1]))
	require.True(t, s.IsSubtype(ids[2], ids[2]))

	// Forward chain.
	require.True(t, s.IsSubtype(ids[1], ids[0]))
	require.True(t, s.IsSubtype(ids[2], ids[0]))
	require.True(t, s.IsSubtype(ids[2], ids[1]))

	// Reverse direction (super <: sub) is false except reflexively.
	require.False(t, s.IsSubtype(ids[0], ids[1]))
	require.False(t, s.IsSubtype(ids[0], ids[2]))
	require.False(t, s.IsSubtype(ids[1], ids[2]))
}

func TestIsSubtype_ForwardRefInRecGroup(t *testing.T) {
	// Within a rec group, the spec allows a type to reference a supertype
	// that is declared LATER in the same group. Verify the two-pass
	// resolution handles that.
	s := newSubtypeTestStore()
	one := uint32(1)
	ts := []FunctionType{
		// Position 0: supertype is position 1 (forward ref).
		{Form: CompositeFormStruct, SuperTypeIndex: &one, RecGroupSize: 2, RecGroupPosition: 0},
		// Position 1: no supertype.
		{Form: CompositeFormStruct, RecGroupSize: 2, RecGroupPosition: 1},
	}
	ids, err := s.GetFunctionTypeIDs(ts)
	require.NoError(t, err)
	require.Equal(t, 2, len(ids))

	// Position 0 is now a subtype of position 1.
	require.True(t, s.IsSubtype(ids[0], ids[1]))
	require.False(t, s.IsSubtype(ids[1], ids[0]))
}

func TestIsSubtype_CycleRejected(t *testing.T) {
	// A <: B, B <: A is a cycle and must be rejected.
	s := newSubtypeTestStore()
	zero := uint32(0)
	one := uint32(1)
	ts := []FunctionType{
		{Form: CompositeFormStruct, SuperTypeIndex: &one, RecGroupSize: 2, RecGroupPosition: 0},
		{Form: CompositeFormStruct, SuperTypeIndex: &zero, RecGroupSize: 2, RecGroupPosition: 1},
	}
	_, err := s.GetFunctionTypeIDs(ts)
	require.Error(t, err)
}

func TestIsSubtype_OutOfRangeSuper(t *testing.T) {
	s := newSubtypeTestStore()
	huge := uint32(42)
	ts := []FunctionType{
		{Form: CompositeFormStruct, SuperTypeIndex: &huge},
	}
	_, err := s.GetFunctionTypeIDs(ts)
	require.Error(t, err)
}

func TestIsSubtype_SeparateRegistrationsShareID(t *testing.T) {
	// Two separate GetFunctionTypeIDs calls for structurally identical
	// types yield the SAME FunctionTypeID and the same subtype display,
	// so cross-"module" subtype checks work.
	s := newSubtypeTestStore()
	ts := []FunctionType{
		{Form: CompositeFormStruct, Fields: []FieldType{{ValueType: ValueTypeI32}}},
	}
	ids1, err := s.GetFunctionTypeIDs(ts)
	require.NoError(t, err)
	ids2, err := s.GetFunctionTypeIDs(ts)
	require.NoError(t, err)
	require.Equal(t, ids1[0], ids2[0], "structurally identical types should share a TypeID")
	require.True(t, s.IsSubtype(ids1[0], ids2[0]))
}

func TestTypeForm(t *testing.T) {
	// TypeForm returns the canonical composite form, populated at
	// registration time. Used by ref.test for the abstract struct/array
	// heap-type targets.
	s := newSubtypeTestStore()
	ts := []FunctionType{
		{Form: CompositeFormFunc, Params: []ValueType{ValueTypeI32}},
		{Form: CompositeFormStruct, Fields: []FieldType{{ValueType: ValueTypeI32}}},
		{Form: CompositeFormArray, ArrayField: FieldType{ValueType: ValueTypeI32}},
	}
	ids, err := s.GetFunctionTypeIDs(ts)
	require.NoError(t, err)

	require.True(t, s.IsResolvedType(ids[0]))
	require.True(t, s.IsResolvedType(ids[1]))
	require.True(t, s.IsResolvedType(ids[2]))
	require.False(t, s.IsResolvedType(FunctionTypeID(99)))

	require.Equal(t, CompositeFormFunc, s.TypeForm(ids[0]))
	require.Equal(t, CompositeFormStruct, s.TypeForm(ids[1]))
	require.Equal(t, CompositeFormArray, s.TypeForm(ids[2]))
}
