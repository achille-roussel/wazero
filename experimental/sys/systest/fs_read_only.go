package systest

import (
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
)

// MakeReadOnlyFS is a function type used to instentiate read-only file systems
// during tests.
type MakeReadOnlyFS func(fs.FS) (sys.FS, CloseFS, error)

// TestReadOnlyFS implements a test suite which validate that read-only
// implementations of the sys.FS interface behave according to the spec.
func TestReadOnlyFS(t *testing.T, newFS MakeReadOnlyFS) {
	t.Run("root", func(t *testing.T) {
		testReadOnlyFS(t, newFS)
	})
	t.Run("sub", func(t *testing.T) {
		testReadOnlyFS(t, func(files fs.FS) (sys.FS, CloseFS, error) {
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
			subFS, err := sys.Sub(baseFS, "mnt/subfs")
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
			scenario: "creating a file errors with sys.ErrReadOnly",
			function: testReadOnlyFSCreateFile,
		},

		{
			scenario: "creating a directory errors with sys.ErrReadOnly",
			function: testReadOnlyFSCreateDirectory,
		},

		{
			scenario: "setting file times errors with sys.ErrReadOnly",
			function: testReadOnlyFSChtimes,
		},
	}

	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) { test.function(t, newFS) })
	}

	fsys, closeFS := assertNewFS(t, readOnlyFS(newFS, fstest.MapFS{
		"foo": new(fstest.MapFile),
	}))
	defer closeFS()
	testFSErrNotExist(t, fsys)

	dir := assertOpenFile(t, fsys, ".", 0, 0)
	defer dir.Close()

	file := assertOpenFile(t, fsys, "foo", 0, 0)
	defer file.Close()

	testFileErrNotExist(t, dir)
	testFileErrClosed(t, file)
}

func testReadOnlyFSCreateEmpty(t *testing.T, newFS MakeReadOnlyFS) {
	fsys, closeFS := assertNewFS(t, readOnlyFS(newFS, nil))
	defer assertCloseFS(t, closeFS)
	testFS(t, fsys, nil)
}

func testReadOnlyFSOpenNotExist(t *testing.T, newFS MakeReadOnlyFS) {
	fsys, closeFS := assertNewFS(t, readOnlyFS(newFS, nil))
	defer assertCloseFS(t, closeFS)

	_, err := fsys.OpenFile("test", 0, 0)
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

	_, err := fsys.OpenFile("tmp", sys.O_CREATE, 0644)
	assertErrorIs(t, err, sys.ErrReadOnly)
}

func testReadOnlyFSCreateDirectory(t *testing.T, newFS MakeReadOnlyFS) {
	fsys, closeFS := assertNewFS(t, readOnlyFS(newFS, nil))
	defer assertCloseFS(t, closeFS)

	err := fsys.Mkdir("tmp", 0755)
	assertErrorIs(t, err, sys.ErrReadOnly)
}

func testReadOnlyFSChtimes(t *testing.T, newFS MakeReadOnlyFS) {
	now := time.Now()

	fsys, closeFS := assertNewFS(t, readOnlyFS(newFS, fstest.MapFS{
		"hello": readOnlyFile(now, "world"),
	}))
	defer assertCloseFS(t, closeFS)

	err := fsys.Chtimes("hello", 0, now.Add(time.Second), now)
	assertErrorIs(t, err, sys.ErrReadOnly)
}

func readOnlyFS(newFS MakeReadOnlyFS, files fstest.MapFS) func() (sys.FS, CloseFS, error) {
	return func() (sys.FS, CloseFS, error) { return newFS(files) }
}

func readOnlyFile(modTime time.Time, data string) *fstest.MapFile {
	return &fstest.MapFile{
		Data:    []byte(data),
		Mode:    0444,
		ModTime: modTime,
	}
}
