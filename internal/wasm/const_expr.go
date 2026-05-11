package wasm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/leb128"
)

type ConstantExpression struct {
	Data []byte
}

// gcConstExprCtx provides the type, allocation, and data/element-segment
// environment for evaluating wasm-gc operations in constant expressions
// (e.g. global initializers).
type gcConstExprCtx struct {
	// Types is the module's type section (post-decode).
	Types []FunctionType
	// TypeIDs is the canonical FunctionTypeID for each type-section entry.
	TypeIDs []FunctionTypeID
	// DataInstances and ElementInstances back array.new_data / array.new_elem
	// resolution. Either may be nil when the corresponding feature isn't used.
	DataInstances    []DataInstance
	ElementInstances []ElementInstance
	// KeepAlive appends a ref to a roots slice so Go's GC keeps it alive
	// past the unsafe.Pointer→uintptr cast used to fit the ref into the
	// uint64 stack slot.
	KeepAlive func(any)
	// ValidateOnly skips actual allocation and pushes a sentinel 0 ref;
	// used by validation passes that scan const-exprs for ref.func
	// indices but don't need real heap allocations.
	ValidateOnly bool
	// FuncTypes maps each declared function index (post-import) to
	// its declared type index, so ref.func can push the precise
	// concrete-ref rich info ((Nullable=false, Concrete, TypeIdx))
	// rather than a bare funcref byte.
	FuncTypes []Index
}

// refToUint64 casts a GC ref pointer into the uint64 stack slot via
// unsafe.Pointer. The caller is responsible for keep-alive.
func refToUint64(v any) uint64 {
	switch r := v.(type) {
	case nil:
		return 0
	case *WasmStruct:
		return uint64(uintptr(unsafe.Pointer(r)))
	case *WasmArray:
		return uint64(uintptr(unsafe.Pointer(r)))
	}
	return 0
}

// boxFieldFromStack converts a raw uint64 stack value into the typed Go
// value that lives inside a WasmStruct.Fields / WasmArray.Elements slot.
// Packed i8 / i16 fields are narrowed to uint8 / uint16; numeric scalars
// are reinterpreted via api.Decode helpers; refs pass through as uintptr.
func boxFieldFromStack(f *FieldType, raw uint64) any {
	switch f.Packed {
	case PackedTypeI8:
		return uint8(raw & 0xFF)
	case PackedTypeI16:
		return uint16(raw & 0xFFFF)
	}
	switch f.ValueType {
	case ValueTypeI32:
		return int32(uint32(raw))
	case ValueTypeI64:
		return int64(raw)
	case ValueTypeF32:
		return float32frombits(uint32(raw))
	case ValueTypeF64:
		return float64frombits(raw)
	}
	// References (any/struct/array/funcref/externref/concrete/...) are
	// stored as the raw uintptr already living in the stack slot. The
	// interpreter's ref-aware code unboxes via IsTaggedI31 and type
	// assertions.
	return uintptr(raw)
}

func float32frombits(b uint32) float32 {
	return *(*float32)(unsafe.Pointer(&b))
}

func float64frombits(b uint64) float64 {
	return *(*float64)(unsafe.Pointer(&b))
}

// padRefs grows refs (or trims it) to exactly `want` entries, padding
// any growth with nil. Used by the const-expr evaluator to keep the
// rich-info sidecar aligned with typeStack.
func padRefs(refs []*ValueTypeRef, want int) []*ValueTypeRef {
	if len(refs) > want {
		return refs[:want]
	}
	for len(refs) < want {
		refs = append(refs, nil)
	}
	return refs
}

// pushConcreteRefAt grows refs so refs[idx] is the rich info for a
// concrete (non-nullable) ref to the given type index.
func pushConcreteRefAt(refs []*ValueTypeRef, idx int, typeIdx uint32) []*ValueTypeRef {
	refs = padRefs(refs, idx)
	return append(refs, &ValueTypeRef{
		Nullable: false,
		HeapKind: HeapTypeKindConcrete,
		TypeIdx:  typeIdx,
	})
}

