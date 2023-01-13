package systest

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero/experimental/sys"
)

// TestReadOnlyFS is a test suite used to test the capabilities of file systems
// supporting only read operations.
//
// The intent is for this test suite to help validate that read-only
// implementations of the sys.FS interface all exhibit the same behavior.
func TestReadOnlyFS(t *testing.T, makeFS MakeFS) {
	fsTestRun(t, makeFS, []fsTestGroup{
		{"OpenFile", testReadOnlyOpenFile},
		{"Open", testReadOnlyOpen},
		{"Mkdir", testReadOnlyMkdir},
		{"Rmdir", testReadOnlyRmdir},
		{"Unlink", testReadOnlyUnlink},
		{"Link", testReadOnlyLink},
		{"Symlink", testReadOnlySymlink},
		{"Readlink", testReadOnlyReadlink},
		{"Rename", testReadOnlyRename},
		{"Chmod", testReadOnlyChmod},
		{"Chtimes", testReadOnlyChtimes},
		{"Truncate", testReadOnlyTruncate},
		{"Stat", testReadOnlyStat},
		{"Lstat", testReadOnlyLstat},
	})

	t.Run("File", func(t *testing.T) {
		fsTestRun(t, makeFS, []fsTestGroup{
			{"Open", testReadOnlyFileOpen},
			{"OpenFile", testReadOnlyFileOpenFile},
			{"Read", testReadOnlyFileRead},
			{"Write", testReadOnlyFileWrite},
			{"Chmod", testReadOnlyFileChmod},
			{"Chtimes", testReadOnlyFileChtimes},
			{"Truncate", testReadOnlyFileTruncate},
			{"Sync", testReadOnlyFileSync},
			{"Datasync", testReadOnlyFileDatasync},
			{"Copy", testReadOnlyFileCopy},
		})
	})

	t.Run("fstest.TestFS", func(t *testing.T) { testFS(t, makeFS) })
}

// The test suites below contain tests validating the behavior of read-only
// file systems.

var testReadOnlyOpenFile = append(testDefaultOpenFile,
	fsTestCase{
		name: "opening a file with O_APPEND fails with ErrReadOnly",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: func(fsys sys.FS) error {
			_, err := fsys.OpenFile("test", sys.O_APPEND, 0)
			return err
		},
	},

	fsTestCase{
		name: "opening a file with O_CREATE fails with ErrReadOnly",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: func(fsys sys.FS) error {
			_, err := fsys.OpenFile("test", sys.O_CREATE, 0)
			return err
		},
	},

	fsTestCase{
		name: "opening a file with O_TRUNC fails with ErrReadOnly",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: func(fsys sys.FS) error {
			_, err := fsys.OpenFile("test", sys.O_TRUNC, 0)
			return err
		},
	},

	fsTestCase{
		name: "opening a file with O_WRONLY fails with ErrReadOnly",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: func(fsys sys.FS) error {
			_, err := fsys.OpenFile("test", sys.O_WRONLY, 0)
			return err
		},
	},
)

var testReadOnlyOpen = append(testDefaultOpen)

var testReadOnlyMkdir = append(testDefaultMkdir,
	fsTestCase{
		name: "creating a directory fails with ErrReadOnly",
		err:  sys.ErrReadOnly,
		test: func(fsys sys.FS) error { return fsys.Mkdir("test", 0755) },
	},
)

var testReadOnlyRmdir = append(testDefaultRmdir,
	fsTestCase{
		name: "removing a directory fails with ErrReadOnly",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0755 | fs.ModeDir}},
		err:  sys.ErrReadOnly,
		test: func(fsys sys.FS) error { return fsys.Rmdir("test") },
	},
)

var testReadOnlyUnlink = append(testDefaultUnlink,
	fsTestCase{
		name: "unlinking a file fails with ErrReadOnly",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: func(fsys sys.FS) error { return sys.Unlink(fsys, "test") },
	},
)

