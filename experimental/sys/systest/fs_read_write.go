package systest

import (
	"fmt"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
)

// TestReadWriteFS is a test suite used to test the capabilities of file systems
// supporting both read and write operations (e.g. creating directories,
// writing files, etc...)
//
// The intent is for this test suite to help validate that read-write
// implementations of the sys.FS interface all exhibit the same behavior.
func TestReadWriteFS(t *testing.T, newFS NewFS) {
	makeFS := func(t *testing.T, baseFS fs.FS) sys.FS {
		testFS := newFS(t)
		readFS := sys.NewFS(baseFS)
		if err := sys.CopyFS(testFS, readFS); err != nil {
			t.Fatal(err)
		}
		if err := sys.EqualFS(testFS, baseFS); err != nil {
			t.Fatal(err)
		}
		return testFS
	}

	fsTestRun(t, makeFS, []fsTestGroup{
		{"OpenFile", testReadWriteOpenFile},
		{"Open", testReadWriteOpen},
		{"Mkdir", testReadWriteMkdir},
		{"Rmdir", testReadWriteRmdir},
		{"Unlink", testReadWriteUnlink},
		{"Link", testReadWriteLink},
		{"Symlink", testReadWriteSymlink},
		{"Readlink", testReadWriteReadlink},
		{"Rename", testReadWriteRename},
		{"Chmod", testReadWriteChmod},
		{"Chtimes", testReadWriteChtimes},
		{"Truncate", testReadWriteTruncate},
		{"Stat", testReadWriteStat},
	})

	t.Run("File", func(t *testing.T) {
		fsTestRun(t, makeFS, []fsTestGroup{
			{"Open", testReadWriteFileOpen},
			{"OpenFile", testReadWriteFileOpenFile},
			{"Read", testReadWriteFileRead},
			{"Write", testReadWriteFileWrite},
			{"Chmod", testReadWriteFileChmod},
			{"Chtimes", testReadWriteFileChtimes},
			{"Truncate", testReadWriteFileTruncate},
			{"Sync", testReadWriteFileSync},
			{"Datasync", testReadWriteFileDatasync},
			{"Copy", testReadWriteFileCopy},
		})
	})

	t.Run("fstest.TestFS", func(t *testing.T) { testFS(t, makeFS) })
}

// The test suites below contain tests validating the behavior of read-write
// file systems.

func testLoop(test func(sys.FS, string) error) func(sys.FS) error {
	return func(fsys sys.FS) error {
		const path = "root"
		const loop = "loop"
		if err := fsys.Symlink(path, loop); err != nil {
			return err
		}
		if err := fsys.Symlink(loop, path); err != nil {
			return err
		}
		return test(fsys, path)
	}
}

var testReadWriteOpenFile = append(testDefaultOpenFile,
	fsTestCase{
		name: "opening a file at a path containing a symbolic link loop fails with ErrLoop",
		err:  sys.ErrLoop,
		test: testLoop(func(fsys sys.FS, path string) error {
			_, err := fsys.OpenFile(path+"/test", sys.O_RDONLY, 0)
			return err
		}),
	},

	fsTestCase{
		name: "files can be created when opened with O_CREATE",
		base: fstest.MapFS{
			"one": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
		},
		want: fstest.MapFS{
			"one": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
			"two": &fstest.MapFile{Mode: 0600, Data: []byte("2")},
		},
		test: func(fsys sys.FS) error {
			f, err := fsys.OpenFile("two", sys.O_CREATE|sys.O_RDWR, 0600)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.WriteString(f, "2")
			return err
		},
	},

	fsTestCase{
		name: "files are truncated to a zero size when opened with O_TRUNC",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("1")}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		test: func(fsys sys.FS) error {
			f, err := fsys.OpenFile("test", sys.O_TRUNC|sys.O_RDWR, 0600)
			if err != nil {
				return err
			}
			defer f.Close()
			return nil
		},
	},

	fsTestCase{
		name: "opening an existing file with O_CREATE and O_EXCL fails with ErrExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrExist,
		test: func(fsys sys.FS) error {
			_, err := fsys.OpenFile("test", sys.O_CREATE|sys.O_EXCL|sys.O_RDWR, 0600)
			return err
		},
	},
)

var testReadWriteOpen = append(testDefaultOpen,
	fsTestCase{
		name: "opening a file at a path containing a symbolic link loop fails with ErrLoop",
		err:  sys.ErrLoop,
		test: testLoop(func(fsys sys.FS, path string) error {
			_, err := fsys.Open(path + "/test")
			return err
		}),
	},
)

