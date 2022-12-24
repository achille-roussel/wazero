package wasi_snapshot_preview1

import (
	"io/fs"
	"testing"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasi/wasitest"
)

func TestContextReadOnlyFS(t *testing.T) {
	wasitest.TestReadOnlyFS(t, func(baseFS fs.FS) (wasi.FS, wasitest.CloseFS, error) {
		ctx := &Context{FileSystem: wasi.NewFS(baseFS)}
		return ctx.FS(), ctx.Close, nil
	})
}
