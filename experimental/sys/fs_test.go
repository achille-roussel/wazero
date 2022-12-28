package sys_test

import (
	"io/fs"
	"os"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/experimental/sys/systest"
)

func TestFS(t *testing.T) {
	systest.TestReadOnlyFS(t, func(baseFS fs.FS) (sys.FS, systest.CloseFS, error) {
		return sys.NewFS(baseFS), func() error { return nil }, nil
	})
}

func TestDirFS(t *testing.T) {
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
		return dirFS, func() error { return os.RemoveAll(tmp) }, nil
	})
}
