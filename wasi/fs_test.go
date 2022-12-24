package wasi_test

import (
	"io/fs"
	"os"
	"testing"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasi/wasitest"
)

func TestFS(t *testing.T) {
	wasitest.TestReadOnlyFS(t, func(baseFS fs.FS) (wasi.FS, wasitest.CloseFS, error) {
		return wasi.NewFS(baseFS), func() error { return nil }, nil
	})
}

func TestDirFS(t *testing.T) {
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
		return dirFS, func() error { return os.RemoveAll(tmp) }, nil
	})
}
