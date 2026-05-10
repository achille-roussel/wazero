package wasm

import "fmt"

// HeapTypeKind discriminates the conceptual kind of a WebAssembly heap type.
//
// In Wasm 1.0 / 2.0, heap types were limited to the abstract types `func`
// and `extern`. The WebAssembly GC proposal (part of Wasm 3.0) introduces
// the abstract types `any`, `eq`, `i31`, `struct`, `array`, `none`, `nofunc`,
// `noextern`, `exn`, `noexn` and concrete type indices that refer to a
// composite type defined in the module's type section.
//
// HeapTypeKind is the conceptual identity, decoupled from any particular
// binary encoding. The value-type byte constants (ValueTypeFuncref etc.)
// remain unchanged and continue to encode "nullable abstract-heap-type
// shorthand" value types. For richer reference types (non-nullable abstract
// references or references to concrete types), GC-aware code paths track
// the heap-type kind alongside the byte tag via ValueTypeRef.
type HeapTypeKind uint8

const (
	// HeapTypeKindUnknown is the zero value, used as "unset" / "not a heap type".
	HeapTypeKindUnknown HeapTypeKind = iota
	HeapTypeKindNoFunc
	HeapTypeKindNoExtern
	HeapTypeKindBottom // spec "none" — bottom of the any hierarchy.
	HeapTypeKindFunc
	HeapTypeKindExtern
	HeapTypeKindAny
	HeapTypeKindEq
	HeapTypeKindI31
	HeapTypeKindStruct
	HeapTypeKindArray
	HeapTypeKindExn
	HeapTypeKindNoExn
	// HeapTypeKindConcrete refers to a concrete type index (an entry in the
	// module's type section). The actual index is carried alongside the
	// kind by ValueTypeRef.
	HeapTypeKindConcrete
)

// String returns the spec text-format name of the heap type kind.
func (k HeapTypeKind) String() string {
	switch k {
	case HeapTypeKindNoFunc:
		return "nofunc"
	case HeapTypeKindNoExtern:
		return "noextern"
	case HeapTypeKindBottom:
		return "none"
	case HeapTypeKindFunc:
		return "func"
	case HeapTypeKindExtern:
		return "extern"
	case HeapTypeKindAny:
		return "any"
	case HeapTypeKindEq:
		return "eq"
	case HeapTypeKindI31:
		return "i31"
	case HeapTypeKindStruct:
		return "struct"
	case HeapTypeKindArray:
		return "array"
	case HeapTypeKindExn:
		return "exn"
	case HeapTypeKindNoExn:
		return "noexn"
	case HeapTypeKindConcrete:
		return "concrete"
	}
	return fmt.Sprintf("<unknown heap kind %d>", k)
}

// HeapTypeKindFromBinary maps a signed s33 LEB heap-type encoding (as
// produced by the binary decoder after a 0x63/0x64 ref prefix byte) to
// (kind, typeIdx, ok). Non-negative encodings denote a concrete type index;
// negative encodings denote an abstract heap type.
//
// Heap-type byte values per the WebAssembly 3.0 binary format:
//
//	-13 nofunc, -12 noexn, -14 noextern, -15 none, -16 func, -17 extern,
//	-18 any,    -19 eq,    -20 i31,      -21 struct, -22 array, -23 exn.
func HeapTypeKindFromBinary(ht int64) (kind HeapTypeKind, typeIdx uint32, ok bool) {
	if ht >= 0 {
		return HeapTypeKindConcrete, uint32(ht), true
	}
	switch ht {
	case -13:
		return HeapTypeKindNoFunc, 0, true
	case -12:
		return HeapTypeKindNoExn, 0, true
	case -14:
		return HeapTypeKindNoExtern, 0, true
	case -15:
		return HeapTypeKindBottom, 0, true
	case -16:
		return HeapTypeKindFunc, 0, true
	case -17:
		return HeapTypeKindExtern, 0, true
	case -18:
		return HeapTypeKindAny, 0, true
	case -19:
		return HeapTypeKindEq, 0, true
	case -20:
		return HeapTypeKindI31, 0, true
	case -21:
		return HeapTypeKindStruct, 0, true
	case -22:
		return HeapTypeKindArray, 0, true
	case -23:
		return HeapTypeKindExn, 0, true
	}
	return HeapTypeKindUnknown, 0, false
}

