package wasi_test

import (
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasi/wasitest"
)

func TestContextReadOnlyFS(t *testing.T) {
	wasitest.TestReadOnlyFS(t, func(files fstest.MapFS) (wasi.FS, wasitest.CloseFS, error) {
		ctx := wasi.NewContext(
			wasi.FileSystem(wasi.NewFS(files)),
		)
		return ctx.FS(), func() { ctx.Close() }, nil
	})
}
