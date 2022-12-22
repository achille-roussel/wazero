package wasi_test

import (
	"io/fs"
	"testing"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasi/wasitest"
)

func TestFS(t *testing.T) {
	wasitest.TestReadOnlyFS(t, func(baseFS fs.FS) (wasi.FS, wasitest.CloseFS, error) {
		return wasi.NewFS(baseFS), func() {}, nil
	})
}