// HeapTypeKindFromAbstractByte maps a single-byte abstract heap-type
// encoding (used as a "shorthand" value type) to its HeapTypeKind. Returns
// (HeapTypeKindUnknown, false) if the byte does not name an abstract heap
// type.
//
// The byte values are the sign-extension of the negative s33 encodings,
// e.g. 0x70 for func (= sign-extension of -16).
func HeapTypeKindFromAbstractByte(b byte) (HeapTypeKind, bool) {
	switch b {
	case 0x73:
		return HeapTypeKindNoFunc, true
	case 0x72:
		return HeapTypeKindNoExtern, true
	case 0x71:
		return HeapTypeKindBottom, true
	case 0x70:
		return HeapTypeKindFunc, true
	case 0x6F:
		return HeapTypeKindExtern, true
	case 0x6E:
		return HeapTypeKindAny, true
	case 0x6D:
		return HeapTypeKindEq, true
	case 0x6C:
		return HeapTypeKindI31, true
	case 0x6B:
		return HeapTypeKindStruct, true
	case 0x6A:
		return HeapTypeKindArray, true
	case 0x69:
		return HeapTypeKindExn, true
	case 0x74:
		return HeapTypeKindNoExn, true
	}
	return HeapTypeKindUnknown, false
}

// AbstractShorthandByte returns the single-byte spec encoding for the
// nullable shorthand of an abstract heap-type kind (e.g. 0x70 for func,
// meaning `funcref` = `(ref null func)`). Returns (0, false) for concrete
// kinds, which have no single-byte shorthand.
func (k HeapTypeKind) AbstractShorthandByte() (byte, bool) {
	switch k {
	case HeapTypeKindNoFunc:
		return 0x73, true
	case HeapTypeKindNoExtern:
		return 0x72, true
	case HeapTypeKindBottom:
		return 0x71, true
	case HeapTypeKindFunc:
		return 0x70, true
	case HeapTypeKindExtern:
		return 0x6F, true
	case HeapTypeKindAny:
		return 0x6E, true
	case HeapTypeKindEq:
		return 0x6D, true
	case HeapTypeKindI31:
		return 0x6C, true
	case HeapTypeKindStruct:
		return 0x6B, true
	case HeapTypeKindArray:
		return 0x6A, true
	case HeapTypeKindExn:
		return 0x69, true
	case HeapTypeKindNoExn:
		return 0x74, true
	}
	return 0, false
}

// ValueTypeRef carries the rich type information for a WebAssembly reference
// type that does not fit into a single byte: non-nullable abstract references
// `(ref ht)` and references to concrete type indices `(ref null $t)` /
// `(ref $t)`.
//
// In Wasm 1.0 / 2.0 every reference value type fit in a single byte (the
// nullable abstract shorthand). With the GC proposal, this is no longer
// true. Rather than widen the byte-sized ValueType, wazero keeps ValueType
// as a byte and carries ValueTypeRef on the side for the GC-only cases.
//
// Phase 1: ValueTypeRef and HeapTypeKind are defined; later phases plumb
// them through FunctionType, locals, globals, table types, and the validator.
type ValueTypeRef struct {
	// Nullable is true for `(ref null ht)`, false for `(ref ht)`.
	Nullable bool
	// HeapKind is the conceptual kind of heap type referred to.
	HeapKind HeapTypeKind
	// TypeIdx is the concrete type-section index referenced. Meaningful only
	// when HeapKind == HeapTypeKindConcrete; ignored otherwise.
	TypeIdx uint32
}

// IsAbstract reports whether the referenced heap type is one of the
// abstract heap types (not a concrete type-section index).
func (r ValueTypeRef) IsAbstract() bool {
	return r.HeapKind != HeapTypeKindConcrete && r.HeapKind != HeapTypeKindUnknown
}

// String renders the ref type as spec text format, e.g. `(ref null func)`
// or `(ref 7)` for a non-nullable concrete reference to type index 7.
func (r ValueTypeRef) String() string {
	prefix := "(ref "
	if r.Nullable {
		prefix = "(ref null "
	}
	if r.HeapKind == HeapTypeKindConcrete {
		return fmt.Sprintf("%s%d)", prefix, r.TypeIdx)
	}
	return prefix + r.HeapKind.String() + ")"
}

