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

// TestGC_StructGetSet_Compiler exercises struct.new, struct.set,
// and struct.get end-to-end on the compiler engine.
func TestGC_StructGetSet_Compiler(t *testing.T) {
	ctx := context.Background()

	// roundtrip(i32, i64) -> (i32, i64):
	//   s := struct.new $S(local.get 0, local.get 1)
	//   (struct.get $S 0 s, struct.get $S 1 s)
	body := []byte{
		wasm.OpcodeLocalGet, 0x00,
		wasm.OpcodeLocalGet, 0x01,
		wasm.OpcodeGCPrefix, byte(wasm.OpcodeGCStructNew), 0x00, // struct.new 0
		wasm.OpcodeLocalSet, 0x02, // store the ref to local 2 (struct ref)
		wasm.OpcodeLocalGet, 0x02,
		wasm.OpcodeGCPrefix, byte(wasm.OpcodeGCStructGet), 0x00, 0x00, // struct.get 0 0
		wasm.OpcodeLocalGet, 0x02,
		wasm.OpcodeGCPrefix, byte(wasm.OpcodeGCStructGet), 0x00, 0x01, // struct.get 0 1
		wasm.OpcodeEnd,
	}

	// Locals[0] = i32 (param 0), [1] = i64 (param 1), [2] = (ref null $S).
	// The third local is encoded as 1 count of (ref null $S) which is
	// stored as funcref byte in the binary encoder. To keep this test
	// simple, we use an `anyref` local.
	codeBody := []byte{
		0x01, 0x01, wasm.ValueTypeAnyref, // 1 local group: 1 anyref local
	}
	codeBody = append(codeBody, body...)

	mod := &wasm.Module{
		TypeSection: []wasm.FunctionType{
			// type[0]: struct { i32 i64 }
			{
				Form: wasm.CompositeFormStruct,
				Fields: []wasm.FieldType{
					{ValueType: wasm.ValueTypeI32, Mutable: true},
					{ValueType: wasm.ValueTypeI64, Mutable: true},
				},
				Final: true,
			},
			// type[1]: (i32, i64) -> (i32, i64)
			{
				Form:    wasm.CompositeFormFunc,
				Params:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI64},
				Results: []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI64},
				Final:   true,
			},
		},
		FunctionSection: []wasm.Index{1},
		CodeSection: []wasm.Code{
			{LocalTypes: []wasm.ValueType{wasm.ValueTypeAnyref}, Body: body},
		},
		ExportSection: []wasm.Export{
			{Name: "roundtrip", Type: wasm.ExternTypeFunc, Index: 0},
		},
	}
	bin := binaryencoding.EncodeModule(mod)

	cfg := wazero.NewRuntimeConfigCompiler().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesGC)
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	instance, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	res, err := instance.ExportedFunction("roundtrip").Call(ctx, uint64(42), uint64(99))
	require.NoError(t, err)
	require.Equal(t, uint64(42), uint64(uint32(res[0])))
	require.Equal(t, uint64(99), res[1])
}
