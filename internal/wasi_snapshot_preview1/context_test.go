package wasi_snapshot_preview1_test

import (
	"io/fs"
	"os"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/experimental/sys/systest"
	"github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
)

func TestContextReadOnlyFS(t *testing.T) {
	systest.TestReadOnlyFS(t, func(baseFS fs.FS) (sys.FS, systest.CloseFS, error) {
		ctx := &wasi_snapshot_preview1.Context{FileSystem: sys.NewFS(baseFS)}
		return ctx.FS(), ctx.Close, nil
	})
}

func TestContextReadWriteFS(t *testing.T) {
	systest.TestReadWriteFS(t, func() (sys.FS, systest.CloseFS, error) {
		tmp, err := os.MkdirTemp("", "systest.*")
		if err != nil {
			return nil, nil, err
		}
		dirFS, err := sys.DirFS(tmp)
		if err != nil {
			os.RemoveAll(tmp)
			return nil, nil, err
		}
		ctx := &wasi_snapshot_preview1.Context{FileSystem: dirFS}
		return ctx.FS(), func() error { defer os.RemoveAll(tmp); return ctx.Close() }, nil
	})
}
