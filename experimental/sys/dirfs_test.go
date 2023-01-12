package sys_test

import (
	"io/fs"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/experimental/sys/systest"
)

func TestDirFS_ReadOnly(t *testing.T) {
	systest.TestReadOnlyFS(t, func(t *testing.T, baseFS fs.FS) sys.FS {
		testFS := sys.DirFS(t.TempDir())
		if err := sys.CopyFS(testFS, baseFS); err != nil {
			t.Fatal(err)
		}
		return sys.NewFS(testFS)
	})
}

func TestDirFS_ReadWrite(t *testing.T) {
	systest.TestReadWriteFS(t, func(t *testing.T) sys.FS {
		return sys.DirFS(t.TempDir())
	})
}

func TestDirFS_RootFile(t *testing.T) {
	systest.TestReadWriteFS(t, func(t *testing.T) sys.FS {
		testFS := sys.DirFS(t.TempDir())
		if err := testFS.Mkdir("tmp", 0755); err != nil {
			t.Fatal(err)
		}
		f, err := testFS.OpenFile("tmp", sys.O_DIRECTORY, 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		return f.FS()
	})
}