var testReadWriteMkdir = append(testDefaultMkdir,
	fsTestCase{
		name: "creating a directory at a path containing a symbolic link loop fails with ErrLoop",
		err:  sys.ErrLoop,
		test: testLoop(func(fsys sys.FS, path string) error { return fsys.Mkdir(path+"/test", 0755) }),
	},

	fsTestCase{
		name: "directories can be created at the root of the file system",
		want: fstest.MapFS{
			"test": &fstest.MapFile{Mode: 0755 | fs.ModeDir},
		},
		test: func(fsys sys.FS) error { return fsys.Mkdir("test", 0755) },
	},

	fsTestCase{
		name: "directories can be created in sub-directories of the file system",
		base: fstest.MapFS{
			"top": &fstest.MapFile{Mode: 0755 | fs.ModeDir},
		},
		want: fstest.MapFS{
			"top":     &fstest.MapFile{Mode: 0755 | fs.ModeDir},
			"top/sub": &fstest.MapFile{Mode: 0700 | fs.ModeDir},
		},
		test: func(fsys sys.FS) error { return fsys.Mkdir("top/sub", 0700) },
	},

	fsTestCase{
		name: "creating a directory at a location where a directory exists fails with ErrExist",
		base: fstest.MapFS{
			"top": &fstest.MapFile{Mode: 0755 | fs.ModeDir},
		},
		err:  sys.ErrExist,
		test: func(fsys sys.FS) error { return fsys.Mkdir("top", 0755) },
	},

	fsTestCase{
		name: "creating a directory at a location where a file exists fails with ErrExist",
		base: fstest.MapFS{
			"test": &fstest.MapFile{Mode: 0644},
		},
		err:  sys.ErrExist,
		test: func(fsys sys.FS) error { return fsys.Mkdir("test", 0755) },
	},

	fsTestCase{
		name: "creating a directory at a location which does not exist fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return fsys.Mkdir("top/sub", 0755) },
	},
)

var testReadWriteRmdir = append(testDefaultRmdir,
	fsTestCase{
		name: "removing a directory at a path containing a symbolic link loop fails with ErrLoop",
		err:  sys.ErrLoop,
		test: testLoop(func(fsys sys.FS, path string) error { return fsys.Rmdir(path + "/test") }),
	},

	fsTestCase{
		name: "removing a directory at a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0755 | fs.ModeDir}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0755 | fs.ModeDir}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return fsys.Rmdir("nope") },
	},

	fsTestCase{
		name: "removing a directory at a location where a file exists fails with ErrNotDirectory",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotDirectory,
		test: func(fsys sys.FS) error { return fsys.Rmdir("test") },
	},

	fsTestCase{
		name: "removing a non-empty directory fails with ErrNotEmpty",
		base: fstest.MapFS{
			"dir":      &fstest.MapFile{Mode: 0755 | fs.ModeDir},
			"dir/file": &fstest.MapFile{Mode: 0644},
		},
		want: fstest.MapFS{
			"dir":      &fstest.MapFile{Mode: 0755 | fs.ModeDir},
			"dir/file": &fstest.MapFile{Mode: 0644},
		},
		err:  sys.ErrNotEmpty,
		test: func(fsys sys.FS) error { return fsys.Rmdir("dir") },
	},

	fsTestCase{
		name: "empty directories can be removed from the file system",
		base: fstest.MapFS{
			"dir-1": &fstest.MapFile{Mode: 0700 | fs.ModeDir},
			"dir-2": &fstest.MapFile{Mode: 0700 | fs.ModeDir},
			"dir-3": &fstest.MapFile{Mode: 0700 | fs.ModeDir},
		},
		want: fstest.MapFS{
			"dir-1": &fstest.MapFile{Mode: 0700 | fs.ModeDir},
			"dir-3": &fstest.MapFile{Mode: 0700 | fs.ModeDir},
		},
		test: func(fsys sys.FS) error { return fsys.Rmdir("dir-2") },
	},

	fsTestCase{
		name: "empty sub-directories can be removed from the file system",
		base: fstest.MapFS{
			"top":     &fstest.MapFile{Mode: 0700 | fs.ModeDir},
			"top/sub": &fstest.MapFile{Mode: 0700 | fs.ModeDir},
		},
		want: fstest.MapFS{
			"top": &fstest.MapFile{Mode: 0700 | fs.ModeDir},
		},
		test: func(fsys sys.FS) error { return fsys.Rmdir("top/sub") },
	},
)

