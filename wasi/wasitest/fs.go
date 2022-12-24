package wasitest

import (
	"errors"
	"io"
	"io/fs"
	"sort"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/wasi"
)

// CloseFS is a function returned by MakeReadOnlyFS and MakeReadWriteFS which is
// used to tear down resources associated with a file system instance created
// during a test.
type CloseFS func() error

// MakeReadOnlyFS is a function type used to instentiate read-only file systems
// during tests.
type MakeReadOnlyFS func(fs.FS) (wasi.FS, CloseFS, error)

// MakeReadWriteFS is a function type used to instantiate read-write file
// systems during tests.
type MakeReadWriteFS func() (wasi.FS, CloseFS, error)

// TestReadOnlyFS implements a test suite which validate that read-only
// implementations of the wasi.FS interface behave according to the spec.
func TestReadOnlyFS(t *testing.T, newFS MakeReadOnlyFS) {
	t.Run("root", func(t *testing.T) {
		testReadOnlyFS(t, newFS)
	})
	t.Run("sub", func(t *testing.T) {
		testReadOnlyFS(t, func(files fs.FS) (wasi.FS, CloseFS, error) {
			mapFiles := files.(fstest.MapFS)
			subFiles := make(fstest.MapFS, len(mapFiles))
			for name, file := range mapFiles {
				subFiles["mnt/subfs/"+name] = file
			}
			baseFS, closeFS, err := newFS(subFiles)
			if err != nil {
				return nil, nil, err
			}
			if len(subFiles) == 0 {
				return baseFS, closeFS, nil
			}
			subFS, err := wasi.Sub(baseFS, "mnt/subfs")
			if err != nil {
				closeFS()
				return nil, nil, err
			}
			return subFS, closeFS, nil
		})
	})
}

func testReadOnlyFS(t *testing.T, newFS MakeReadOnlyFS) {
	tests := []struct {
		scenario string
		function func(*testing.T, MakeReadOnlyFS)
	}{
		{
			scenario: "create a file system with an empty state",
			function: testReadOnlyFSCreateEmpty,
		},

		{
			scenario: "opening a file which does not exist gives fs.ErrNotExist",
			function: testReadOnlyFSOpenNotExist,
		},

		{
			scenario: "existing files can be open and read",
			function: testReadOnlyFSOpenAndRead,
		},

		{
			scenario: "creating a directory errors with wasi.ErrReadOnly",
			function: testReadOnlyFSCreateDirectory,
		},

		{
			scenario: "setting file times errors with wasi.ErrReadOnly",
			function: testReadOnlyFSSetFileTimes,
		},
	}

	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) { test.function(t, newFS) })
	}
}

func testReadOnlyFSCreateEmpty(t *testing.T, newFS MakeReadOnlyFS) {
	fsys, closeFS := assertNewFS(t, readOnlyFS(newFS, nil))
	defer assertCloseFS(t, closeFS)
	testFS(t, fsys, nil)
}

func testReadOnlyFSOpenNotExist(t *testing.T, newFS MakeReadOnlyFS) {
	fsys, closeFS := assertNewFS(t, readOnlyFS(newFS, nil))
	defer assertCloseFS(t, closeFS)

	_, err := fsys.OpenFile("/test", 0, 0)
	assertErrorIs(t, err, fs.ErrNotExist)
}

func testReadOnlyFSOpenAndRead(t *testing.T, newFS MakeReadOnlyFS) {
	now := time.Now()

	files := fstest.MapFS{
		"file-0": readOnlyFile(now, `Hello World!`),
		"file-1": readOnlyFile(now, `42`),
		"file-2": readOnlyFile(now, ``),
	}

	fsys, closeFS := assertNewFS(t, readOnlyFS(newFS, files))
	defer assertCloseFS(t, closeFS)

	assertPathData(t, fsys, "file-0", `Hello World!`)
	assertPathData(t, fsys, "file-1", `42`)
	assertPathData(t, fsys, "file-2", ``)

	testFS(t, fsys, files)
}

func testReadOnlyFSCreateDirectory(t *testing.T, newFS MakeReadOnlyFS) {
	fsys, closeFS := assertNewFS(t, readOnlyFS(newFS, nil))
	defer assertCloseFS(t, closeFS)

	err := fsys.CreateDir("tmp", 0755)
	assertErrorIs(t, err, wasi.ErrReadOnly)
}

