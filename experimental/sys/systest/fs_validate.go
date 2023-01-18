package systest

import (
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"sort"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
)

// The following test suites contain tests verifying input validation for
// implementations of the sys.FS interface.

var testValidateOpenFile = fsTestSuite{
	{
		name: "opening an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := fsys.OpenFile("/", sys.O_RDONLY|sys.O_DIRECTORY, 0)
			return err
		},
	},

	{
		name: "opening a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := fsys.OpenFile("nope", sys.O_RDONLY, 0)
			return err
		},
	},

	{
		name: "opening a file with O_DIRECTORY fails with ErrNotDirectory",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotDirectory,
		test: func(fsys sys.FS) error {
			_, err := fsys.OpenFile("test", sys.O_RDONLY|sys.O_DIRECTORY, 0)
			return err
		},
	},
}

var testValidateOpen = fsTestSuite{
	{
		name: "opening an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := fsys.Open("/")
			return err
		},
	},

	{
		name: "opening a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := fsys.Open("nope")
			return err
		},
	},

	{
		name: "existing files can be open",
		base: fstest.MapFS{
			"file-0": &fstest.MapFile{Mode: 0644, Data: []byte("A")},
			"file-1": &fstest.MapFile{Mode: 0644, Data: []byte("B")},
			"file-2": &fstest.MapFile{Mode: 0644, Data: []byte("C")},
		},
		test: func(fsys sys.FS) error {
			f, err := fsys.Open("file-2")
			if err != nil {
				return err
			}
			return f.Close()
		},
	},

	{
		name: "existing directories can be open",
		base: fstest.MapFS{
			"test":      &fstest.MapFile{Mode: 0755 | fs.ModeDir},
			"test/file": &fstest.MapFile{Mode: 0644, Data: []byte("test")},
		},
		test: func(fsys sys.FS) error {
			d, err := fsys.Open("test")
			if err != nil {
				return err
			}
			return d.Close()
		},
	},

	{
		name: "existing files can be read",
		base: fstest.MapFS{
			"file-0": &fstest.MapFile{Mode: 0644, Data: []byte("A")},
			"file-1": &fstest.MapFile{Mode: 0644, Data: []byte("B")},
			"file-2": &fstest.MapFile{Mode: 0644, Data: []byte("C")},
		},
		test: func(fsys sys.FS) error {
			b, err := fs.ReadFile(fsys, "file-1")
			if err != nil {
				return err
			}
			if string(b) != "B" {
				return fmt.Errorf("wrong file data: want=%q got=%q", "B", b)
			}
			return nil
		},
	},

	{
		name: "existing directories can be read",
		base: fstest.MapFS{
			"test":        &fstest.MapFile{Mode: 0755 | fs.ModeDir},
			"test/file-0": &fstest.MapFile{Mode: 0644, Data: []byte("A")},
			"test/file-1": &fstest.MapFile{Mode: 0644, Data: []byte("B")},
			"test/file-2": &fstest.MapFile{Mode: 0644, Data: []byte("C")},
		},
		test: func(fsys sys.FS) error {
			names := []string{}

			err := fs.WalkDir(fsys, "test", func(path string, _ fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				names = append(names, path)
				return nil
			})
			if err != nil {
				return err
			}

			sort.Strings(names)
			want := []string{
				"test",
				"test/file-0",
				"test/file-1",
				"test/file-2",
			}

			if !reflect.DeepEqual(names, want) {
				return fmt.Errorf("wrong directory entries: want=%q got=%q", want, names)
			}
			return nil
		},
	},
}

var testValidateAccess = fsTestSuite{
	{
		name: "accessing a file with an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Access(fsys, "/", sys.O_RDONLY) },
	},

	{
		name: "existing files can be tested for existance",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0700}},
		test: func(fsys sys.FS) error { return sys.Access(fsys, "test", sys.F_OK) },
	},

	{
		name: "existing files can be accessed in execution mode",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0700}},
		test: func(fsys sys.FS) error { return sys.Access(fsys, "test", sys.X_OK) },
	},

	{
		name: "existing files can be accessed in write mode",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0700}},
		test: func(fsys sys.FS) error { return sys.Access(fsys, "test", sys.W_OK) },
	},

	{
		name: "existing files can be accessed in read mode",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0700}},
		test: func(fsys sys.FS) error { return sys.Access(fsys, "test", sys.R_OK) },
	},
}

