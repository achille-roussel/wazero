package wazevo

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// TestEngine_CompileModule_AcceptsGCTypes asserts that the optimising
// compiler now accepts modules whose type section contains struct or
// array composite types. Phase 4 of the wasm-gc port adds the
// allocation machinery; instructions not yet implemented (struct.get,
// array.set, ref.test/cast, …) panic at compile time inside lowerGC
// with a clear "TODO: unsupported wasm-gc instruction" message.
func TestEngine_CompileModule_AcceptsGCTypes(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		mod  *wasm.Module
	}{
		{
			name: "struct type",
			mod: &wasm.Module{
				TypeSection: []wasm.FunctionType{
					{
						Form: wasm.CompositeFormStruct,
						Fields: []wasm.FieldType{
							{ValueType: wasm.ValueTypeI32, Mutable: true},
						},
					},
				},
				ID: wasm.ModuleID{},
			},
		},
		{
			name: "array type",
			mod: &wasm.Module{
				TypeSection: []wasm.FunctionType{
					{
						Form:       wasm.CompositeFormArray,
						ArrayField: wasm.FieldType{ValueType: wasm.ValueTypeI32, Mutable: true},
					},
				},
				ID: wasm.ModuleID{0xa},
			},
		},
		{
			name: "func then struct",
			mod: &wasm.Module{
				TypeSection: []wasm.FunctionType{
					{Form: wasm.CompositeFormFunc},
					{
						Form: wasm.CompositeFormStruct,
						Fields: []wasm.FieldType{
							{ValueType: wasm.ValueTypeI64},
						},
					},
				},
				ID: wasm.ModuleID{0xb},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEngine(ctx, 0, nil).(*engine)
			err := e.CompileModule(ctx, tt.mod, nil, false)
			require.NoError(t, err)
		})
	}
}

// TestEngine_CompileModule_AcceptsFuncOnly is a regression guard for
// pre-wasm-gc modules: a module whose type section contains only
// ordinary function types still compiles cleanly.
func TestEngine_CompileModule_AcceptsFuncOnly(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx, 0, nil).(*engine)
	mod := &wasm.Module{
		TypeSection: []wasm.FunctionType{
			{Form: wasm.CompositeFormFunc, Params: []wasm.ValueType{wasm.ValueTypeI32}, Results: []wasm.ValueType{wasm.ValueTypeI32}},
		},
		FunctionSection: []wasm.Index{0},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeEnd}},
		},
		ID: wasm.ModuleID{0xc},
	}
	err := e.CompileModule(ctx, mod, nil, false)
	require.NoError(t, err)
}