func evaluateConstExpr(e *ConstantExpression, globalResolver func(globalIndex Index) (ValueType, uint64, uint64, error), funcRefResolver func(funcIndex Index) (Reference, error), gcCtx *gcConstExprCtx) ([]uint64, ValueType, error) {
	results, typ, _, err := evaluateConstExprRich(e, globalResolver, funcRefResolver, gcCtx)
	return results, typ, err
}

// evaluateConstExprRich is evaluateConstExpr with an extra rich-info
// return so validators can check refs precisely (nullable / concrete
// TypeIdx). The rich info is non-nil only when the top of the type
// stack at OpcodeEnd carries it (e.g. ref.func on a typed function,
// struct.new / array.new producing concrete refs).
func evaluateConstExprRich(e *ConstantExpression, globalResolver func(globalIndex Index) (ValueType, uint64, uint64, error), funcRefResolver func(funcIndex Index) (Reference, error), gcCtx *gcConstExprCtx) ([]uint64, ValueType, *ValueTypeRef, error) {
	var stack []uint64
	var typeStack []ValueType
	// typeRefs is the parallel rich-info slice for typeStack — non-nil
	// entries describe the precise reference type at that position.
	// Maintained lazily: most non-ref pushes leave it short.
	var typeRefs []*ValueTypeRef
	var pc uint64
	data := e.Data
	for {
		if pc >= uint64(len(data)) {
			return nil, 0, nil, io.ErrUnexpectedEOF
		}
		opCode := data[pc]
		pc++
		switch opCode {
		case OpcodeI32Const:
			v, n, err := leb128.LoadInt32(data[pc:])
			if err != nil {
				return nil, 0, nil, fmt.Errorf("read i32: %w", err)
			}
			pc += n
			stack = append(stack, uint64(uint32(v)))
			typeStack = append(typeStack, ValueTypeI32)
		case OpcodeI64Const:
			v, n, err := leb128.LoadInt64(data[pc:])
			if err != nil {
				return nil, 0, nil, fmt.Errorf("read i64: %w", err)
			}
			pc += n
			stack = append(stack, uint64(v))
			typeStack = append(typeStack, ValueTypeI64)
		case OpcodeF32Const:
			if len(data[pc:]) < 4 {
				return nil, 0, nil, io.ErrUnexpectedEOF
			}
			v := binary.LittleEndian.Uint32(data[pc:])
			pc += 4
			stack = append(stack, uint64(v))
			typeStack = append(typeStack, ValueTypeF32)
		case OpcodeF64Const:
			if len(data[pc:]) < 8 {
				return nil, 0, nil, io.ErrUnexpectedEOF
			}
			v := binary.LittleEndian.Uint64(data[pc:])
			pc += 8
			stack = append(stack, uint64(v))
			typeStack = append(typeStack, ValueTypeF64)
		case OpcodeGlobalGet:
			v, n, err := leb128.LoadUint32(data[pc:])
			if err != nil {
				return nil, 0, nil, fmt.Errorf("read index of global: %w", err)
			}
			pc += n
			typ, lo, hi, err := globalResolver(Index(v))
			if err != nil {
				return nil, 0, nil, err
			}
			switch typ {
			case ValueTypeV128:
				stack = append(stack, lo, hi)
			default:
				stack = append(stack, lo)
			}
			typeStack = append(typeStack, typ)
		case OpcodeRefNull:
			// Reference types are opaque 64bit pointer at runtime.
			if pc >= uint64(len(data)) {
				return nil, 0, nil, fmt.Errorf("read reference type for ref.null: %w", io.ErrShortBuffer)
			}
			// With wasm-gc enabled, the heap type is encoded as s33;
			// support both the legacy byte-shorthand path and the wider
			// abstract / concrete heap-type encoding.
			if gcCtx != nil {
				br := bytes.NewReader(data[pc:])
				ht, n, hterr := leb128.DecodeInt33AsInt64(br)
				if hterr != nil {
					return nil, 0, nil, fmt.Errorf("read ref.null heap type: %w", hterr)
				}
				kind, typeIdx, ok := HeapTypeKindFromBinary(ht)
				if !ok {
					return nil, 0, nil, fmt.Errorf("invalid heap type for ref.null: %d", ht)
				}
				pc += n
				stack = append(stack, 0)
				var byteType ValueType
				switch kind {
				case HeapTypeKindFunc, HeapTypeKindNoFunc:
					byteType = ValueTypeFuncref
				case HeapTypeKindExtern, HeapTypeKindNoExtern:
					byteType = ValueTypeExternref
				case HeapTypeKindExn, HeapTypeKindNoExn:
					byteType = ValueTypeExnref
				case HeapTypeKindConcrete:
					if int(typeIdx) < len(gcCtx.FuncTypes) {
						byteType = ValueTypeFuncref
					} else {
						byteType = ValueTypeAnyref
					}
				default:
					byteType = ValueTypeAnyref
				}
				typeStack = append(typeStack, byteType)
				if kind == HeapTypeKindConcrete {
					typeRefs = padRefs(typeRefs, len(typeStack)-1)
					typeRefs = append(typeRefs, &ValueTypeRef{
						Nullable: true,
						HeapKind: HeapTypeKindConcrete,
						TypeIdx:  typeIdx,
					})
				} else if kind != HeapTypeKindFunc && kind != HeapTypeKindExtern {
					typeRefs = padRefs(typeRefs, len(typeStack)-1)
					typeRefs = append(typeRefs, &ValueTypeRef{
						Nullable: true,
						HeapKind: kind,
					})
				}
			} else {
				valType := ValueType(data[pc])
				if valType != RefTypeFuncref && valType != RefTypeExternref {
					return nil, 0, nil, fmt.Errorf("invalid type for ref.null: 0x%x", valType)
				}
				pc += 1
				stack = append(stack, 0)
				typeStack = append(typeStack, valType)
			}
		case OpcodeRefFunc:
			v, n, err := leb128.LoadUint32(data[pc:])
			if err != nil {
				return nil, 0, nil, fmt.Errorf("read i32: %w", err)
			}
			pc += n
			ref, err := funcRefResolver(Index(v))
			if err != nil {
				return nil, 0, nil, err
			}
			stack = append(stack, uint64(ref))
			typeStack = append(typeStack, ValueTypeFuncref)
			// Push rich info for the concrete function type so a
			// subsequent subtype check against (ref $T) can use
			// the precise TypeIdx (instead of just matching the
			// funcref sentinel byte).
			if gcCtx != nil && int(v) < len(gcCtx.FuncTypes) {
				typeIdx := gcCtx.FuncTypes[v]
				typeRefs = padRefs(typeRefs, len(typeStack)-1)
				typeRefs = append(typeRefs, &ValueTypeRef{
					Nullable: false,
					HeapKind: HeapTypeKindConcrete,
					TypeIdx:  typeIdx,
				})
			}
		case OpcodeVecPrefix:
			if data[pc] != OpcodeVecV128Const {
				return nil, 0, nil, fmt.Errorf("invalid vector opcode for const expression: %#x", data[pc-1])
			}
			pc++
			if len(data[pc:]) < 16 {
				return nil, 0, nil, fmt.Errorf("%s needs 16 bytes but was %d bytes", OpcodeVecV128ConstName, len(data[pc:]))
			}
			lo := binary.LittleEndian.Uint64(data[pc:])
			pc += 8
			hi := binary.LittleEndian.Uint64(data[pc:])
			pc += 8
			stack = append(stack, lo, hi)
			typeStack = append(typeStack, ValueTypeV128)
		case OpcodeI32Add:
			if len(typeStack) < 2 {
				return nil, 0, nil, errors.New("stack underflow on i32.add")
			}
			v1 := typeStack[len(typeStack)-1]
			v2 := typeStack[len(typeStack)-2]
			if v1 != ValueTypeI32 || v2 != ValueTypeI32 {
				return nil, 0, nil, fmt.Errorf("type mismatch on i32.add: %s, %s", ValueTypeName(v2), ValueTypeName(v1))
			}
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, uint64(uint32(a)+uint32(b)))
			typeStack = typeStack[:len(typeStack)-2]
			typeStack = append(typeStack, ValueTypeI32)
		case OpcodeI32Sub:
			if len(typeStack) < 2 {
				return nil, 0, nil, errors.New("stack underflow on i32.sub")
			}
			v1 := typeStack[len(typeStack)-1]
			v2 := typeStack[len(typeStack)-2]
			if v1 != ValueTypeI32 || v2 != ValueTypeI32 {
				return nil, 0, nil, fmt.Errorf("type mismatch on i32.sub: %s, %s", ValueTypeName(v2), ValueTypeName(v1))
			}
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, uint64(uint32(a)-uint32(b)))
			typeStack = typeStack[:len(typeStack)-2]
			typeStack = append(typeStack, ValueTypeI32)
		case OpcodeI32Mul:
			if len(typeStack) < 2 {
				return nil, 0, nil, errors.New("stack underflow on i32.mul")
			}
			v1 := typeStack[len(typeStack)-1]
			v2 := typeStack[len(typeStack)-2]
			if v1 != ValueTypeI32 || v2 != ValueTypeI32 {
				return nil, 0, nil, fmt.Errorf("type mismatch on i32.mul: %s, %s", ValueTypeName(v2), ValueTypeName(v1))
			}
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, uint64(uint32(a)*uint32(b)))
			typeStack = typeStack[:len(typeStack)-2]
			typeStack = append(typeStack, ValueTypeI32)
		case OpcodeI64Add:
			if len(typeStack) < 2 {
				return nil, 0, nil, errors.New("stack underflow on i64.add")
			}
			v1 := typeStack[len(typeStack)-1]
			v2 := typeStack[len(typeStack)-2]
			if v1 != ValueTypeI64 || v2 != ValueTypeI64 {
				return nil, 0, nil, fmt.Errorf("type mismatch on i64.add: %s, %s", ValueTypeName(v2), ValueTypeName(v1))
			}
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, a+b)
			typeStack = typeStack[:len(typeStack)-2]
			typeStack = append(typeStack, ValueTypeI64)
		case OpcodeI64Sub:
			if len(typeStack) < 2 {
				return nil, 0, nil, errors.New("stack underflow on i64.sub")
			}
			v1 := typeStack[len(typeStack)-1]
			v2 := typeStack[len(typeStack)-2]
			if v1 != ValueTypeI64 || v2 != ValueTypeI64 {
				return nil, 0, nil, fmt.Errorf("type mismatch on i64.sub: %s, %s", ValueTypeName(v2), ValueTypeName(v1))
			}
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, a-b)
			typeStack = typeStack[:len(typeStack)-2]
			typeStack = append(typeStack, ValueTypeI64)
		case OpcodeI64Mul:
			if len(typeStack) < 2 {
				return nil, 0, nil, errors.New("stack underflow on i64.mul")
			}
			v1 := typeStack[len(typeStack)-1]
			v2 := typeStack[len(typeStack)-2]
			if v1 != ValueTypeI64 || v2 != ValueTypeI64 {
				return nil, 0, nil, fmt.Errorf("type mismatch on i64.mul: %s, %s", ValueTypeName(v2), ValueTypeName(v1))
			}
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, a*b)
			typeStack = typeStack[:len(typeStack)-2]
			typeStack = append(typeStack, ValueTypeI64)
		case OpcodeGCPrefix:
			if gcCtx == nil {
				return nil, 0, nil, fmt.Errorf("GC const expression requires wasm-gc context")
			}
			sub, n, suberr := leb128.LoadUint32(data[pc:])
			if suberr != nil {
				return nil, 0, nil, fmt.Errorf("read GC sub-opcode: %w", suberr)
			}
			pc += n
			switch OpcodeGC(sub) {
			case OpcodeGCStructNew:
				tIdx, m, err := leb128.LoadUint32(data[pc:])
				if err != nil {
					return nil, 0, nil, fmt.Errorf("struct.new typeidx: %w", err)
				}
				pc += m
				if int(tIdx) >= len(gcCtx.Types) {
					return nil, 0, nil, fmt.Errorf("struct.new: type index %d out of range", tIdx)
				}
				ft := &gcCtx.Types[tIdx]
				numFields := len(ft.Fields)
				if len(stack) < numFields {
					return nil, 0, nil, fmt.Errorf("struct.new: stack underflow (want %d, have %d)", numFields, len(stack))
				}
				if gcCtx.ValidateOnly {
					stack = stack[:len(stack)-numFields]
					typeStack = typeStack[:len(typeStack)-numFields]
					stack = append(stack, 0)
					typeStack = append(typeStack, RefTypeFuncref)
					typeRefs = pushConcreteRefAt(typeRefs, len(typeStack)-1, tIdx)
					continue
				}
				fields := make([]any, numFields)
				for i := 0; i < numFields; i++ {
					raw := stack[len(stack)-numFields+i]
					fields[i] = boxFieldFromStack(&ft.Fields[i], raw)
				}
				stack = stack[:len(stack)-numFields]
				typeStack = typeStack[:len(typeStack)-numFields]
				ws := NewWasmStructWith(gcCtx.TypeIDs[tIdx], fields)
				gcCtx.KeepAlive(ws)
				stack = append(stack, refToUint64(ws))
				typeStack = append(typeStack, RefTypeFuncref)
				typeRefs = pushConcreteRefAt(typeRefs, len(typeStack)-1, tIdx)
			case OpcodeGCStructNewDefault:
				tIdx, m, err := leb128.LoadUint32(data[pc:])
				if err != nil {
					return nil, 0, nil, fmt.Errorf("struct.new_default typeidx: %w", err)
				}
				pc += m
				if int(tIdx) >= len(gcCtx.Types) {
					return nil, 0, nil, fmt.Errorf("struct.new_default: type index %d out of range", tIdx)
				}
				if gcCtx.ValidateOnly {
					stack = append(stack, 0)
					typeStack = append(typeStack, RefTypeFuncref)
					typeRefs = pushConcreteRefAt(typeRefs, len(typeStack)-1, tIdx)
					continue
				}
				ft := &gcCtx.Types[tIdx]
				numFields := len(ft.Fields)
				fields := make([]any, numFields)
				for i := 0; i < numFields; i++ {
					fields[i] = DefaultFieldValue(ft.Fields[i])
				}
				ws := NewWasmStructWith(gcCtx.TypeIDs[tIdx], fields)
				gcCtx.KeepAlive(ws)
				stack = append(stack, refToUint64(ws))
				typeStack = append(typeStack, RefTypeFuncref)
				typeRefs = pushConcreteRefAt(typeRefs, len(typeStack)-1, tIdx)
			case OpcodeGCArrayNew:
				tIdx, m, err := leb128.LoadUint32(data[pc:])
				if err != nil {
					return nil, 0, nil, fmt.Errorf("array.new typeidx: %w", err)
				}
				pc += m
				if int(tIdx) >= len(gcCtx.Types) {
					return nil, 0, nil, fmt.Errorf("array.new: type index %d out of range", tIdx)
				}
				if len(stack) < 2 {
					return nil, 0, nil, fmt.Errorf("array.new: stack underflow")
				}
				if gcCtx.ValidateOnly {
					stack = stack[:len(stack)-2]
					typeStack = typeStack[:len(typeStack)-2]
					stack = append(stack, 0)
					typeStack = append(typeStack, RefTypeFuncref)
					typeRefs = pushConcreteRefAt(typeRefs, len(typeStack)-1, tIdx)
					continue
				}
				ft := &gcCtx.Types[tIdx]
				// Stack: [..., value, length]
				lengthRaw := stack[len(stack)-1]
				valueRaw := stack[len(stack)-2]
				stack = stack[:len(stack)-2]
				typeStack = typeStack[:len(typeStack)-2]
				length := uint32(lengthRaw)
				boxed := boxFieldFromStack(&ft.ArrayField, valueRaw)
				elems := make([]any, length)
				for i := uint32(0); i < length; i++ {
					elems[i] = boxed
				}
				wa := NewWasmArrayWith(gcCtx.TypeIDs[tIdx], elems)
				gcCtx.KeepAlive(wa)
				stack = append(stack, refToUint64(wa))
				typeStack = append(typeStack, RefTypeFuncref)
				typeRefs = pushConcreteRefAt(typeRefs, len(typeStack)-1, tIdx)
			case OpcodeGCArrayNewDefault:
				tIdx, m, err := leb128.LoadUint32(data[pc:])
				if err != nil {
					return nil, 0, nil, fmt.Errorf("array.new_default typeidx: %w", err)
				}
				pc += m
				if int(tIdx) >= len(gcCtx.Types) {
					return nil, 0, nil, fmt.Errorf("array.new_default: type index %d out of range", tIdx)
				}
				if len(stack) < 1 {
					return nil, 0, nil, fmt.Errorf("array.new_default: stack underflow")
				}
				if gcCtx.ValidateOnly {
					stack = stack[:len(stack)-1]
					typeStack = typeStack[:len(typeStack)-1]
					stack = append(stack, 0)
					typeStack = append(typeStack, RefTypeFuncref)
					typeRefs = pushConcreteRefAt(typeRefs, len(typeStack)-1, tIdx)
					continue
				}
				ft := &gcCtx.Types[tIdx]
				length := uint32(stack[len(stack)-1])
				stack = stack[:len(stack)-1]
				typeStack = typeStack[:len(typeStack)-1]
				def := DefaultFieldValue(ft.ArrayField)
				elems := make([]any, length)
				for i := uint32(0); i < length; i++ {
					elems[i] = def
				}
				wa := NewWasmArrayWith(gcCtx.TypeIDs[tIdx], elems)
				gcCtx.KeepAlive(wa)
				stack = append(stack, refToUint64(wa))
				typeStack = append(typeStack, RefTypeFuncref)
				typeRefs = pushConcreteRefAt(typeRefs, len(typeStack)-1, tIdx)
			case OpcodeGCArrayNewFixed:
				tIdx, m, err := leb128.LoadUint32(data[pc:])
				if err != nil {
					return nil, 0, nil, fmt.Errorf("array.new_fixed typeidx: %w", err)
				}
				pc += m
				length, n2, err := leb128.LoadUint32(data[pc:])
				if err != nil {
					return nil, 0, nil, fmt.Errorf("array.new_fixed length: %w", err)
				}
				pc += n2
				if int(tIdx) >= len(gcCtx.Types) {
					return nil, 0, nil, fmt.Errorf("array.new_fixed: type index %d out of range", tIdx)
				}
				if uint32(len(stack)) < length {
					return nil, 0, nil, fmt.Errorf("array.new_fixed: stack underflow")
				}
				if gcCtx.ValidateOnly {
					stack = stack[:uint32(len(stack))-length]
					typeStack = typeStack[:uint32(len(typeStack))-length]
					stack = append(stack, 0)
					typeStack = append(typeStack, RefTypeFuncref)
					typeRefs = pushConcreteRefAt(typeRefs, len(typeStack)-1, tIdx)
					continue
				}
				ft := &gcCtx.Types[tIdx]
				elems := make([]any, length)
				for i := uint32(0); i < length; i++ {
					raw := stack[uint32(len(stack))-length+i]
					elems[i] = boxFieldFromStack(&ft.ArrayField, raw)
				}
				stack = stack[:uint32(len(stack))-length]
				typeStack = typeStack[:uint32(len(typeStack))-length]
				wa := NewWasmArrayWith(gcCtx.TypeIDs[tIdx], elems)
				gcCtx.KeepAlive(wa)
				stack = append(stack, refToUint64(wa))
				typeStack = append(typeStack, RefTypeFuncref)
				typeRefs = pushConcreteRefAt(typeRefs, len(typeStack)-1, tIdx)
			case OpcodeGCRefI31:
				if len(stack) < 1 {
					return nil, 0, nil, fmt.Errorf("ref.i31: stack underflow")
				}
				v := uint32(stack[len(stack)-1])
				stack = stack[:len(stack)-1]
				typeStack = typeStack[:len(typeStack)-1]
				stack = append(stack, uint64(PackI31(v)))
				// ref.i31 produces (ref i31): non-null abstract i31.
				typeStack = append(typeStack, ValueTypeI31ref)
				typeRefs = padRefs(typeRefs, len(typeStack)-1)
				typeRefs = append(typeRefs, &ValueTypeRef{
					Nullable: false,
					HeapKind: HeapTypeKindI31,
				})
			case OpcodeGCAnyConvertExtern:
				// extern→any: the result is anyref. Keep the same value
				// on the stack; just update the byte tag.
				if len(typeStack) >= 1 {
					typeStack[len(typeStack)-1] = ValueTypeAnyref
				}
			case OpcodeGCExternConvertAny:
				// any→extern: the result is externref.
				if len(typeStack) >= 1 {
					typeStack[len(typeStack)-1] = ValueTypeExternref
				}
			default:
				return nil, 0, nil, fmt.Errorf("invalid GC sub-opcode for const expression: 0x%x", sub)
			}
		case OpcodeEnd:
			if len(typeStack) != 1 {
				return nil, 0, nil, errors.New("stack has more than one value at end of constant expression")
			}
			var topRef *ValueTypeRef
			if len(typeRefs) > 0 {
				topRef = typeRefs[len(typeRefs)-1]
			}
			return stack, typeStack[0], topRef, nil
		default:
			return nil, 0, nil, fmt.Errorf("invalid opcode for const expression: 0x%x", opCode)
		}
	}
}