var testReadOnlyLink = append(testDefaultLink,
	fsTestCase{
		name: "linking a file fails with ErrReadOnly",
		base: fstest.MapFS{"old": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: func(fsys sys.FS) error { return fsys.Link("old", "new", fsys) },
	},
)

var testReadOnlySymlink = append(testDefaultSymlink,
	fsTestCase{
		name: "creating a symbolic link fails with ErrReadOnly",
		base: fstest.MapFS{"old": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: func(fsys sys.FS) error { return fsys.Symlink("old", "new") },
	},
)

var testReadOnlyReadlink = append(testDefaultReadlink)

var testReadOnlyRename = append(testDefaultRename,
	fsTestCase{
		name: "renaming a file fails with ErrReadOnly",
		base: fstest.MapFS{"old": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: func(fsys sys.FS) error { return fsys.Rename("old", "new", fsys) },
	},
)

var testReadOnlyChmod = append(testDefaultChmod,
	fsTestCase{
		name: "changing a file permissions fails with ErrReadOnly",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: func(fsys sys.FS) error { return sys.Chmod(fsys, "test", 0644) },
	},
)

var testReadOnlyChtimes = append(testDefaultChtimes,
	fsTestCase{
		name: "changing a file times fails with ErrReadOnly",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: func(fsys sys.FS) error { return sys.Chtimes(fsys, "test", epoch, epoch) },
	},
)

var testReadOnlyTruncate = append(testDefaultTruncate,
	fsTestCase{
		name: "truncating a file fails with ErrReadOnly",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: func(fsys sys.FS) error { return sys.Truncate(fsys, "test", 0) },
	},
)

var testReadOnlyStat = append(testDefaultStat)

var testReadOnlyLstat = append(testDefaultLstat)

var testReadOnlyFileOpen = append(testDefaultFileOpen)

var testReadOnlyFileOpenFile = append(testDefaultFileOpenFile)

var testReadOnlyFileRead = append(testDefaultFileRead)

var testReadOnlyFileWrite = append(testDefaultFileWrite)

var testReadOnlyFileChmod = append(testDefaultFileChmod,
	fsTestCase{
		name: "changing a file permissions fails with ErrReadOnly",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error {
			return f.Chmod(0600)
		}),
	},
)

var testReadOnlyFileChtimes = append(testDefaultFileChtimes,
	fsTestCase{
		name: "changing a file times fails with ErrReadOnly",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error {
			atime := epoch
			mtime := epoch
			return f.Chtimes(atime, mtime)
		}),
	},
)

var testReadOnlyFileTruncate = append(testDefaultFileTruncate,
	fsTestCase{
		name: "truncating a file fails with ErrReadOnly",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error {
			return f.Truncate(0)
		}),
	},
)

var testReadOnlyFileSync = append(testDefaultFileSync,
	fsTestCase{
		name: "syncing a file fails with ErrReadOnly",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error {
			return f.Sync()
		}),
	},
)

var testReadOnlyFileDatasync = append(testDefaultFileDatasync,
	fsTestCase{
		name: "datasyncing a file fails with ErrReadOnly",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrReadOnly,
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error {
			return f.Datasync()
		}),
	},
)

var testReadOnlyFileCopy = append(testDefaultFileCopy,
	fsTestCase{
		name: "copying from a file reads its content",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error {
			b := bytes.NewBuffer(nil)
			if _, err := io.Copy(struct{ io.Writer }{b}, f); err != nil {
				return err
			}
			if want, got := "hello", b.String(); want != got {
				return fmt.Errorf("file content mismatch: want=%q got=%q", want, got)
			}
			return nil
		}),
	},

	fsTestCase{
		name: "copying to a file files with ErrNotSupported",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		err:  sys.ErrNotSupported,
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error {
			r := strings.NewReader("nope")
			_, err := io.Copy(f, struct{ io.Reader }{r})
			return err
		}),
	},
)
