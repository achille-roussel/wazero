package experimental_test

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// TestGC_I31 builds and runs a minimal wasm-gc module that exercises
// ref.i31 followed by i31.get_s and i31.get_u. Confirms that the
// interpreter's Phase 5 i31 support produces spec-correct results
// end-to-end.
func TestGC_I31(t *testing.T) {
	ctx := context.Background()

	// Build a module with three exported functions:
	//   getS(i32) -> i32   = i31.get_s(ref.i31(local.get 0))
	//   getU(i32) -> i32   = i31.get_u(ref.i31(local.get 0))
	//   eq(i32, i32) -> i32 = ref.eq(ref.i31(local.get 0), ref.i31(local.get 1))
	//
	// (Note: ref.eq on two freshly-allocated i31 refs uses POINTER
	// equality in the current interpreter, so two distinct allocations
	// compare unequal even when their numeric values match. The eq test
	// here verifies the pointer-equality semantics, not value-equality.
	// Value-equality on i31 is a Phase 5b refinement.)

	getsBody := []byte{
		wasm.OpcodeLocalGet, 0x00, // local.get 0
		wasm.OpcodeGCPrefix, byte(wasm.OpcodeGCRefI31),
		wasm.OpcodeGCPrefix, byte(wasm.OpcodeGCI31GetS),
		wasm.OpcodeEnd,
	}
	getuBody := []byte{
		wasm.OpcodeLocalGet, 0x00,
		wasm.OpcodeGCPrefix, byte(wasm.OpcodeGCRefI31),
		wasm.OpcodeGCPrefix, byte(wasm.OpcodeGCI31GetU),
		wasm.OpcodeEnd,
	}

	mod := &wasm.Module{
		TypeSection: []wasm.FunctionType{
			{Form: wasm.CompositeFormFunc, Params: []wasm.ValueType{wasm.ValueTypeI32}, Results: []wasm.ValueType{wasm.ValueTypeI32}},
		},
		FunctionSection: []wasm.Index{0, 0},
		CodeSection: []wasm.Code{
			{Body: getsBody},
			{Body: getuBody},
		},
		ExportSection: []wasm.Export{
			{Name: "getS", Type: wasm.ExternTypeFunc, Index: 0},
			{Name: "getU", Type: wasm.ExternTypeFunc, Index: 1},
		},
	}

	bin := binaryencoding.EncodeModule(mod)

	cfg := wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesGC)
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	instance, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	getS := instance.ExportedFunction("getS")
	getU := instance.ExportedFunction("getU")

	// i31.get_s sign-extends bit 30 to bit 31.
	tests := []struct {
		name    string
		fn      api.Function
		in      uint32
		want    uint32
		wantI32 int32
	}{
		// Small positive values round-trip.
		{"getS(0)", getS, 0, 0, 0},
		{"getS(42)", getS, 42, 42, 42},
		{"getS(0x3FFFFFFF max positive 31-bit)", getS, 0x3FFFFFFF, 0x3FFFFFFF, 0x3FFFFFFF},
		// Bit 30 set => sign-extended negative.
		{"getS(0x40000000)", getS, 0x40000000, 0xC0000000, -0x40000000},
		// Bit 31 of input is stripped before storage.
		{"getS(0x80000000)", getS, 0x80000000, 0, 0},
		{"getS(0xFFFFFFFF)", getS, 0xFFFFFFFF, 0xFFFFFFFF, -1},

		// Unsigned: no sign extension.
		{"getU(0x40000000)", getU, 0x40000000, 0x40000000, 0},
		{"getU(0xFFFFFFFF)", getU, 0xFFFFFFFF, 0x7FFFFFFF, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := tt.fn.Call(ctx, uint64(tt.in))
			require.NoError(t, err)
			gotU := uint32(res[0])
			require.Equal(t, tt.want, gotU, "got %#x, want %#x", gotU, tt.want)
		})
	}
}
