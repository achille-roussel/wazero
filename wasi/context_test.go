package wasi_test

import (
	"io/fs"
	"testing"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasi/wasitest"
)

func TestContextReadOnlyFS(t *testing.T) {
	wasitest.TestReadOnlyFS(t, func(baseFS fs.FS) (wasi.FS, wasitest.CloseFS, error) {
		ctx := wasi.NewContext(
			wasi.FileSystem(wasi.NewFS(baseFS)),
		)
		return ctx.FS(), func() { ctx.Close() }, nil
	})
}
