package wasitest

import (
	"io"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero/wasi"
)

// MakeReadWriteFS is a function type used to instantiate read-write file
// systems during tests.
type MakeReadWriteFS func() (wasi.FS, CloseFS, error)

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
			if err := baseFS.MakeDir("mnt", 0755); err != nil {
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
			scenario: "files are created when opened with wasi.O_CREATE",
			function: testReadWriteFSCreateFileWithOpen,
		},

		{
			scenario: "files cannot be recreated when opened with wasi.O_EXCL",
			function: testReadWriteFSCannotCreateExistingFile,
		},

		{
			scenario: "files can be truncated when opened with wasi.O_TRUNC",
			function: testReadWriteFSTruncateFileWithOpen,
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

	assertMakeDir(t, fsys, "etc", 0755)
	assertMakeDir(t, fsys, "var", 0755)
	assertMakeDir(t, fsys, "tmp", 0755)

	testFS(t, fsys, fstest.MapFS{
		"etc": nil,
		"var": nil,
		"tmp": nil,
	})
}

func testReadWriteFSCreateSubDirectory(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	assertMakeDir(t, fsys, "1", 0755)
	assertMakeDir(t, fsys, "1/2", 0755)
	assertMakeDir(t, fsys, "1/2/3", 0755)

	testFS(t, fsys, fstest.MapFS{
		"1/2/3": nil,
	})
}

func testReadWriteFSCreateExistingDirectory(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	assertMakeDir(t, fsys, "tmp", 0755)
	assertErrorIs(t, fsys.MakeDir("tmp", 0755), fs.ErrExist)
}

func testReadWriteFSCreateDirectoryHasPermissions(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	assertMakeDir(t, fsys, "A", 0755)
	assertMakeDir(t, fsys, "B", 0700)
	assertMakeDir(t, fsys, "C", 0500)
}

func testReadWriteFSCreateFileWithOpen(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	f0 := assertOpenFile(t, fsys, "foo", wasi.O_CREATE, 0644)
	defer assertClose(t, f0)

	// OK because O_EXCL is not set
	f1 := assertOpenFile(t, fsys, "foo", wasi.O_CREATE, 0644)
	defer assertClose(t, f1)
}

func testReadWriteFSCannotCreateExistingFile(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	f0 := assertOpenFile(t, fsys, "foo", wasi.O_CREATE, 0644)
	assertClose(t, f0)

	_, err := fsys.OpenFile("foo", wasi.O_CREATE|wasi.O_EXCL, 0644)
	assertErrorIs(t, err, fs.ErrExist)
}

func testReadWriteFSTruncateFileWithOpen(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	f0 := assertOpenFile(t, fsys, "foo", wasi.O_RDWR|wasi.O_CREATE, 0644)
	defer assertClose(t, f0)

	assertWrite(t, f0, "Hello World!")

	f1 := assertOpenFile(t, fsys, "foo", wasi.O_RDWR|wasi.O_TRUNC, 0)
	defer assertClose(t, f1)

	assertSeek(t, f0, 0, io.SeekStart)
	b0 := assertRead(t, f0)
	if len(b0) != 0 {
		t.Errorf("original file still has access to content after truncation: %q", b0)
	}

	b1 := assertRead(t, f1)
	if len(b1) != 0 {
		t.Errorf("second file still has access to content after truncation: %q", b1)
	}
}

func testReadWriteFSSetFileAccessTime(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	now := time.Now().Add(time.Hour)
	assertMakeDir(t, fsys, "tmp", 0755)
	assertSetFileTimes(t, fsys, "tmp", now, time.Time{})
}

func testReadWriteFSSetFileModTime(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	now := time.Now().Add(time.Hour)
	assertMakeDir(t, fsys, "tmp", 0755)
	assertSetFileTimes(t, fsys, "tmp", time.Time{}, now)
}

func testReadWriteFSSetFileTimes(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	now := time.Now().Add(time.Hour)
	assertMakeDir(t, fsys, "tmp", 0755)
	assertSetFileTimes(t, fsys, "tmp", now, now)
}
