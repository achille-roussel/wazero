package systest

import (
	"testing"

	"github.com/tetratelabs/wazero/experimental/sysfs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// MakeReadWriteFS is a function type used to instantiate read-write file
// systems during tests.
type MakeReadWriteFS func() (sysfs.FS, CloseFS, error)

// TestReadOnlyFS implements a test suite which validate that read-write
// implementations of the sysfs.FS interface behave according to the spec.
func TestReadWriteFS(t *testing.T, makeFS MakeReadWriteFS) {
	withFS := func(t *testing.T, do func(sysfs.FS)) {
		fsys, closeFS, err := makeFS()
		if err != nil {
			t.Fatal(err)
		}
		defer closeFS()
		do(fsys)
	}

	withFS(t, func(fsys sysfs.FS) {
		dir, err := fsys.OpenFile(".", sysfs.O_DIRECTORY, 0)
		require.NoError(t, err)

		file, err := fsys.OpenFile("test", sysfs.O_CREATE|sysfs.O_RDWR, 0644)
		require.NoError(t, err)

		testFileErrIsDir(t, dir)
		testFileErrNotDir(t, file)
		testFileErrClosed(t, file)
	})
}
