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

// The test suites below contain tests for the basic functionalities of both the
// read-only and read-write test cases.

var testDefaultOpenFile = append(testValidateOpenFile,
	fsTestCase{
		name: "opening a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := fsys.OpenFile("nope", sys.O_RDONLY, 0)
			return err
		},
	},

	fsTestCase{
		name: "opening a file with O_DIRECTORY fails with ErrNotDirectory",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotDirectory,
		test: func(fsys sys.FS) error {
			_, err := fsys.OpenFile("test", sys.O_RDONLY|sys.O_DIRECTORY, 0)
			return err
		},
	},
)

var testDefaultOpen = append(testValidateOpen,
	fsTestCase{
		name: "opening a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := fsys.Open("nope")
			return err
		},
	},

	fsTestCase{
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

	fsTestCase{
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

	fsTestCase{
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

	fsTestCase{
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
)

var testDefaultAccess = append(testValidateAccess,
	fsTestCase{
		name: "existing files can be tested for existance",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		test: func(fsys sys.FS) error { return sys.Access(fsys, "test", 0) },
	},

	fsTestCase{
		name: "existing files can be accessed in read mode",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		test: func(fsys sys.FS) error { return sys.Access(fsys, "test", 0b100) },
	},
)

var testDefaultMknod = append(testValidateMknod)

var testDefaultMkdir = append(testValidateMkdir)

var testDefaultRmdir = append(testValidateRmdir)

var testDefaultUnlink = append(testValidateUnlink)

var testDefaultLink = append(testValidateLink)

var testDefaultSymlink = append(testValidateSymlink)

var testDefaultReadlink = append(testValidateReadlink)

var testDefaultRename = append(testValidateRename)

var testDefaultStat = append(testValidateStat,
	fsTestCase{
		name: "stat of a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := sys.Stat(fsys, "nope")
			return err
		},
	},

	fsTestCase{
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
)

var testDefaultLstat = append(testValidateLstat,
	fsTestCase{
		name: "lstat of a location which does not exist fails with ErrNotExist",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error {
			_, err := sys.Lstat(fsys, "nope")
			return err
		},
	},

	fsTestCase{
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
)

var testDefaultChmod = append(testValidateChmod)

var testDefaultChtimes = append(testValidateChtimes)

var testDefaultTruncate = append(testValidateTruncate)

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

var testDefaultFileOpen = fsTestSuite{
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

var testDefaultFileOpenFile = fsTestSuite{
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

var testDefaultFileRead = fsTestSuite{
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

var testDefaultFileWrite = fsTestSuite{
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

var testDefaultFileChmod = fsTestSuite{}

var testDefaultFileChtimes = fsTestSuite{}

var testDefaultFileTruncate = fsTestSuite{
	{
		name: "truncating a file to a negative size fails with ErrInvalid",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644, Data: []byte("123")}},
		err:  sys.ErrInvalid,
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error { return f.Truncate(-1) }),
	},
}

var testDefaultFileSync = fsTestSuite{}

var testDefaultFileDatasync = fsTestSuite{}

var testDefaultFileCopy = fsTestSuite{}
