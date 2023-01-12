package sys_test

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/experimental/sys/systest"
)

func TestRootFS_ReadOnly(t *testing.T) {
	systest.TestReadOnlyFS(t, func(t *testing.T, baseFS fs.FS) sys.FS {
		return sys.RootFS(sys.NewFS(baseFS))
	})
}

func TestRootFS_ReadWrite(t *testing.T) {
	systest.TestReadWriteFS(t, func(t *testing.T) sys.FS {
		return sys.RootFS(sys.DirFS(t.TempDir()))
	})
}

func TestRootFS_Sandbox(t *testing.T) {
	rootFS := sys.RootFS(sys.DirFS("testdata"))

	t.Run("follow symlinks", func(t *testing.T) {
		testFollowSymlink(t, rootFS, "symlink-to-relative-answer", "42\n")
		testFollowSymlink(t, rootFS, "symlink-to-absolute-answer", "42\n")
		testFollowSymlink(t, rootFS, "sub/symlink-to-answer", "42\n")
		testFollowSymlink(t, rootFS, "symlink-to-symlink-to-answer", "42\n")
	})

	t.Run("broken symlinks", func(t *testing.T) {
		testBrokenSymlink(t, rootFS, "sub/symlink-to-nowhere-1", sys.ErrNotExist)
		testBrokenSymlink(t, rootFS, "sub/symlink-to-nowhere-2", sys.ErrNotExist)
		testBrokenSymlink(t, rootFS, "symlink-in-loop", sys.ErrLoop)
		testBrokenSymlink(t, rootFS, "symlink-to-unknown-location", sys.ErrNotExist)
	})
}

func testFollowSymlink(t *testing.T, fsys sys.FS, name, data string) {
	t.Run(name, func(t *testing.T) {
		testFileIsSymlink(t, fsys, name)

		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			t.Error(err)
		} else if string(b) != data {
			t.Errorf("%s: content mismatch: want=%q got=%q", name, data, b)
		}
	})
}

func testBrokenSymlink(t *testing.T, fsys sys.FS, name string, want error) {
	t.Run(name, func(t *testing.T) {
		testFileIsSymlink(t, fsys, name)

		f, err := fsys.Open(name)
		if err == nil {
			f.Close()
			t.Errorf("%s: BROKE OUT OF ROOTFS!!!", name)
		} else if !errors.Is(err, want) {
			t.Errorf("%s: error mismatch: want=%s got=%s", name, want, err)
		}
	})
}

func testFileIsSymlink(t *testing.T, fsys sys.FS, name string) {
	link, err := fsys.OpenFile(name, sys.O_RDONLY|sys.O_NOFOLLOW, 0)
	if err != nil {
		t.Error(err)
		return
	}
	defer link.Close()

	stat, err := link.Stat()
	if err != nil {
		t.Error(err)
	} else if mode := stat.Mode(); mode.Type() != fs.ModeSymlink {
		t.Errorf("%s: not a symbolic link: %s", name, mode)
	}
}