// ShorthandByte returns the single-byte value-type shorthand for r when it
// has one (a nullable reference to an abstract heap type). Returns
// (0, false) for non-nullable or concrete references, which require the
// multi-byte (0x63/0x64 + heaptype) encoding.
func (r ValueTypeRef) ShorthandByte() (byte, bool) {
	if !r.Nullable {
		return 0, false
	}
	return r.HeapKind.AbstractShorthandByte()
}

// hierarchyTops returns the set of "top" abstract heap-type kinds —
// supertypes a kind eventually reaches. The three Wasm 3.0 hierarchies are
// disjoint: each abstract kind belongs to exactly one of them.
//
//	any:     any > eq > {i31, struct, array} > none
//	func:    func > nofunc                            (plus concrete func types)
//	extern:  extern > noextern
//	exn:     exn > noexn
//
// HeapTypeKindConcrete returns HeapTypeKindUnknown because we cannot tell
// without module context whether the concrete type belongs to the any
// hierarchy (struct/array) or the func hierarchy.
func (k HeapTypeKind) hierarchyTop() HeapTypeKind {
	switch k {
	case HeapTypeKindNoFunc, HeapTypeKindFunc:
		return HeapTypeKindFunc
	case HeapTypeKindNoExtern, HeapTypeKindExtern:
		return HeapTypeKindExtern
	case HeapTypeKindNoExn, HeapTypeKindExn:
		return HeapTypeKindExn
	case HeapTypeKindBottom, HeapTypeKindI31, HeapTypeKindStruct,
		HeapTypeKindArray, HeapTypeKindEq, HeapTypeKindAny:
		return HeapTypeKindAny
	}
	return HeapTypeKindUnknown
}

// IsAbstractSubtypeOf reports whether actual is a subtype of expected,
// considering only the static abstract hierarchy of the WebAssembly GC
// spec. Both arguments must be abstract kinds — pairs involving
// HeapTypeKindConcrete return false because their subtyping requires
// module-level type-section context (declared supertypes via 0x4F / 0x50,
// concrete-form-determined hierarchy membership, etc.). The Phase 4
// validator threads module context through a separate check.
//
// Subtype relationships per the spec:
//   - reflexive: every kind is a subtype of itself.
//   - bottom kinds (none, nofunc, noextern, noexn) are subtypes of every
//     other kind in their respective hierarchy.
//   - top kinds (any, func, extern, exn) are supertypes of every other
//     kind in their respective hierarchy.
//   - within the any hierarchy: i31, struct, array each <: eq, and eq <: any.
//   - the four hierarchies are disjoint: no cross-hierarchy subtyping.
//
// Sources:
//   - https://webassembly.github.io/spec/core/syntax/types.html
//   - https://webassembly.github.io/spec/core/valid/types.html
func (actual HeapTypeKind) IsAbstractSubtypeOf(expected HeapTypeKind) bool {
	if actual == HeapTypeKindUnknown || expected == HeapTypeKindUnknown {
		return false
	}
	if actual == HeapTypeKindConcrete || expected == HeapTypeKindConcrete {
		return false
	}
	if actual == expected {
		return true
	}

	actualTop := actual.hierarchyTop()
	if actualTop != expected.hierarchyTop() {
		return false
	}

	// Bottom of each hierarchy is a subtype of everything in that hierarchy.
	switch actual {
	case HeapTypeKindBottom, HeapTypeKindNoFunc, HeapTypeKindNoExtern, HeapTypeKindNoExn:
		return true
	}

	// In the func / extern / exn hierarchies the only non-bottom abstract
	// types are the tops themselves; if actual is the top it's not a
	// strict subtype of anything else in the hierarchy.
	if actualTop != HeapTypeKindAny {
		return false
	}

	// Within the any hierarchy.
	if expected == HeapTypeKindAny {
		// any is the top of the hierarchy; everything below is a subtype.
		return true
	}
	if actual == HeapTypeKindAny {
		return false
	}
	if expected == HeapTypeKindEq {
		// eq's strict subtypes among abstract kinds are i31, struct, array.
		// (HeapTypeKindBottom is handled above.)
		return actual == HeapTypeKindI31 || actual == HeapTypeKindStruct || actual == HeapTypeKindArray
	}
	// Remaining pairs among {i31, struct, array, eq} where actual != expected
	// are unrelated: i31 / struct / array are sibling subtypes of eq.
	return false
}