var testReadWriteUnlink = append(testDefaultUnlink,
	fsTestCase{
		name: "unlinking a file at a path containing a symbolic link loop fails with ErrLoop",
		err:  sys.ErrLoop,
		test: testLoop(func(fsys sys.FS, path string) error { return fsys.Unlink(path + "/test") }),
	},

	fsTestCase{
		name: "unlinking a file at a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return fsys.Unlink("nope") },
	},

	fsTestCase{
		name: "unlinking a file at a location where a directory exists fails with ErrPermission",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0755 | fs.ModeDir}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0755 | fs.ModeDir}},
		err:  sys.ErrPermission,
		test: func(fsys sys.FS) error { return fsys.Unlink("test") },
	},

	fsTestCase{
		name: "existing files can be removed from the file system",
		base: fstest.MapFS{
			"file-1": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
			"file-2": &fstest.MapFile{Mode: 0644, Data: []byte("2")},
			"file-3": &fstest.MapFile{Mode: 0644, Data: []byte("3")},
		},
		want: fstest.MapFS{
			"file-1": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
			"file-3": &fstest.MapFile{Mode: 0644, Data: []byte("3")},
		},
		test: func(fsys sys.FS) error { return fsys.Unlink("file-2") },
	},
)

var testReadWriteLink = append(testDefaultLink,
	fsTestCase{
		name: "linking files at a path containing a symbolic link loop fails with ErrLoop",
		err:  sys.ErrLoop,
		test: testLoop(func(fsys sys.FS, path string) error {
			oldName := path + "/old"
			newName := path + "/new"
			return fsys.Link(oldName, newName, fsys)
		}),
	},

	fsTestCase{
		name: "linking with a source location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"source": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"source": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return fsys.Link("nope", "target", fsys) },
	},

	fsTestCase{
		name: "linking with a target location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"source": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"source": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return fsys.Link("source", "dir/nope", fsys) },
	},

	fsTestCase{
		name: "linking a file to its own location fails with ErrExist",
		base: fstest.MapFS{"source": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"source": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrExist,
		test: func(fsys sys.FS) error { return fsys.Link("source", "source", fsys) },
	},

	fsTestCase{
		name: "linking a file to a location where a file already exists fails with ErrExist",
		base: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
			"target": &fstest.MapFile{Mode: 0644, Data: []byte("2")},
		},
		want: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
			"target": &fstest.MapFile{Mode: 0644, Data: []byte("2")},
		},
		err:  sys.ErrExist,
		test: func(fsys sys.FS) error { return fsys.Link("source", "target", fsys) },
	},

	fsTestCase{
		name: "linking a file to a location where a directory already exists fails with ErrExist",
		base: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
			"target": &fstest.MapFile{Mode: 0755 | fs.ModeDir},
		},
		want: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
			"target": &fstest.MapFile{Mode: 0755 | fs.ModeDir},
		},
		err:  sys.ErrExist,
		test: func(fsys sys.FS) error { return fsys.Link("source", "target", fsys) },
	},

	fsTestCase{
		name: "linking a file creates another entry for it on the file system",
		base: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
		},
		want: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
			"target": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
		},
		test: func(fsys sys.FS) error { return fsys.Link("source", "target", fsys) },
	},

	fsTestCase{
		name: "writes to a file are reflected at all linked locations",
		base: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
		},
		want: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("2")},
			"target": &fstest.MapFile{Mode: 0644, Data: []byte("2")},
		},
		test: func(fsys sys.FS) error {
			if err := fsys.Link("source", "target", fsys); err != nil {
				return err
			}
			return writeFile(fsys, "source", []byte("2"))
		},
	},
)

