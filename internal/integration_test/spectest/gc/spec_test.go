package spectest

import (
	"context"
	"embed"
	"math"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/integration_test/spectest"
	"github.com/tetratelabs/wazero/internal/platform"
)

//go:embed testdata
var testcases embed.FS

const enabledFeatures = api.CoreFeaturesV2 |
	experimental.CoreFeaturesExceptionHandling |
	experimental.CoreFeaturesTailCall |
	experimental.CoreFeaturesGC

// gcTestCases enumerates the WebAssembly GC proposal's spec-test .wast
// files (https://github.com/WebAssembly/gc/tree/main/test/core/gc),
// pre-converted to JSON + .wasm via `wasm-tools json-from-wast`.
//
// All 16 suites are listed. The interpreter is the only engine that
// implements wasm-gc (wazevo's CompileModule rejects GC modules with a
// clear error per Phase 6); the optimizing-compiler test variant skips
// itself accordingly.
var gcTestCases = []string{
	"array",
	"array_copy",
	"array_fill",
	"array_init_data",
	"array_init_elem",
	"array_new_data",
	"array_new_elem",
	"br_on_cast",
	"br_on_cast_fail",
	"extern",
	"i31",
	"ref_cast",
	"ref_eq",
	"ref_test",
	"struct",
	"type-subtyping",
}

func TestCompiler(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	// The optimising compiler does not yet support wasm-gc — the Phase 6
	// guardrail rejects struct/array modules upfront. Skip the suite.
	t.Skip("wasm-gc is not yet supported by the optimizing compiler")
}

func TestInterpreter(t *testing.T) {
	ctx := context.Background()
	config := wazero.NewRuntimeConfigInterpreter().WithCoreFeatures(enabledFeatures)
	for _, name := range gcTestCases {
		spectest.RunCase(t, testcases, name, ctx, config, -1, 0, math.MaxInt)
	}
}
