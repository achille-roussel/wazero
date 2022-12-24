package wasi_snapshot_preview1_test

import (
	"io/fs"
	"os"
	"testing"

	"github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasi/wasitest"
)

func TestContextReadOnlyFS(t *testing.T) {
	wasitest.TestReadOnlyFS(t, func(baseFS fs.FS) (wasi.FS, wasitest.CloseFS, error) {
		ctx := &wasi_snapshot_preview1.Context{FileSystem: wasi.NewFS(baseFS)}
		return ctx.FS(), ctx.Close, nil
	})
}

func TestContextReadWriteFS(t *testing.T) {
	wasitest.TestReadWriteFS(t, func() (wasi.FS, wasitest.CloseFS, error) {
		tmp, err := os.MkdirTemp("", "wasitest.*")
		if err != nil {
			return nil, nil, err
		}
		dirFS, err := wasi.DirFS(tmp)
		if err != nil {
			os.RemoveAll(tmp)
			return nil, nil, err
		}
		ctx := &wasi_snapshot_preview1.Context{FileSystem: dirFS}
		return ctx.FS(), func() error { defer os.RemoveAll(tmp); return ctx.Close() }, nil
	})
}
