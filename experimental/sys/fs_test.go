package sys_test

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/experimental/sys/systest"
)

func TestErrFS(t *testing.T) {
	err := errors.New("nope")
	systest.TestErrorFS(t, err, func(t *testing.T) sys.FS {
		return sys.ErrFS(err)
	})
}

func TestNewFS(t *testing.T) {
	systest.TestReadOnlyFS(t, func(t *testing.T, baseFS fs.FS) sys.FS {
		return sys.NewFS(baseFS)
	})
}

func TestNewFS_Root(t *testing.T) {
	systest.TestReadOnlyFS(t, func(t *testing.T, baseFS fs.FS) sys.FS {
		testFS := sys.NewFS(baseFS)
		f, err := testFS.OpenFile(".", sys.O_RDONLY|sys.O_DIRECTORY, 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		return f.FS()
	})
}

func TestDirFS(t *testing.T) {
	systest.TestReadWriteFS(t, func(t *testing.T) sys.FS {
		return sys.DirFS(t.TempDir())
	})
}

func TestDirFS_Root(t *testing.T) {
	systest.TestReadWriteFS(t, func(t *testing.T) sys.FS {
		testFS := sys.DirFS(t.TempDir())
		if err := testFS.Mkdir("tmp", 0755); err != nil {
			t.Fatal(err)
		}
		f, err := testFS.OpenFile("tmp", sys.O_RDONLY|sys.O_DIRECTORY, 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		return f.FS()
	})
}

func TestDirFS_ReadOnly(t *testing.T) {
	systest.TestReadOnlyFS(t, func(t *testing.T, baseFS fs.FS) sys.FS {
		testFS := sys.DirFS(t.TempDir())
		if err := sys.CopyFS(testFS, baseFS); err != nil {
			t.Fatal(err)
		}
		return sys.NewFS(testFS)
	})
}

func TestCopyFS(t *testing.T) {
	t0 := time.Now().Truncate(time.Microsecond)
	t1 := t0.Add(time.Second)
	t2 := t0.Add(time.Millisecond)

	testFS := sys.DirFS(t.TempDir())
	baseFS := fstest.MapFS{
		"top_level_file": &fstest.MapFile{
			Data:    []byte(`top level data`),
			Mode:    0644,
			ModTime: t0,
		},

		"top_level_directory": &fstest.MapFile{
			Mode:    0755 | fs.ModeDir,
			ModTime: t1,
		},

		"top_level_directory/sub_level_file0": &fstest.MapFile{
			Data:    []byte(``),
			Mode:    0600,
			ModTime: t2,
		},
	}

	if err := sys.CopyFS(testFS, baseFS); err != nil {
		t.Fatal(err)
	}
	if err := sys.EqualFS(testFS, baseFS); err != nil {
		t.Fatal(err)
	}
}