var testValidateMknod = fsTestSuite{
	{
		name: "creating a directory with an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Mknod(fsys, "/", 0600, sys.Dev(0, 0)) },
	},
}

var testValidateMkdir = fsTestSuite{
	{
		name: "creating a node with an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Mkdir(fsys, "/", 0755) },
	},
}

var testValidateRmdir = fsTestSuite{
	{
		name: "removing a directory with an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Rmdir(fsys, "/") },
	},
}

var testValidateUnlink = fsTestSuite{
	{
		name: "unlinking a file with an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Unlink(fsys, "/") },
	},
}

var testValidateLink = fsTestSuite{
	{
		name: "linking a file with an invalid source name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Link(fsys, "/", "new") },
	},
	{
		name: "linking a file with an invalid target name fails with ErrInvalid",
		err:  sys.ErrInvalid,
		test: func(fsys sys.FS) error { return sys.Link(fsys, "old", "/") },
	},
}

var testValidateSymlink = fsTestSuite{
	{
		name: "creating a symbolic link with an invalid target name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Symlink(fsys, "old", "/") },
	},
}

var testValidateReadlink = fsTestSuite{
	{
		name: "reading a symbolic link with an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := sys.Readlink(fsys, "/")
			return err
		},
	},
}

var testValidateRename = fsTestSuite{
	{
		name: "renaming a file with an invalid source name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Rename(fsys, "/", "new") },
	},
	{
		name: "renaming a file with an invalid target name fails with ErrInvalid",
		err:  sys.ErrInvalid,
		test: func(fsys sys.FS) error { return sys.Rename(fsys, "old", "/") },
	},
}

var testValidateChmod = fsTestSuite{
	{
		name: "changing permissions of a file with and invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Chmod(fsys, "/", 0644) },
	},
}

var testValidateChtimes = fsTestSuite{
	{
		name: "changing times of a file with and invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Chtimes(fsys, "/", epoch, epoch) },
	},
}

var testValidateTruncate = fsTestSuite{
	{
		name: "truncating a file with and invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return sys.Truncate(fsys, "/", 0) },
	},
}

var testValidateStat = fsTestSuite{
	{
		name: "stat of a file with and invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := sys.Stat(fsys, "/")
			return err
		},
	},

	{
		name: "stat of a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := sys.Stat(fsys, "nope")
			return err
		},
	},

	{
		name: "stat on a symolic link returns information about the target file",
		base: fstest.MapFS{
			"test": &fstest.MapFile{Mode: 0600},
			"link": &fstest.MapFile{Mode: 0777 | fs.ModeSymlink, Data: []byte("test")},
		},
		test: func(fsys sys.FS) error {
			s, err := sys.Stat(fsys, "link")
			if err != nil {
				return err
			}
			if mode := s.Mode(); mode.Type() != 0 {
				return fmt.Errorf("wrong mode: %s", mode)
			}
			return nil
		},
	},
}

var testValidateLstat = fsTestSuite{
	{
		name: "lstat of a file with and invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := sys.Lstat(fsys, "/")
			return err
		},
	},

	{
		name: "lstat of a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := sys.Lstat(fsys, "nope")
			return err
		},
	},

	{
		name: "lstat on a symolic link returns information about the link itself",
		base: fstest.MapFS{
			"link": &fstest.MapFile{Mode: 0777 | fs.ModeSymlink, Data: []byte("test")},
		},
		test: func(fsys sys.FS) error {
			s, err := sys.Lstat(fsys, "link")
			if err != nil {
				return err
			}
			if mode := s.Mode(); mode.Type() != fs.ModeSymlink {
				return fmt.Errorf("wrong mode: %s", mode)
			}
			return nil
		},
	},
}

func testOpen(name string, test func(fs.File) error) func(sys.FS) error {
	return func(fsys sys.FS) error {
		f, err := fsys.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		return test(f)
	}
}

func testOpenFile(name string, flags int, mode fs.FileMode, test func(sys.File) error) func(sys.FS) error {
	return func(fsys sys.FS) error {
		f, err := fsys.OpenFile(name, flags, mode)
		if err != nil {
			return err
		}
		defer f.Close()
		return test(f)
	}
}

