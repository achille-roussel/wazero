package sys_test

// These test suites exercise the support for extended attributes on the local
// file system. If the error "operation not supported" (ENOTSUP) shows up during
// a run, it might be because the underlying file system was not mounted with
// extended attributes enabled, so try doing this and running again:
//
//	$ sudo mount -o remount,user_xattr rw /
//
// See:
// - https://manpages.ubuntu.com/manpages/bionic/en/man5/attr.5.html
// - https://askubuntu.com/questions/175739/how-do-i-remount-a-filesystem-as-read-write

import (
	"io/fs"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/experimental/sys/systest"
)

func TestDirFS_ReadOnly(t *testing.T) {
	systest.TestReadOnlyFS(t, func(t *testing.T, baseFS fs.FS) sys.FS {
		testFS := sys.DirFS(t.TempDir())
		readFS := sys.NewFS(baseFS)
		if err := sys.CopyFS(testFS, readFS); err != nil {
			t.Fatal(err)
		}
		return sys.ReadOnlyFS(testFS)
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
		if err := sys.Mkdir(testFS, "tmp", 0755); err != nil {
			t.Fatal(err)
		}
		f, err := sys.OpenDir(testFS, "tmp")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		return sys.FileFS(f)
	})
}
