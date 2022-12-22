package wasitest

import (
	"errors"
	"io"
	"io/fs"
	"sort"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero/wasi"
)

// CloseFS is a function returned by MakeReadOnlyFS and MakeReadWriteFS which is
// used to tear down resources associated with a file system instance created
// during a test.
type CloseFS func()

// MakeReadOnlyFS is a function type used to instentiate read-only file systems
// during tests.
type MakeReadOnlyFS func(fs.FS) (wasi.FS, CloseFS, error)

// MakeReadWriteFS is a function type used to instantiate read-write file
// systems during tests.
type MakeReadWriteFS func() (wasi.FS, CloseFS, error)

// TestReadOnlyFS implements a test suite which validate that read-only
// implementations of the wasi.FS interface behave according to the spec.
func TestReadOnlyFS(t *testing.T, newFS MakeReadOnlyFS) {
	tests := []struct {
		scenario string
		function func(*testing.T, MakeReadOnlyFS)
	}{
		{
			scenario: "create a file system with an empty state",
			function: testReadOnlyFSCreateEmpty,
		},

		{
			scenario: "opening a file which does not exist gives ErrNotExist",
			function: testReadOnlyFSOpenNotExist,
		},

		{
			scenario: "existing files can be open and read",
			function: testReadOnlyFSOpenAndRead,
		},
	}

	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) { test.function(t, newFS) })
	}
}

func testReadOnlyFSCreateEmpty(t *testing.T, newFS MakeReadOnlyFS) {
	fsys, closeFS := assertNewFS(t, func() (wasi.FS, CloseFS, error) {
		return newFS(fstest.MapFS{})
	})
	defer closeFS()
	testFS(t, fsys, nil)
}

func testReadOnlyFSOpenNotExist(t *testing.T, newFS MakeReadOnlyFS) {
	fsys, closeFS := assertNewFS(t, func() (wasi.FS, CloseFS, error) {
		return newFS(fstest.MapFS{})
	})
	defer closeFS()

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

	fsys, closeFS := assertNewFS(t, func() (wasi.FS, CloseFS, error) { return newFS(files) })
	defer closeFS()

	assertPathData(t, fsys, "file-0", `Hello World!`)
	assertPathData(t, fsys, "file-1", `42`)
	assertPathData(t, fsys, "file-2", ``)

	testFS(t, fsys, files)
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
	tests := []struct {
		scenario string
		function func(*testing.T, MakeReadWriteFS)
	}{
		{
			scenario: "create a file system with an empty state",
			function: testReadWriteFSCreateEmpty,
		},
	}

	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) { test.function(t, newFS) })
	}
}

func testReadWriteFSCreateEmpty(t *testing.T, newFS MakeReadWriteFS) {
	_, closeFS := assertNewFS(t, newFS)
	closeFS()
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

func assertErrorIs(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Errorf("error mismatch\nwant: %v\ngot:  %v", want, got)
	}
}
