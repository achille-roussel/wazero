package systest

import (
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero/experimental/sysfs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// MakeReadOnlyFS is a function type used to instentiate read-only file systems
// during tests.
type MakeReadOnlyFS func(fs.FS) (sysfs.FS, CloseFS, error)

// TestReadOnlyFS implements a test suite which validate that read-only
// implementations of the sysfs.FS interface behave according to the spec.
func TestReadOnlyFS(t *testing.T, makeFS MakeReadOnlyFS) {
	withFS := func(t *testing.T, base fs.FS, do func(sysfs.FS)) {
		fsys, closeFS, err := makeFS(base)
		if err != nil {
			t.Fatal(err)
		}
		defer closeFS()
		do(fsys)
	}

	now := time.Now()

	files := fstest.MapFS{
		"file-0": readOnlyFile(now, `Hello World!`),
		"file-1": readOnlyFile(now, `42`),
		"file-2": readOnlyFile(now, ``),
	}

	withFS(t, files, func(fsys sysfs.FS) {
		dir, err := fsys.OpenFile(".", 0, 0)
		require.NoError(t, err)

		file, err := fsys.OpenFile("file-0", 0, 0)
		require.NoError(t, err)

		testFileErrIsDir(t, dir)
		testFileErrNotDir(t, file)
		testFileErrClosed(t, file)
		testFS(t, fsys, files)
	})
}

func readOnlyFile(modTime time.Time, data string) *fstest.MapFile {
	return &fstest.MapFile{
		Data:    []byte(data),
		Mode:    0444,
		ModTime: modTime,
	}
}
