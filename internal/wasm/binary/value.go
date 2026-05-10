package binary

import (
	"bytes"
	"fmt"
	"io"
	"unicode/utf8"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func decodeValueTypes(r *bytes.Reader, num uint32) ([]wasm.ValueType, error) {
	types, _, err := decodeValueTypesWithRefInfo(r, num)
	return types, err
}

// decodeValueTypesWithRefInfo reads `num` value types and returns both the
// byte representation (for backward compatibility with code paths that
// only need the shorthand byte) and a parallel rich-info slice.
//
// refInfos is nil when no position requires rich info (the common case
// for Wasm 2.0 modules). When any position is a non-nullable reference or
// a reference to a concrete type index, refInfos has len == num with
// non-nil entries at those positions and nil entries elsewhere.
//
// Non-nullable refs are still placed into `types` as the corresponding
// nullable-shorthand byte so existing byte-only consumers continue to
// see a well-formed value-type byte. The validator (and any consumer
// that cares about nullability or concrete type indices) reads refInfos
// for the precise info.
func decodeValueTypesWithRefInfo(r *bytes.Reader, num uint32) ([]wasm.ValueType, []*wasm.ValueTypeRef, error) {
	if num == 0 {
		return nil, nil, nil
	}

	types := make([]wasm.ValueType, 0, num)
	var refInfos []*wasm.ValueTypeRef
	setRefInfo := func(i int, ref *wasm.ValueTypeRef) {
		if refInfos == nil {
			refInfos = make([]*wasm.ValueTypeRef, num)
		}
		refInfos[i] = ref
	}

	for i := uint32(0); i < num; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return nil, nil, err
		}
		switch b {
		case wasm.ValueTypeI32, wasm.ValueTypeF32, wasm.ValueTypeI64, wasm.ValueTypeF64,
			wasm.ValueTypeExternref, wasm.ValueTypeFuncref, wasm.ValueTypeV128,
			wasm.ValueTypeExnref:
			types = append(types, b)
		case wasm.RefPrefixNullable, wasm.RefPrefixNonNullable:
			nullable := b == wasm.RefPrefixNullable
			ht, _, err := leb128.DecodeInt33AsInt64(r)
			if err != nil {
				return nil, nil, fmt.Errorf("read ref heap type: %w", err)
			}
			kind, typeIdx, ok := wasm.HeapTypeKindFromBinary(ht)
			if !ok {
				return nil, nil, fmt.Errorf("invalid heap type: %d", ht)
			}
			// Place the nullable-shorthand byte in `types` for byte-only
			// consumers. Concrete-ref kinds default to the funcref byte
			// since they have no abstract shorthand (the actual heap
			// type is in refInfos[i].HeapKind and TypeIdx).
			var shorthand byte
			if kind == wasm.HeapTypeKindConcrete {
				shorthand = byte(wasm.ValueTypeFuncref)
			} else if sb, sbOK := kind.AbstractShorthandByte(); sbOK {
				shorthand = sb
			} else {
				shorthand = byte(wasm.ValueTypeFuncref)
			}
			types = append(types, shorthand)
			// Record rich info iff the byte alone doesn't faithfully
			// describe the type: any non-nullable form, or any concrete
			// reference.
			if !nullable || kind == wasm.HeapTypeKindConcrete {
				setRefInfo(int(i), &wasm.ValueTypeRef{
					Nullable: nullable,
					HeapKind: kind,
					TypeIdx:  typeIdx,
				})
			}
		default:
			return nil, nil, fmt.Errorf("invalid value type: %d", b)
		}
	}
	return types, refInfos, nil
}

// decodeUTF8 decodes a size prefixed string from the reader, returning it and the count of bytes read.
// contextFormat and contextArgs apply an error format when present
func decodeUTF8(r *bytes.Reader, contextFormat string, contextArgs ...interface{}) (string, uint32, error) {
	size, sizeOfSize, err := leb128.DecodeUint32(r)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read %s size: %w", fmt.Sprintf(contextFormat, contextArgs...), err)
	}

	if size == 0 {
		return "", uint32(sizeOfSize), nil
	}

	buf := make([]byte, size)
	if _, err = io.ReadFull(r, buf); err != nil {
		return "", 0, fmt.Errorf("failed to read %s: %w", fmt.Sprintf(contextFormat, contextArgs...), err)
	}

	if !utf8.Valid(buf) {
		return "", 0, fmt.Errorf("%s is not valid UTF-8", fmt.Sprintf(contextFormat, contextArgs...))
	}

	ret := unsafe.String(&buf[0], int(size))
	return ret, size + uint32(sizeOfSize), nil
}
