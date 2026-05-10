package wasm

// i31Ref is an `i31` reference value introduced by the WebAssembly GC
// proposal. An i31 holds a 31-bit integer, conceptually "tagged" in a
// reference slot. The spec defines exactly three operations:
//
//   - ref.i31    : i32 -> i31ref  (keeps the low 31 bits of the source)
//   - i31.get_s  : i31ref -> i32  (sign-extends bit 30 into bit 31)
//   - i31.get_u  : i31ref -> i32  (zero-extends; bit 31 is 0)
//
// In this implementation, i31Ref is a heap-allocated struct holding the
// 31-bit payload. Go's GC reclaims unreachable instances. Future
// optimisations (e.g., tagged uintptr or pointer-low-bit tagging) can be
// applied without changing callers, since the only public surface is the
// constructor and the two get_s / get_u helpers.
//
// See https://webassembly.github.io/spec/core/exec/instructions.html#xref-syntax-instructions-syntax-instr-i31-mathsf-i31-get-sx-mathit-sx
type i31Ref struct {
	// bits stores the 31-bit i31 value in the low 31 bits. Bit 31 is
	// always 0 and ignored by consumers.
	bits uint32
}

// I31RefMask is the bit-mask that ref.i31 applies to its input.
const I31RefMask uint32 = 0x7FFFFFFF

// NewI31Ref constructs an i31 reference holding the low 31 bits of v.
func NewI31Ref(v uint32) *i31Ref {
	return &i31Ref{bits: v & I31RefMask}
}

// I31RefFromInt32 is the typed sibling of NewI31Ref for callers that hold
// a Go int32 (e.g., the operand-stack pop helper in the interpreter).
func I31RefFromInt32(v int32) *i31Ref {
	return NewI31Ref(uint32(v))
}

// SignedI32 returns the i31 value as a 32-bit signed integer, with bit 30
// of the i31 sign-extended into the upper bit. Implements i31.get_s.
func (r *i31Ref) SignedI32() int32 {
	b := r.bits
	if b&0x40000000 != 0 {
		// Bit 30 set => negative; set bit 31 to sign-extend.
		return int32(b | 0x80000000)
	}
	return int32(b)
}

// UnsignedI32 returns the i31 value zero-extended to 32 bits. The result
// is always in [0, 2^31 - 1]. Implements i31.get_u.
func (r *i31Ref) UnsignedI32() uint32 {
	return r.bits
}

// Equals reports whether two i31 references hold the same payload. This is
// the semantic equality used by ref.eq when both operands are i31 refs.
//
// Per the spec, ref.eq on two i31 refs returns 1 iff their numeric values
// are equal — independent of whether they are the same heap pointer. So
// we compare payload bits, not pointer identity.
func (r *i31Ref) Equals(other *i31Ref) bool {
	if r == nil || other == nil {
		return r == other
	}
	return r.bits == other.bits
}