var testReadWriteSymlink = append(testDefaultSymlink,
	fsTestCase{
		name: "linking files at a path containing a symbolic link loop fails with ErrLoop",
		err:  sys.ErrLoop,
		test: testLoop(func(fsys sys.FS, path string) error {
			oldName := path + "/old"
			newName := path + "/new"
			return fsys.Symlink(oldName, newName)
		}),
	},

	fsTestCase{
		name: "linking with a target location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"source": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"source": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return fsys.Symlink("source", "dir/nope") },
	},

	fsTestCase{
		name: "linking a file to its own location fails with ErrExist",
		base: fstest.MapFS{"source": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"source": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrExist,
		test: func(fsys sys.FS) error { return fsys.Symlink("source", "source") },
	},

	fsTestCase{
		name: "linking a file to a location where a file already exists fails with ErrExist",
		base: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
			"target": &fstest.MapFile{Mode: 0644, Data: []byte("2")},
		},
		want: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
			"target": &fstest.MapFile{Mode: 0644, Data: []byte("2")},
		},
		err:  sys.ErrExist,
		test: func(fsys sys.FS) error { return fsys.Symlink("source", "target") },
	},

	fsTestCase{
		name: "linking a file to a location where a directory already exists fails with ErrExist",
		base: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
			"target": &fstest.MapFile{Mode: 0755 | fs.ModeDir},
		},
		want: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
			"target": &fstest.MapFile{Mode: 0755 | fs.ModeDir},
		},
		err:  sys.ErrExist,
		test: func(fsys sys.FS) error { return fsys.Symlink("source", "target") },
	},

	fsTestCase{
		name: "linking a file creates another entry for it on the file system",
		base: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
		},
		want: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
			"target": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
		},
		test: func(fsys sys.FS) error { return fsys.Symlink("source", "target") },
	},

	fsTestCase{
		name: "writes to a file are reflected at all linked locations",
		base: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
		},
		want: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("2")},
			"target": &fstest.MapFile{Mode: 0644, Data: []byte("2")},
		},
		test: func(fsys sys.FS) error {
			if err := fsys.Symlink("source", "target"); err != nil {
				return err
			}
			return writeFile(fsys, "source", []byte("2"))
		},
	},

	fsTestCase{
		name: "linking with a source location which does not exist creates a broken link",
		base: fstest.MapFS{"source": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"source": &fstest.MapFile{Mode: 0644}},
		test: func(fsys sys.FS) error { return fsys.Symlink("nope", "target") },
	},
)

var testReadWriteReadlink = append(testDefaultReadlink,
	fsTestCase{
		name: "reading a link at a path containing a symbolic link loop fails with ErrLoop",
		err:  sys.ErrLoop,
		test: testLoop(func(fsys sys.FS, path string) error {
			_, err := sys.Readlink(fsys, path+"/test")
			return err
		}),
	},

	fsTestCase{
		name: "reading a link at a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := sys.Readlink(fsys, "nope")
			return err
		},
	},

	fsTestCase{
		name: "reading a link at a location where a file exists fails with ErrInvalid",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrInvalid,
		test: func(fsys sys.FS) error {
			_, err := sys.Readlink(fsys, "test")
			return err
		},
	},

	fsTestCase{
		name: "reading a link at a location where a directory exists fails with ErrInvalid",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0755 | fs.ModeDir}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0755 | fs.ModeDir}},
		err:  sys.ErrInvalid,
		test: func(fsys sys.FS) error {
			_, err := sys.Readlink(fsys, "test")
			return err
		},
	},

	fsTestCase{
		name: "reading a link returns the path to the link target",
		base: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
		},
		want: fstest.MapFS{
			"source": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
			"target": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
		},
		test: func(fsys sys.FS) error {
			const source = "./source" // preserve relative location
			const target = "target"
			if err := fsys.Symlink(source, target); err != nil {
				return err
			}
			s, err := sys.Readlink(fsys, target)
			if err != nil {
				return err
			}
			if s != source {
				return fmt.Errorf("link mismatch: want=%q got=%q", source, s)
			}
			return nil
		},
	},
)

