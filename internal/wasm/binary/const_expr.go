package binary

import (
	"bytes"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/ieee754"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func decodeConstantExpression(r *bytes.Reader, enabledFeatures api.CoreFeatures, ret *wasm.ConstantExpression) error {
	lenAtStart := r.Len()
	startPos := r.Size() - int64(lenAtStart)
	for {
		opcode, err := r.ReadByte()
		if err != nil {
			return fmt.Errorf("read const expression opcode: %v", err)
		}
		switch opcode {
		case wasm.OpcodeI32Const:
			// Treat constants as signed as their interpretation is not yet known per /RATIONALE.md
			_, _, err = leb128.DecodeInt32(r)
		case wasm.OpcodeI32Add, wasm.OpcodeI32Sub, wasm.OpcodeI32Mul:
			// No immediate to read.
			if !enabledFeatures.IsEnabled(experimental.CoreFeaturesExtendedConst) {
				return fmt.Errorf("%v is not supported in a constant expression as feature \"extended-const\" is disabled", wasm.InstructionName(opcode))
			}
		case wasm.OpcodeI64Const:
			// Treat constants as signed as their interpretation is not yet known per /RATIONALE.md
			_, _, err = leb128.DecodeInt64(r)
		case wasm.OpcodeI64Add, wasm.OpcodeI64Sub, wasm.OpcodeI64Mul:
			// No immediate to read.
			if !enabledFeatures.IsEnabled(experimental.CoreFeaturesExtendedConst) {
				return fmt.Errorf("%v is not supported in a constant expression as feature \"extended-const\" is disabled", wasm.InstructionName(opcode))
			}
		case wasm.OpcodeF32Const:
			buf := make([]byte, 4)
			if _, err := io.ReadFull(r, buf); err != nil {
				return fmt.Errorf("read f32 constant: %v", err)
			}
			_, err = ieee754.DecodeFloat32(buf)
		case wasm.OpcodeF64Const:
			buf := make([]byte, 8)
			if _, err := io.ReadFull(r, buf); err != nil {
				return fmt.Errorf("read f64 constant: %v", err)
			}
			_, err = ieee754.DecodeFloat64(buf)
		case wasm.OpcodeGlobalGet:
			_, _, err = leb128.DecodeUint32(r)
		case wasm.OpcodeRefNull:
			if err := enabledFeatures.RequireEnabled(api.CoreFeatureBulkMemoryOperations); err != nil {
				return fmt.Errorf("ref.null is not supported as %w", err)
			}
			// With wasm-gc enabled, the immediate is an s33 heap type
			// supporting both abstract shorthand bytes and concrete
			// type indices. Without GC, fall back to the byte-only
			// funcref/externref decode for backwards compatibility.
			if enabledFeatures.IsEnabled(experimental.CoreFeaturesGC) {
				ht, _, hterr := leb128.DecodeInt33AsInt64(r)
				if hterr != nil {
					return fmt.Errorf("read ref.null heap type: %w", hterr)
				}
				if _, _, ok := wasm.HeapTypeKindFromBinary(ht); !ok {
					return fmt.Errorf("invalid heap type for ref.null: %d", ht)
				}
			} else {
				reftype, rerr := r.ReadByte()
				if rerr != nil {
					return fmt.Errorf("read reference type for ref.null: %w", rerr)
				} else if reftype != wasm.RefTypeFuncref && reftype != wasm.RefTypeExternref {
					return fmt.Errorf("invalid type for ref.null: 0x%x", reftype)
				}
			}
		case wasm.OpcodeRefFunc:
			if err := enabledFeatures.RequireEnabled(api.CoreFeatureBulkMemoryOperations); err != nil {
				return fmt.Errorf("ref.func is not supported as %w", err)
			}
			// Parsing index.
			_, _, err = leb128.DecodeUint32(r)
		case wasm.OpcodeGCPrefix:
			if err := enabledFeatures.RequireEnabled(experimental.CoreFeaturesGC); err != nil {
				return fmt.Errorf("GC instructions are not supported as %w", err)
			}
			sub, _, suberr := leb128.DecodeUint32(r)
			if suberr != nil {
				return fmt.Errorf("read GC sub-opcode for const expression: %w", suberr)
			}
			switch wasm.OpcodeGC(sub) {
			case wasm.OpcodeGCStructNew, wasm.OpcodeGCStructNewDefault,
				wasm.OpcodeGCArrayNew, wasm.OpcodeGCArrayNewDefault,
				wasm.OpcodeGCArrayNewData, wasm.OpcodeGCArrayNewElem:
				// typeidx (or for new_data/new_elem: typeidx + dataidx/elemidx)
				if _, _, err = leb128.DecodeUint32(r); err != nil {
					return fmt.Errorf("read GC typeidx immediate: %w", err)
				}
				switch wasm.OpcodeGC(sub) {
				case wasm.OpcodeGCArrayNewData, wasm.OpcodeGCArrayNewElem:
					if _, _, err = leb128.DecodeUint32(r); err != nil {
						return fmt.Errorf("read GC data/elem index: %w", err)
					}
				}
			case wasm.OpcodeGCArrayNewFixed:
				// typeidx, length
				if _, _, err = leb128.DecodeUint32(r); err != nil {
					return fmt.Errorf("read array.new_fixed typeidx: %w", err)
				}
				if _, _, err = leb128.DecodeUint32(r); err != nil {
					return fmt.Errorf("read array.new_fixed length: %w", err)
				}
			case wasm.OpcodeGCRefI31, wasm.OpcodeGCAnyConvertExtern, wasm.OpcodeGCExternConvertAny:
				// No immediates.
			default:
				return fmt.Errorf("%v for const expression GC sub-op: %#x",
					ErrInvalidByte, sub)
			}
		case wasm.OpcodeVecPrefix:
			if err := enabledFeatures.RequireEnabled(api.CoreFeatureSIMD); err != nil {
				return fmt.Errorf("vector instructions are not supported as %w", err)
			}
			opcode, err = r.ReadByte()
			if err != nil {
				return fmt.Errorf("read vector instruction opcode suffix: %w", err)
			}

			if opcode != wasm.OpcodeVecV128Const {
				return fmt.Errorf("invalid vector opcode for const expression: %#x", opcode)
			}

			n, err := r.Read(make([]byte, 16))
			if err != nil {
				return fmt.Errorf("read vector const instruction immediates: %w", err)
			} else if n != 16 {
				return fmt.Errorf("read vector const instruction immediates: needs 16 bytes but was %d bytes", n)
			}
		case wasm.OpcodeEnd:
			data := make([]byte, lenAtStart-(r.Len()))
			if _, err := r.ReadAt(data, startPos); err != nil {
				return fmt.Errorf("error re-buffering ConstantExpression.Data: %w", err)
			}
			ret.Data = data
			return nil
		default:
			return fmt.Errorf("%v for const expression op code: %#x", ErrInvalidByte, opcode)
		}

		if err != nil {
			return fmt.Errorf("read value: %v", err)
		}
	}
}
