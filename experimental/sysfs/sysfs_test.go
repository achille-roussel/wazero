package sysfs_test

import (
	"io/fs"
	"os"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sysfs"
	"github.com/tetratelabs/wazero/experimental/systest"
)

func TestNewFS(t *testing.T) {
	systest.TestReadOnlyFS(t, func(base fs.FS) (sysfs.FS, systest.CloseFS, error) {
		return sysfs.NewFS(base), func() {}, nil
	})
}

func TestDirFS(t *testing.T) {
	systest.TestReadWriteFS(t, func() (sysfs.FS, systest.CloseFS, error) {
		tmp, err := os.MkdirTemp("", "sysfs.test.*")
		if err != nil {
			return nil, nil, err
		}
		fsys, err := sysfs.DirFS(tmp)
		if err != nil {
			os.RemoveAll(tmp)
			return nil, nil, err
		}
		return fsys, func() { os.RemoveAll(tmp) }, nil
	})
}