func evaluateConstExprInModuleInstance(e *ConstantExpression, m *ModuleInstance) []uint64 {
	var gcCtx *gcConstExprCtx
	if src := m.Source; src != nil && len(src.TypeSection) > 0 {
		// Build a GC context lazily — it is only consulted when the
		// expression contains GC opcodes.
		gcCtx = &gcConstExprCtx{
			Types:            src.TypeSection,
			TypeIDs:          m.TypeIDs,
			DataInstances:    m.DataInstances,
			ElementInstances: m.ElementInstances,
			KeepAlive:        func(v any) { m.GCRoots = append(m.GCRoots, v) },
		}
	}
	v, _, _ := evaluateConstExpr(
		e,
		func(globalIndex Index) (ValueType, uint64, uint64, error) {
			g := m.Globals[globalIndex]
			return g.Type.ValType, g.Val, g.ValHi, nil
		},
		func(funcIndex Index) (Reference, error) {
			return m.Engine.FunctionInstanceReference(funcIndex), nil
		},
		gcCtx,
	)
	return v
}

func NewConstantExpressionFromOpcode(
	opcode byte, opData []byte,
) ConstantExpression {
	data := make([]byte, 0, 3+len(opData)) // 2 for opcode and optional vec prefix, 1 for end
	if opcode == OpcodeVecV128Const {
		data = append(data, OpcodeVecPrefix)
	}
	data = append(data, opcode)
	data = append(data, opData...)
	data = append(data, OpcodeEnd)
	return ConstantExpression{Data: data}
}

func NewConstantExpressionFromI32(val int32) ConstantExpression {
	return NewConstantExpressionFromOpcode(OpcodeI32Const, leb128.EncodeInt32(val))
}

func NewConstantExpressionFromI64(val int64) ConstantExpression {
	return NewConstantExpressionFromOpcode(OpcodeI64Const, leb128.EncodeInt64(val))
}