var testReadWriteRename = append(testDefaultRename,
	fsTestCase{
		name: "moving a file from a path containing a symbolic link loop fails with ErrLoop",
		err:  sys.ErrLoop,
		test: testLoop(func(fsys sys.FS, path string) error {
			oldName := path + "/old"
			newName := "new"
			return fsys.Rename(oldName, newName, fsys)
		}),
	},

	fsTestCase{
		name: "moving a file to a path containing a symbolic link loop fails with ErrLoop",
		base: fstest.MapFS{"old": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrLoop,
		test: testLoop(func(fsys sys.FS, path string) error {
			oldName := "old"
			newName := path + "/new"
			return fsys.Rename(oldName, newName, fsys)
		}),
	},

	fsTestCase{
		name: "moving a file to a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return fsys.Rename("old", "dir/nope", fsys) },
	},

	fsTestCase{
		name: "moving a file from a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return fsys.Rename("old", "new", fsys) },
	},

	fsTestCase{
		name: "moving a file to a a new location modifies the file system",
		base: fstest.MapFS{"old": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		want: fstest.MapFS{"new": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		test: func(fsys sys.FS) error { return fsys.Rename("old", "new", fsys) },
	},

	fsTestCase{
		name: "moving a file to its own location does not modify the file system",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		test: func(fsys sys.FS) error { return fsys.Rename("test", "test", fsys) },
	},

	fsTestCase{
		name: "moving a file to a location where a file already exists unlinks it",
		base: fstest.MapFS{
			"old": &fstest.MapFile{Mode: 0644, Data: []byte("hello")},
			"new": &fstest.MapFile{Mode: 0644, Data: []byte("world")},
		},
		want: fstest.MapFS{
			"new": &fstest.MapFile{Mode: 0644, Data: []byte("hello")},
		},
		test: func(fsys sys.FS) error { return fsys.Rename("old", "new", fsys) },
	},

	fsTestCase{
		name: "moving a file to a location where a directory already exists fails with ErrExist",
		base: fstest.MapFS{
			"old": &fstest.MapFile{Mode: 0644, Data: []byte("hello")},
			"new": &fstest.MapFile{Mode: 0755 | fs.ModeDir},
		},
		want: fstest.MapFS{
			"old": &fstest.MapFile{Mode: 0644, Data: []byte("hello")},
			"new": &fstest.MapFile{Mode: 0755 | fs.ModeDir},
		},
		err:  sys.ErrExist,
		test: func(fsys sys.FS) error { return fsys.Rename("old", "new", fsys) },
	},
)

var testReadWriteChmod = append(testDefaultChmod,
	fsTestCase{
		name: "changing file permissions at a path containing a symbolic link loop fails with ErrLoop",
		err:  sys.ErrLoop,
		test: testLoop(func(fsys sys.FS, path string) error { return fsys.Chmod(path+"/test", 0600) }),
	},

	fsTestCase{
		name: "changing file permissions at a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return fsys.Chmod("nope", 0) },
	},

	fsTestCase{
		name: "chaging file permissions of an existing file",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0600}},
		test: func(fsys sys.FS) error { return fsys.Chmod("test", 0600) },
	},
)

var testReadWriteChtimes = append(testDefaultChtimes,
	fsTestCase{
		name: "changing file times at a path containing a symbolic link loop fails with ErrLoop",
		err:  sys.ErrLoop,
		test: testLoop(func(fsys sys.FS, path string) error {
			atime := time.Time{}
			mtime := time.Time{}
			return fsys.Chtimes(path+"/test", atime, mtime)
		}),
	},

	fsTestCase{
		name: "changing file times at a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return fsys.Chtimes("nope", epoch, epoch) },
	},

	fsTestCase{
		name: "chaging file times of an existing file",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, ModTime: epoch}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, ModTime: epoch.Add(-time.Second)}},
		test: func(fsys sys.FS) error {
			atime := time.Time{}
			mtime := epoch.Add(-time.Second)
			return fsys.Chtimes("test", atime, mtime)
		},
	},
)

var testReadWriteTruncate = append(testDefaultTruncate,
	fsTestCase{
		name: "truncating a file at a path containing a symbolic link loop fails with ErrLoop",
		err:  sys.ErrLoop,
		test: testLoop(func(fsys sys.FS, path string) error { return fsys.Truncate(path+"/test", 0) }),
	},

	fsTestCase{
		name: "truncating a file to a negative size fails with ErrInvalid",
		base: fstest.MapFS{
			"test": &fstest.MapFile{Mode: 0644, Data: []byte("123")},
		},
		err:  sys.ErrInvalid,
		test: func(fsys sys.FS) error { return fsys.Truncate("test", -1) },
	},

	fsTestCase{
		name: "truncating a file to less than its size erases its content",
		base: fstest.MapFS{
			"test": &fstest.MapFile{Mode: 0644, Data: []byte("123")},
		},
		want: fstest.MapFS{
			"test": &fstest.MapFile{Mode: 0644, Data: []byte("1")},
		},
		test: func(fsys sys.FS) error { return fsys.Truncate("test", 1) },
	},

	fsTestCase{
		name: "truncating a file to more than its size adds trailing zeros",
		base: fstest.MapFS{
			"test": &fstest.MapFile{Mode: 0644, Data: []byte("123")},
		},
		want: fstest.MapFS{
			"test": &fstest.MapFile{Mode: 0644, Data: []byte("123\x00\x00\x00")},
		},
		test: func(fsys sys.FS) error { return fsys.Truncate("test", 6) },
	},
)