func testReadOnlyFSSetFileTimes(t *testing.T, newFS MakeReadOnlyFS) {
	now := time.Now()

	fsys, closeFS := assertNewFS(t, readOnlyFS(newFS, fstest.MapFS{
		"hello": readOnlyFile(now, "world"),
	}))
	defer assertCloseFS(t, closeFS)

	err := fsys.SetFileTimes("hello", 0, now.Add(time.Second), now)
	assertErrorIs(t, err, wasi.ErrReadOnly)
}

func readOnlyFS(newFS MakeReadOnlyFS, files fstest.MapFS) func() (wasi.FS, CloseFS, error) {
	return func() (wasi.FS, CloseFS, error) { return newFS(files) }
}

func readOnlyFile(modTime time.Time, data string) *fstest.MapFile {
	return &fstest.MapFile{
		Data:    []byte(data),
		Mode:    0444,
		ModTime: modTime,
	}
}

// TestReadOnlyFS implements a test suite which validate that read-write
// implementations of the wasi.FS interface behave according to the spec.
func TestReadWriteFS(t *testing.T, newFS MakeReadWriteFS) {
	t.Run("root", func(t *testing.T) {
		testReadWriteFS(t, newFS)
	})
	t.Run("sub", func(t *testing.T) {
		testReadWriteFS(t, func() (wasi.FS, CloseFS, error) {
			baseFS, closeFS, err := newFS()
			if err != nil {
				return nil, nil, err
			}
			if err := baseFS.CreateDir("mnt", 0755); err != nil {
				closeFS()
				return nil, nil, err
			}
			subFS, err := wasi.Sub(baseFS, "mnt")
			if err != nil {
				closeFS()
				return nil, nil, err
			}
			return subFS, closeFS, nil
		})
	})
}

func testReadWriteFS(t *testing.T, newFS MakeReadWriteFS) {
	tests := []struct {
		scenario string
		function func(*testing.T, MakeReadWriteFS)
	}{
		{
			scenario: "create a file system with an empty state",
			function: testReadWriteFSCreateEmpty,
		},

		{
			scenario: "the file system can create directories",
			function: testReadWriteFSCreateDirectory,
		},

		{
			scenario: "the file system can create sub-directories",
			function: testReadWriteFSCreateSubDirectory,
		},

		{
			scenario: "creating an existing directory errors with fs.ErrExist",
			function: testReadWriteFSCreateExistingDirectory,
		},

		{
			scenario: "permissions are set on directories",
			function: testReadWriteFSCreateDirectoryHasPermissions,
		},

		{
			scenario: "set access time on file",
			function: testReadWriteFSSetFileAccessTime,
		},

		{
			scenario: "set modification time on file",
			function: testReadWriteFSSetFileModTime,
		},

		{
			scenario: "set access and modification times on file",
			function: testReadWriteFSSetFileTimes,
		},
	}

	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) { test.function(t, newFS) })
	}
}

func testReadWriteFSCreateEmpty(t *testing.T, newFS MakeReadWriteFS) {
	_, closeFS := assertNewFS(t, newFS)
	assertCloseFS(t, closeFS)
}

func testReadWriteFSCreateDirectory(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	assertCreateDir(t, fsys, "etc")
	assertCreateDir(t, fsys, "var")
	assertCreateDir(t, fsys, "tmp")

	testFS(t, fsys, fstest.MapFS{
		"etc": nil,
		"var": nil,
		"tmp": nil,
	})
}

func testReadWriteFSCreateSubDirectory(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	assertCreateDir(t, fsys, "1")
	assertCreateDir(t, fsys, "1/2")
	assertCreateDir(t, fsys, "1/2/3")

	testFS(t, fsys, fstest.MapFS{
		"1/2/3": nil,
	})
}

func testReadWriteFSCreateExistingDirectory(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	assertCreateDir(t, fsys, "tmp")
	assertErrorIs(t, fsys.CreateDir("tmp", 0755), fs.ErrExist)
}

func testReadWriteFSSetFileAccessTime(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	now := time.Now().Add(time.Hour)
	assertCreateDir(t, fsys, "tmp")
	assertSetFileTimes(t, fsys, "tmp", now, time.Time{})
}

func testReadWriteFSSetFileModTime(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	now := time.Now().Add(time.Hour)
	assertCreateDir(t, fsys, "tmp")
	assertSetFileTimes(t, fsys, "tmp", time.Time{}, now)
}

func testReadWriteFSSetFileTimes(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	now := time.Now().Add(time.Hour)
	assertCreateDir(t, fsys, "tmp")
	assertSetFileTimes(t, fsys, "tmp", now, now)
}

