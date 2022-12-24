package wasitest

import (
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero/wasi"
)

// MakeReadOnlyFS is a function type used to instentiate read-only file systems
// during tests.
type MakeReadOnlyFS func(fs.FS) (wasi.FS, CloseFS, error)

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
			scenario: "creating a file errors with wasi.ErrReadOnly",
			function: testReadOnlyFSCreateFile,
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

func testReadOnlyFSCreateFile(t *testing.T, newFS MakeReadOnlyFS) {
	fsys, closeFS := assertNewFS(t, readOnlyFS(newFS, nil))
	defer assertCloseFS(t, closeFS)

	_, err := fsys.OpenFile("tmp", wasi.O_CREATE, 0644)
	assertErrorIs(t, err, wasi.ErrReadOnly)
}

func testReadOnlyFSCreateDirectory(t *testing.T, newFS MakeReadOnlyFS) {
	fsys, closeFS := assertNewFS(t, readOnlyFS(newFS, nil))
	defer assertCloseFS(t, closeFS)

	err := fsys.MakeDir("tmp", 0755)
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