var testReadWriteStat = append(testDefaultStat,
	fsTestCase{
		name: "stat at a path containing a symbolic link loop fails with ErrLoop",
		err:  sys.ErrLoop,
		test: testLoop(func(fsys sys.FS, path string) error {
			_, err := fsys.Stat(path + "/test")
			return err
		}),
	},

	fsTestCase{
		name: "stat on a symolic link returns information about the target file",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0600}},
		test: func(fsys sys.FS) error {
			if err := fsys.Symlink("test", "link"); err != nil {
				return err
			}
			s, err := fsys.Stat("link")
			if err != nil {
				return err
			}
			if mode := s.Mode(); mode.Type() != 0 {
				return fmt.Errorf("wrong mode: %s", mode)
			}
			return nil
		},
	},
)

var testReadWriteFileOpen = append(testDefaultFileOpen)

var testReadWriteFileOpenFile = append(testDefaultFileOpenFile)

var testReadWriteFileRead = append(testDefaultFileRead,
	fsTestCase{
		name: "reading from a file open with O_WRONLY fails with ErrNotSupported",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotSupported,
		test: testOpenFile("test", sys.O_WRONLY, 0, func(f sys.File) error {
			_, err := f.Read(make([]byte, 1))
			return err
		}),
	},
)

var testReadWriteFileWrite = append(testDefaultFileWrite,
	fsTestCase{
		name: "writing bytes to a file changes its content",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		test: testOpenFile("test", sys.O_WRONLY, 0, func(f sys.File) error {
			_, err := f.Write([]byte("hello"))
			return err
		}),
	},

	fsTestCase{
		name: "writing a string to a file changes its content",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		test: testOpenFile("test", sys.O_WRONLY, 0, func(f sys.File) error {
			_, err := io.WriteString(f, "hello")
			return err
		}),
	},
)

var testReadWriteFileChmod = append(testDefaultFileChmod,
	fsTestCase{
		name: "change file permissions of an open file",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0600}},
		test: testOpenFile("test", sys.O_RDWR, 0, func(f sys.File) error {
			return f.Chmod(0600)
		}),
	},
)

var testReadWriteFileChtimes = append(testDefaultFileChtimes,
	fsTestCase{
		name: "change file times of an open file",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, ModTime: epoch}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, ModTime: epoch.Add(time.Second)}},
		test: testOpenFile("test", sys.O_RDWR, 0, func(f sys.File) error {
			atime := time.Time{}
			mtime := epoch.Add(time.Second)
			return f.Chtimes(atime, mtime)
		}),
	},
)

var testReadWriteFileTruncate = append(testDefaultFileTruncate,
	fsTestCase{
		name: "truncating a file to less than its size erases its content",
		base: fstest.MapFS{
			"test": &fstest.MapFile{
				Data: []byte("123"),
				Mode: 0644,
			},
		},
		want: fstest.MapFS{
			"test": &fstest.MapFile{
				Data: []byte("1"),
				Mode: 0644,
			},
		},
		test: testOpenFile("test", sys.O_RDWR, 0, func(f sys.File) error {
			return f.Truncate(1)
		}),
	},

	fsTestCase{
		name: "truncating a file to more than its size adds trailing zeros",
		base: fstest.MapFS{
			"test": &fstest.MapFile{
				Data: []byte("123"),
				Mode: 0644,
			},
		},
		want: fstest.MapFS{
			"test": &fstest.MapFile{
				Data: []byte("123\x00\x00\x00"),
				Mode: 0644,
			},
		},
		test: testOpenFile("test", sys.O_RDWR, 0, func(f sys.File) error {
			return f.Truncate(6)
		}),
	},
)

var testReadWriteFileSync = append(testDefaultFileSync,
	fsTestCase{
		name: "syncing a file flushes buffered mutations to persistent storage",
		base: fstest.MapFS{},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		test: testOpenFile("test", sys.O_CREATE|sys.O_WRONLY, 0644, func(f sys.File) error {
			if _, err := io.WriteString(f, "hello"); err != nil {
				return err
			}
			return f.Sync()
		}),
	},
)

