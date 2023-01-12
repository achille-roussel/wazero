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
	t.Run("FS", func(t *testing.T) {
		testSandbox(t, rootFS)
	})
	t.Run("File", func(t *testing.T) {
		f, err := rootFS.OpenFile(".", sys.O_DIRECTORY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		testSandbox(t, f.FS())
	})
}

func testSandbox(t *testing.T, fsys sys.FS) {
	t.Run("follow symlinks", func(t *testing.T) {
		testFollowSymlink(t, fsys, "symlink-to-relative-answer", "42\n")
		testFollowSymlink(t, fsys, "symlink-to-absolute-answer", "42\n")
		testFollowSymlink(t, fsys, "sub/symlink-to-answer", "42\n")
		testFollowSymlink(t, fsys, "symlink-to-symlink-to-answer", "42\n")
	})

	t.Run("broken symlinks", func(t *testing.T) {
		testBrokenSymlink(t, fsys, "sub/symlink-to-nowhere-1", sys.ErrNotExist)
		testBrokenSymlink(t, fsys, "sub/symlink-to-nowhere-2", sys.ErrNotExist)
		testBrokenSymlink(t, fsys, "symlink-to-unknown-location", sys.ErrNotExist)
		testBrokenSymlink(t, fsys, "symlink-in-loop", sys.ErrLoop)
	})

	t.Run("forbidden paths", func(t *testing.T) {
		testForbiddenPath(t, fsys, "..")
		testForbiddenPath(t, fsys, "../")
		testForbiddenPath(t, fsys, "./..")
		testForbiddenPath(t, fsys, "../.")
		testForbiddenPath(t, fsys, "../..")
		testForbiddenPath(t, fsys, "../../")
	})
}

func testFollowSymlink(t *testing.T, fsys sys.FS, path, data string) {
	t.Run(path, func(t *testing.T) {
		testFileIsSymlink(t, fsys, path)

		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			t.Error(err)
		} else if string(b) != data {
			t.Errorf("%s: content mismatch: want=%q got=%q", path, data, b)
		}
	})
}

func testBrokenSymlink(t *testing.T, fsys sys.FS, path string, want error) {
	t.Run(path, func(t *testing.T) {
		testFileIsSymlink(t, fsys, path)

		f, err := fsys.Open(path)
		if err == nil {
			f.Close()
			t.Errorf("%s: BROKE OUT OF ROOTFS!!!", path)
		} else if !errors.Is(err, want) {
			t.Errorf("%s: error mismatch: want=%s got=%s", path, want, err)
		}
	})
}

func testForbiddenPath(t *testing.T, fsys sys.FS, path string) {
	t.Run(path, func(t *testing.T) {
		f, err := fsys.Open(path)
		if err == nil {
			f.Close()
			t.Errorf("%s: BROKE OUT OF ROOTFS!!!", path)
		} else if !errors.Is(err, sys.ErrNotExist) {
			t.Errorf("%s: error mismatch: want=%s got=%s", path, sys.ErrNotExist, err)
		}
	})
}

func testFileIsSymlink(t *testing.T, fsys sys.FS, path string) {
	link, err := fsys.OpenFile(path, sys.O_RDONLY|sys.O_NOFOLLOW, 0)
	if err != nil {
		t.Error(err)
		return
	}
	defer link.Close()

	stat, err := link.Stat()
	if err != nil {
		t.Error(err)
	} else if mode := stat.Mode(); mode.Type() != fs.ModeSymlink {
		t.Errorf("%s: not a symbolic link: %s", path, mode)
	}
}
