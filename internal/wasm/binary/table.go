package binary

import (
	"bytes"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// decodeTable returns the wasm.Table decoded with the WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-table
//
// wasm-gc extension: a 0x40 0x00 prefix marks a table-with-init-expr
// form (tabletype expr) per https://webassembly.github.io/gc/core/binary/modules.html#table-section.
// The reftype can also be a non-abstract `(ref t)` / `(ref null t)`
// form (prefix byte 0x63 / 0x64 followed by an s33 heap type).
func decodeTable(r *bytes.Reader, enabledFeatures api.CoreFeatures, ret *wasm.Table) (err error) {
	first, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read leading byte: %v", err)
	}
	hasInit := false
	if first == 0x40 {
		// wasm-gc table-with-init prefix: 0x40 0x00 then tabletype expr.
		next, nerr := r.ReadByte()
		if nerr != nil {
			return fmt.Errorf("read table init prefix continuation: %v", nerr)
		}
		if next != 0x00 {
			return fmt.Errorf("invalid table init prefix: 0x40 followed by 0x%x", next)
		}
		hasInit = true
		first, err = r.ReadByte()
		if err != nil {
			return fmt.Errorf("read table reftype after init prefix: %v", err)
		}
	}
	// The reftype may be a `(ref t)` / `(ref null t)` prefix-form; in
	// that case consume the s33 heap type and record the funcref
	// sentinel byte so the byte-level table slot type is consistent
	// with our concrete-ref convention.
	if first == wasm.RefPrefixNullable || first == wasm.RefPrefixNonNullable {
		if _, _, lerr := leb128.DecodeInt33AsInt64(r); lerr != nil {
			return fmt.Errorf("read table ref heap type: %w", lerr)
		}
		ret.Type = wasm.RefTypeFuncref
	} else {
		ret.Type = first
	}

	if ret.Type != wasm.RefTypeFuncref {
		if err = enabledFeatures.RequireEnabled(api.CoreFeatureReferenceTypes); err != nil {
			return fmt.Errorf("table type funcref is invalid: %w", err)
		}
	}

	var shared bool
	ret.Min, ret.Max, shared, err = decodeLimitsType(r)
	if err != nil {
		return fmt.Errorf("read limits: %v", err)
	}
	if ret.Min > wasm.MaximumFunctionIndex {
		return fmt.Errorf("table min must be at most %d", wasm.MaximumFunctionIndex)
	}
	if ret.Max != nil {
		if *ret.Max < ret.Min {
			return fmt.Errorf("table size minimum must not be greater than maximum")
		}
	}
	if shared {
		return fmt.Errorf("tables cannot be marked as shared")
	}
	if hasInit {
		ret.Init = &wasm.ConstantExpression{}
		if err = decodeConstantExpression(r, enabledFeatures, ret.Init); err != nil {
			return fmt.Errorf("read table init const expression: %w", err)
		}
	}
	return
}