func fsTestCaseOpenAndClose(method string, test func(fs.File) error) fsTestCase {
	return fsTestCase{
		name: "calling " + method + " after a file is closed fails with ErrClosed",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrClosed,
		test: testOpen("test", func(f fs.File) error {
			if err := f.Close(); err != nil {
				return err
			}
			return test(f)
		}),
	}
}

var testValidateFileOpen = fsTestSuite{
	fsTestCaseOpenAndClose("Close", func(f fs.File) error {
		err := f.Close()
		return err
	}),

	fsTestCaseOpenAndClose("Read", func(f fs.File) error {
		_, err := f.Read(nil)
		return err
	}),

	fsTestCaseOpenAndClose("Stat", func(f fs.File) error {
		_, err := f.Stat()
		return err
	}),
}

func fsTestCaseOpenFileAndClose(method string, test func(sys.File) error) fsTestCase {
	return fsTestCase{
		name: "calling " + method + " after a file is closed fails with ErrClosed",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrClosed,
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error {
			if err := f.Close(); err != nil {
				return err
			}
			return test(f)
		}),
	}
}

var testValidateFileOpenFile = fsTestSuite{
	fsTestCaseOpenFileAndClose("Close", func(f sys.File) error {
		err := f.Close()
		return err
	}),

	fsTestCaseOpenFileAndClose("Read", func(f sys.File) error {
		_, err := f.Read(nil)
		return err
	}),

	fsTestCaseOpenFileAndClose("ReadAt", func(f sys.File) error {
		_, err := f.ReadAt(nil, 0)
		return err
	}),

	fsTestCaseOpenFileAndClose("Write", func(f sys.File) error {
		_, err := f.Write(nil)
		return err
	}),

	fsTestCaseOpenFileAndClose("WriteAt", func(f sys.File) error {
		_, err := f.WriteAt(nil, 0)
		return err
	}),

	fsTestCaseOpenFileAndClose("Seek", func(f sys.File) error {
		_, err := f.Seek(0, 0)
		return err
	}),

	fsTestCaseOpenFileAndClose("Stat", func(f sys.File) error {
		_, err := f.Stat()
		return err
	}),

	fsTestCaseOpenFileAndClose("ReadDir", func(f sys.File) error {
		_, err := f.ReadDir(0)
		return err
	}),

	fsTestCaseOpenFileAndClose("Chmod", func(f sys.File) error {
		err := f.Chmod(0)
		return err
	}),

	fsTestCaseOpenFileAndClose("Chtimes", func(f sys.File) error {
		now := time.Now()
		err := f.Chtimes(now, now)
		return err
	}),

	fsTestCaseOpenFileAndClose("Truncate", func(f sys.File) error {
		err := f.Truncate(0)
		return err
	}),
}

var testValidateFileRead = fsTestSuite{
	{
		name: "reading an empty file returns EOF immediately",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  io.EOF,
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error {
			_, err := f.Read(make([]byte, 1))
			return err
		}),
	},

	{
		name: "reading a file returns its content",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("hello")}},
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error {
			b, err := io.ReadAll(f)
			if err != nil {
				return err
			}
			if string(b) != "hello" {
				return fmt.Errorf("wrong file content: %q", b)
			}
			return nil
		}),
	},
}

var testValidateFileWrite = fsTestSuite{
	{
		name: "writing bytes to a file open with O_RDONLY fails with ErrPermission",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrPermission,
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error {
			_, err := f.Write(make([]byte, 1))
			return err
		}),
	},

	{
		name: "writing a string to a file open with O_RDONLY fails with ErrPermission",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrPermission,
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error {
			_, err := io.WriteString(f, "hello")
			return err
		}),
	},
}

var testValidateFileChmod = fsTestSuite{}

var testValidateFileChtimes = fsTestSuite{}

var testValidateFileTruncate = fsTestSuite{
	{
		name: "truncating a file to a negative size fails with ErrInvalid",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("123")}},
		err:  sys.ErrInvalid,
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error { return f.Truncate(-1) }),
	},
}

var testValidateFileSync = fsTestSuite{}

var testValidateFileDatasync = fsTestSuite{}

var testValidateFileCopy = fsTestSuite{}