var testReadWriteFileDatasync = append(testDefaultFileDatasync,
	fsTestCase{
		name: "syncing a file data flushes buffered writes to persistent storage",
		base: fstest.MapFS{},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		test: testOpenFile("test", sys.O_CREATE|sys.O_WRONLY, 0644, func(f sys.File) error {
			if _, err := io.WriteString(f, "hello"); err != nil {
				return err
			}
			return f.Datasync()
		}),
	},
)

func testCopy(test func(r, w sys.File) error) func(sys.FS) error {
	return func(fsys sys.FS) error {
		r, err := fsys.OpenFile("source", sys.O_RDONLY, 0)
		if err != nil {
			return err
		}
		defer r.Close()
		w, err := fsys.OpenFile("target", sys.O_CREATE|sys.O_TRUNC|sys.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer w.Close()
		return test(w, r)
	}
}

var testReadWriteFileCopy = append(testDefaultFileCopy,
	fsTestCase{
		name: "copying from a closed file fails with ErrClosed",
		base: fstest.MapFS{
			"source": &fstest.MapFile{
				Data: []byte(`Hello World!`),
				Mode: 0644,
			},
		},
		want: fstest.MapFS{
			"source": &fstest.MapFile{
				Data: []byte(`Hello World!`),
				Mode: 0644,
			},
			"target": &fstest.MapFile{
				Mode: 0644,
			},
		},
		err: sys.ErrClosed,
		test: testCopy(func(w, r sys.File) error {
			if err := r.Close(); err != nil {
				return err
			}
			_, err := io.Copy(w, r)
			return err
		}),
	},

	fsTestCase{
		name: "copying to a closed file fails with ErrClosed",
		base: fstest.MapFS{
			"source": &fstest.MapFile{
				Data: []byte(`Hello World!`),
				Mode: 0644,
			},
		},
		want: fstest.MapFS{
			"source": &fstest.MapFile{
				Data: []byte(`Hello World!`),
				Mode: 0644,
			},
			"target": &fstest.MapFile{
				Mode: 0644,
			},
		},
		err: sys.ErrClosed,
		test: testCopy(func(w, r sys.File) error {
			if err := w.Close(); err != nil {
				return err
			}
			_, err := io.Copy(w, r)
			return err
		}),
	},

	fsTestCase{
		name: "files can be copied with io.Copy",
		base: fstest.MapFS{
			"source": &fstest.MapFile{
				Data: []byte(`Hello World!`),
				Mode: 0644,
			},
		},
		want: fstest.MapFS{
			"source": &fstest.MapFile{
				Data: []byte(`Hello World!`),
				Mode: 0644,
			},
			"target": &fstest.MapFile{
				Data: []byte(`Hello World!`),
				Mode: 0644,
			},
		},
		test: testCopy(func(w, r sys.File) error {
			_, err := io.Copy(w, r)
			return err
		}),
	},

	fsTestCase{
		name: "files can be copied when the source is an io.Reader",
		base: fstest.MapFS{
			"source": &fstest.MapFile{
				Data: []byte(`Hello World!`),
				Mode: 0644,
			},
		},
		want: fstest.MapFS{
			"source": &fstest.MapFile{
				Data: []byte(`Hello World!`),
				Mode: 0644,
			},
			"target": &fstest.MapFile{
				Data: []byte(`Hello World!`),
				Mode: 0644,
			},
		},
		test: testCopy(func(w, r sys.File) error {
			_, err := io.Copy(w, struct{ io.Reader }{r})
			return err
		}),
	},

	fsTestCase{
		name: "files can be copied when the destination is an io.Writer",
		base: fstest.MapFS{
			"source": &fstest.MapFile{
				Data: []byte(`Hello World!`),
				Mode: 0644,
			},
		},
		want: fstest.MapFS{
			"source": &fstest.MapFile{
				Data: []byte(`Hello World!`),
				Mode: 0644,
			},
			"target": &fstest.MapFile{
				Data: []byte(`Hello World!`),
				Mode: 0644,
			},
		},
		test: testCopy(func(w, r sys.File) error {
			_, err := io.Copy(struct{ io.Writer }{w}, r)
			return err
		}),
	},
)

func writeFile(fsys sys.FS, name string, data []byte) error {
	f, err := fsys.OpenFile(name, sys.O_CREATE|sys.O_TRUNC|sys.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Close()
}
