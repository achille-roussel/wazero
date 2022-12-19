package wasi_test

import (
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasi/wasitest"
)

func TestFS(t *testing.T) {
	wasitest.TestReadOnlyFS(t, func(state fstest.MapFS) (wasi.FS, wasitest.CloseFS, error) {
		return wasi.NewFS(state), func() {}, nil
	})
}