func testFS(t *testing.T, fsys fs.FS, files fstest.MapFS) {
	t.Helper()
	t.Run("fstest", func(t *testing.T) {
		expected := make([]string, 0, len(files))
		for fileName := range files {
			expected = append(expected, fileName)
		}
		sort.Strings(expected)
		if err := fstest.TestFS(fsys, expected...); err != nil {
			t.Error(err)
		}
	})
}

func testReadWriteFSCreateDirectoryHasPermissions(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	assertErrorIs(t, fsys.CreateDir("A", 0755), nil)
	assertErrorIs(t, fsys.CreateDir("B", 0700), nil)
	assertErrorIs(t, fsys.CreateDir("C", 0500), nil)

	assertPermission(t, fsys, "A", 0755)
	assertPermission(t, fsys, "B", 0700)
	assertPermission(t, fsys, "C", 0500)
}

func assertNewFS(t *testing.T, newFS func() (wasi.FS, CloseFS, error)) (wasi.FS, CloseFS) {
	t.Helper()
	fsys, closeFS, err := newFS()
	if err != nil {
		t.Fatal("creating file system:", err)
	}
	return fsys, closeFS
}

func assertClose(t *testing.T, c io.Closer) {
	t.Helper()
	assertErrorIs(t, c.Close(), nil)
}

func assertCloseFS(t *testing.T, closeFS CloseFS) {
	t.Helper()
	assertErrorIs(t, closeFS(), nil)
}

func assertOpenFile(t *testing.T, fsys wasi.FS, path string, oflags int, perm fs.FileMode) wasi.File {
	t.Helper()
	f, err := fsys.OpenFile(path, oflags, perm)
	assertErrorIs(t, err, nil)
	return f
}

func assertPathData(t *testing.T, fsys wasi.FS, path, data string) {
	t.Helper()
	f := assertOpenFile(t, fsys, path, 0, 0)
	defer assertClose(t, f)
	b, err := io.ReadAll(f)
	assertErrorIs(t, err, nil)
	if string(b) != data {
		t.Errorf("%s: content mismatch\nwant: %q\ngot:  %q", path, data, b)
	}
}

func assertCreateDir(t *testing.T, fsys wasi.FS, path string) {
	t.Helper()
	assertErrorIs(t, fsys.CreateDir(path, 0755), nil)
	s, err := fsys.Stat(path)
	if err != nil {
		t.Error(err)
	} else if !s.IsDir() {
		t.Errorf("%s: not a directory", path)
	} else if perm := s.Mode() & 0777; perm != 0755 {
		t.Errorf("%s: permissions mismatch: want=%03o got=%03o", path, 0755, perm)
	}
}

func assertSetFileTimes(t *testing.T, fsys wasi.FS, path string, atim, mtim time.Time) {
	t.Helper()
	assertErrorIs(t, fsys.SetFileTimes(path, 0, atim, mtim), nil)
	s, err := fsys.Stat(path)
	if err != nil {
		t.Error(err)
	} else {
		statAtim := time.Time{}
		statMtim := time.Time{}

		switch stat := s.Sys().(type) {
		case *wasi_snapshot_preview1.Filestat:
			statAtim = stat.Atim.Time()
			statMtim = stat.Mtim.Time()
		case *syscall.Stat_t:
			statAtim = time.Unix(stat.Atim.Unix())
			statMtim = time.Unix(stat.Mtim.Unix())
		default:
			t.Error("unsupported file stat is neither wasi_snapshot_preview1.Filestat nor syscall.Stat_t")
		}

		if !atim.IsZero() {
			if !atim.Equal(statAtim) {
				t.Errorf("access time mismatch: want=%v got=%v", atim, statAtim)
			}
		}

		if !mtim.IsZero() {
			if !mtim.Equal(statMtim) {
				t.Errorf("modification time mismatch: want=%v got=%v", mtim, statMtim)
			}
		}
	}
}

func assertPermission(t *testing.T, fsys wasi.FS, path string, want fs.FileMode) {
	t.Helper()
	s, err := fsys.StatFile(path, 0)
	if err != nil {
		t.Error(err)
	} else {
		perm := s.Mode() & 0777
		if perm != want {
			t.Errorf("%s: permissions mismatch: want=%03o got=%03o", path, want, perm)
		}
	}
}

func assertErrorIs(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Errorf("error mismatch\nwant: %v\ngot:  %v", want, got)
	}
}
