package systest

import (
	"io/fs"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
)

// TestErrorFS is a test suite used to ensure that calling methods of a FS
// instsance return a specific error.
//
// Note that the FS implementation is still expected to behavior like a file
// system. For example, it must validate its inputs and return errors if they
// are invalid, then only fallback to the desired error.
func TestErrorFS(t *testing.T, want error, newFS NewFS) {
	openFile := append(testValidateOpenFile, fsTestCase{
		name: "opening a file errors",
		err:  want,
		test: func(fsys sys.FS) error {
			_, err := fsys.OpenFile("test", sys.O_RDONLY, 0)
			return err
		},
	})

	open := append(testValidateOpen, fsTestCase{
		name: "opening a file errors",
		err:  want,
		test: func(fsys sys.FS) error {
			_, err := fsys.Open("test")
			return err
		},
	})

	mkdir := append(testValidateMkdir, fsTestCase{
		name: "creating a directory errors",
		err:  want,
		test: func(fsys sys.FS) error {
			return sys.Mkdir(fsys, "test", 0755)
		},
	})

	rmdir := append(testValidateRmdir, fsTestCase{
		name: "removing a directory errors",
		err:  want,
		test: func(fsys sys.FS) error {
			return sys.Rmdir(fsys, "test")
		},
	})

	unlink := append(testValidateUnlink, fsTestCase{
		name: "unlinking a file errors",
		err:  want,
		test: func(fsys sys.FS) error {
			return sys.Unlink(fsys, "test")
		},
	})

	symlink := append(testValidateSymlink, fsTestCase{
		name: "creating a symbolic link errors",
		err:  want,
		test: func(fsys sys.FS) error {
			return sys.Symlink(fsys, "old", "new")
		},
	})

	readlink := append(testValidateUnlink, fsTestCase{
		name: "reading a symbolic link errors",
		err:  want,
		test: func(fsys sys.FS) error {
			_, err := sys.Readlink(fsys, "test")
			return err
		},
	})

	link := append(testValidateLink, fsTestCase{
		name: "linking a file errors",
		err:  want,
		test: func(fsys sys.FS) error {
			return sys.Link(fsys, "old", "new")
		},
	})

	rename := append(testValidateRename, fsTestCase{
		name: "renaming a file errors",
		err:  want,
		test: func(fsys sys.FS) error {
			return sys.Rename(fsys, "old", "new")
		},
	})

	chmod := append(testValidateChmod, fsTestCase{
		name: "changing file permissions errors",
		err:  want,
		test: func(fsys sys.FS) error {
			return sys.Chmod(fsys, "test", 0644)
		},
	})

	chtimes := append(testValidateChtimes, fsTestCase{
		name: "changing file times errors",
		err:  want,
		test: func(fsys sys.FS) error {
			return sys.Chtimes(fsys, "test", epoch, epoch)
		},
	})

	truncate := append(testValidateTruncate, fsTestCase{
		name: "changing file permissions errors",
		err:  want,
		test: func(fsys sys.FS) error {
			return sys.Truncate(fsys, "test", 0)
		},
	})

	stat := append(testValidateStat, fsTestCase{
		name: "stating a file errors",
		err:  want,
		test: func(fsys sys.FS) error {
			_, err := sys.Stat(fsys, "test")
			return err
		},
	})

	lstat := append(testValidateStat, fsTestCase{
		name: "stating a link errors",
		err:  want,
		test: func(fsys sys.FS) error {
			_, err := sys.Lstat(fsys, "test")
			return err
		},
	})

	makeFS := func(t *testing.T, _ fs.FS) sys.FS {
		return newFS(t)
	}

	fsTestRun(t, makeFS, []fsTestGroup{
		{"OpenFile", openFile},
		{"Open", open},
		{"Mkdir", mkdir},
		{"Rmdir", rmdir},
		{"Unlink", unlink},
		{"Link", link},
		{"Symlink", symlink},
		{"Readlink", readlink},
		{"Rename", rename},
		{"Chmod", chmod},
		{"Chtimes", chtimes},
		{"Truncate", truncate},
		{"Stat", stat},
		{"Lstat", lstat},
	})
}
