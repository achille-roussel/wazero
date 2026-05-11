package wazevo_test

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

// TestGC_I31_Compiler exercises wazevo's wasm-gc i31 lowering end-to-end.
// Phase 3 lowers ref.i31, i31.get_s, i31.get_u, and ref.eq inline as SSA
// arithmetic. This test confirms the lowering produces spec-correct
// results on the compiler engine.
func TestGC_I31_Compiler(t *testing.T) {
	ctx := context.Background()

	// getS(x) -> i32  = i31.get_s(ref.i31(local.get 0))
	getsBody := []byte{
		wasm.OpcodeLocalGet, 0x00,
		wasm.OpcodeGCPrefix, byte(wasm.OpcodeGCRefI31),
		wasm.OpcodeGCPrefix, byte(wasm.OpcodeGCI31GetS),
		wasm.OpcodeEnd,
	}
	// getU(x) -> i32  = i31.get_u(ref.i31(local.get 0))
	getuBody := []byte{
		wasm.OpcodeLocalGet, 0x00,
		wasm.OpcodeGCPrefix, byte(wasm.OpcodeGCRefI31),
		wasm.OpcodeGCPrefix, byte(wasm.OpcodeGCI31GetU),
		wasm.OpcodeEnd,
	}
	// eq(a, b) -> i32 = ref.eq(ref.i31(a), ref.i31(b))
	eqBody := []byte{
		wasm.OpcodeLocalGet, 0x00,
		wasm.OpcodeGCPrefix, byte(wasm.OpcodeGCRefI31),
		wasm.OpcodeLocalGet, 0x01,
		wasm.OpcodeGCPrefix, byte(wasm.OpcodeGCRefI31),
		wasm.OpcodeRefEq,
		wasm.OpcodeEnd,
	}

	mod := &wasm.Module{
		TypeSection: []wasm.FunctionType{
			{Form: wasm.CompositeFormFunc, Params: []wasm.ValueType{wasm.ValueTypeI32}, Results: []wasm.ValueType{wasm.ValueTypeI32}, Final: true},
			{Form: wasm.CompositeFormFunc, Params: []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32}, Results: []wasm.ValueType{wasm.ValueTypeI32}, Final: true},
		},
		FunctionSection: []wasm.Index{0, 0, 1},
		CodeSection: []wasm.Code{
			{Body: getsBody},
			{Body: getuBody},
			{Body: eqBody},
		},
		ExportSection: []wasm.Export{
			{Name: "getS", Type: wasm.ExternTypeFunc, Index: 0},
			{Name: "getU", Type: wasm.ExternTypeFunc, Index: 1},
			{Name: "eq", Type: wasm.ExternTypeFunc, Index: 2},
		},
	}
	bin := binaryencoding.EncodeModule(mod)

	cfg := wazero.NewRuntimeConfigCompiler().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesGC)
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	instance, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	getS := instance.ExportedFunction("getS")
	getU := instance.ExportedFunction("getU")
	eq := instance.ExportedFunction("eq")

	// ref.i31 narrows to 31 bits; i31.get_s sign-extends bit 30 to bit 31.
	getsTests := []struct {
		in   uint32
		want uint32
	}{
		{0, 0},
		{42, 42},
		{0x3FFFFFFF, 0x3FFFFFFF},       // max positive 31-bit
		{0x40000000, 0xC0000000},       // bit 30 set → sign-extended negative
		{0x80000000, 0},                // bit 31 stripped before storage
		{0xFFFFFFFF, 0xFFFFFFFF},       // sign-extends to -1
	}
	for _, tt := range getsTests {
		res, err := getS.Call(ctx, uint64(tt.in))
		require.NoError(t, err)
		got := uint32(res[0])
		require.Equal(t, tt.want, got, "getS(%#x) = %#x, want %#x", tt.in, got, tt.want)
	}

	// i31.get_u: low 31 bits, no sign extension.
	getuTests := []struct {
		in   uint32
		want uint32
	}{
		{0x40000000, 0x40000000},
		{0xFFFFFFFF, 0x7FFFFFFF},
	}
	for _, tt := range getuTests {
		res, err := getU.Call(ctx, uint64(tt.in))
		require.NoError(t, err)
		got := uint32(res[0])
		require.Equal(t, tt.want, got, "getU(%#x) = %#x, want %#x", tt.in, got, tt.want)
	}

	// ref.eq on i31s compares by value (tagged-uintptr bit pattern matches).
	eqTests := []struct {
		a, b uint32
		want uint32
	}{
		{42, 42, 1},
		{42, 43, 0},
		{0, 0, 1},
		{0xFFFFFFFF, 0xFFFFFFFF, 1}, // same packed payload (0x7FFFFFFF)
	}
	for _, tt := range eqTests {
		res, err := eq.Call(ctx, uint64(tt.a), uint64(tt.b))
		require.NoError(t, err)
		got := uint32(res[0])
		require.Equal(t, tt.want, got, "eq(%#x, %#x) = %d, want %d", tt.a, tt.b, got, tt.want)
	}
}
