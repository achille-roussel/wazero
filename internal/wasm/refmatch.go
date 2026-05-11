package wasm

import "unsafe"

// RefMatches reports whether a wasm-gc ref value v matches the static
// type (kind, nullable, typeIdx) under the wasm-gc subtype rules.
//
// v is a uint64 in operand-stack representation:
//   - 0 represents the null reference; matches iff nullable.
//   - Low 2 bits 0b01 marks a tagged i31; matches i31 / eq / any.
//   - Low 2 bits 0b11 marks a tagged externref-as-anyref; matches any.
//   - Other non-zero values are heap pointers to either a
//     *WasmStruct / *WasmArray (TypeID at offset 0), or a function
//     instance (TypeID at offset 16). We disambiguate by trying the
//     offset-0 read and consulting Store.IsResolvedType.
//
// Both engines (interpreter and wazevo) share this helper so the
// dispatch is engine-agnostic.
func RefMatches(v uint64, kind HeapTypeKind, nullable bool, typeIdx uint32, mi *ModuleInstance) bool {
	if v == 0 {
		return nullable
	}
	// Externref / nofunc / nofuncref / noexn are in disjoint
	// hierarchies; for ref.test against them, any non-null value
	// matches extern (the static validator ensured the value's
	// origin is correct).
	switch kind {
	case HeapTypeKindExtern:
		return true
	case HeapTypeKindNoExtern:
		return false
	}
	if IsTaggedI31(uintptr(v)) {
		switch kind {
		case HeapTypeKindI31, HeapTypeKindEq, HeapTypeKindAny:
			return true
		case HeapTypeKindConcrete:
			return false
		}
		return false
	}
	if IsTaggedExternAsAny(uintptr(v)) {
		if kind == HeapTypeKindAny {
			return true
		}
		return false
	}
	// Read offset 0 as a FunctionTypeID (safe via double-pointer
	// reinterpretation, avoiding checkptr violations).
	var ptr uintptr = uintptr(v)
	fidPtr := *(**FunctionTypeID)(unsafe.Pointer(&ptr))
	objTypeID := *fidPtr
	store := mi.GetStore()
	if !store.IsResolvedType(objTypeID) {
		// Not a known TypeID at offset 0 ⇒ this is a function
		// reference. Both engines lay out functionInstance with
		// typeID at offset 16.
		funcIDAddr := unsafe.Pointer(&ptr)
		// Bump the local-variable's underlying value by 16 bytes
		// by reinterpreting through a struct. Simpler: load via a
		// pointer-to-pointer-to-byte at offset 16.
		basePtr := *(*unsafe.Pointer)(funcIDAddr)
		funcTypeID := *(*FunctionTypeID)(unsafe.Pointer(uintptr(basePtr) + 16))
		switch kind {
		case HeapTypeKindFunc:
			return true
		case HeapTypeKindNoFunc:
			return false
		case HeapTypeKindAny:
			return false
		case HeapTypeKindConcrete:
			if int(typeIdx) >= len(mi.TypeIDs) {
				return false
			}
			return store.IsSubtype(funcTypeID, mi.TypeIDs[typeIdx])
		}
		return false
	}
	// Heap struct/array dispatch via stored TypeID + form.
	objForm := store.TypeForm(objTypeID)
	switch kind {
	case HeapTypeKindAny, HeapTypeKindEq:
		return objForm == CompositeFormStruct || objForm == CompositeFormArray
	case HeapTypeKindStruct:
		return objForm == CompositeFormStruct
	case HeapTypeKindArray:
		return objForm == CompositeFormArray
	case HeapTypeKindFunc:
		return objForm == CompositeFormFunc
	case HeapTypeKindI31:
		return false
	case HeapTypeKindConcrete:
		if int(typeIdx) >= len(mi.TypeIDs) {
			return false
		}
		return store.IsSubtype(objTypeID, mi.TypeIDs[typeIdx])
	}
	return false
}
