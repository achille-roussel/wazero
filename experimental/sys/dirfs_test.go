package sys_test

import (
	"io/fs"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/experimental/sys/systest"
)

func TestDirFS_ReadOnly(t *testing.T) {
	systest.TestReadOnlyFS(t, func(t *testing.T, baseFS fs.FS) sys.FS {
		return sys.ReadOnlyFS(makeDirFS(t, baseFS))
	})
}

func TestDirFS_ReadWrite(t *testing.T) {
	systest.TestReadWriteFS(t, func(t *testing.T, baseFS fs.FS) sys.FS {
		return makeDirFS(t, baseFS)
	})
}

func TestDirFS_RootFile(t *testing.T) {
	systest.TestReadWriteFS(t, func(t *testing.T, baseFS fs.FS) sys.FS {
		testFS := sys.DirFS(t.TempDir())
		if err := sys.Mkdir(testFS, "tmp", 0755); err != nil {
			t.Fatal(err)
		}
		f, err := sys.OpenDir(testFS, "tmp")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		fileFS := sys.FileFS(f)
		readFS := sys.NewFS(baseFS)
		if err := sys.CopyFS(fileFS, readFS); err != nil {
			t.Fatal(err)
		}
		return fileFS
	})
}

func makeDirFS(t *testing.T, baseFS fs.FS) sys.FS {
	testFS := sys.DirFS(t.TempDir())
	readFS := sys.NewFS(baseFS)
	if err := sys.CopyFS(testFS, readFS); err != nil {
		t.Fatal(err)
	}
	return testFS
}
