package systest

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"sort"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
)

// MakeFS is a function type used to construct file systems during tests.
//
// The function receives the *testing.T instance of the test that the file
// system is created for, and fs.FS value representing the initial state
// expected by the test.
//
// If cleanup needs to be done to tear down the file system, the MakeFS function
// must register a cleanup function to the test using testing.(*T).Cleanup.
//
// If the file system creation fails, the function must abort the test by
// calling testing.(*T).Fatal or testing.(*T).Fatalf.
type MakeFS func(*testing.T, fs.FS) sys.FS

// NewFS is a function tyep used to instantiate file systems during tests.
//
// The function recives the *testing.T instance of the test that the file
// system is created for.
//
// If cleanup needs to be done to tear down the file system, the NewFS function
// must register a cleanup function to the test using testing.(*T).Cleanup.
//
// If the file system creation fails, the function must abort the test by
// calling testing.(*T).Fatal or testing.(*T).Fatalf.
type NewFS func(*testing.T) sys.FS

func testFS(t *testing.T, makeFS MakeFS) {
	fstestTestFS(t, makeFS, fstest.MapFS{
		"file-0": &fstest.MapFile{
			Data: []byte(""),
			Mode: 0644,
		},

		"file-1": &fstest.MapFile{
			Data: []byte("Hello World!"),
			Mode: 0644,
		},

		"file-2": &fstest.MapFile{
			Data: []byte("1234567890"),
			Mode: 0644,
		},

		"tmp/file": &fstest.MapFile{
			Mode: 0644,
		},

		"tmp": &fstest.MapFile{
			Mode:    0755 | fs.ModeDir,
			ModTime: epoch,
		},
	})
}

func fstestTestFS(t *testing.T, makeFS MakeFS, files fstest.MapFS) {
	fsys := makeFS(t, files)

	expected := make([]string, 0, len(files))
	for name := range files {
		expected = append(expected, name)
	}

	sort.Strings(expected)

	if err := fstest.TestFS(fsys, expected...); err != nil {
		t.Error(err)
	}
}

// use microsecond precision because a lot of file systems cannot record
// times with nanosecond granularity.
var epoch = time.Now().Truncate(time.Microsecond)

type fsTestCase struct {
	name string
	base fs.FS // initial file system state before the test
	want fs.FS // expected file system state after the test
	err  error // expected error returned by the test
	test func(sys.FS) error
}

type fsTestSuite []fsTestCase

func (suite fsTestSuite) run(t *testing.T, makeFS MakeFS) {
	tests := make(map[string]*fsTestCase, len(suite))
	names := make([]string, 0, len(suite))

	for i := range suite {
		test := &suite[i]
		if _, exists := tests[test.name]; exists {
			t.Errorf("test suite contains two suite named %q", test.name)
			return
		}
		tests[test.name] = test
		names = append(names, test.name)
	}

	sort.Strings(names)

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			test := tests[name]
			base := test.base
			if base == nil {
				base = fstest.MapFS{}
			}
			fsys := makeFS(t, base)
			if err := test.test(fsys); !errors.Is(err, test.err) {
				t.Errorf("error mismatch: want=%v got=%v", test.err, err)
			} else if test.want != nil {
				if err := sys.EqualFS(fsys, sys.NewFS(test.want)); err != nil {
					t.Error(err)
				}
			}
		})
	}
}

func (suite fsTestSuite) runFunc(makeFS MakeFS) func(*testing.T) {
	return func(t *testing.T) { suite.run(t, makeFS) }
}

type fsTestGroup struct {
	name  string
	suite fsTestSuite
}

func (group *fsTestGroup) run(t *testing.T, makeFS MakeFS) {
	t.Run(group.name, group.suite.runFunc(makeFS))
}

func fsTestRun(t *testing.T, makeFS MakeFS, groups []fsTestGroup) {
	for _, group := range groups {
		group.run(t, makeFS)
	}
}

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
}

var testValidateMkdir = fsTestSuite{
	{
		name: "creating a directory with an invalid name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return fsys.Mkdir("/", 0755) },
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
		test: func(fsys sys.FS) error { return fsys.Link("/", "new", fsys) },
	},
	{
		name: "linking a file with an invalid target name fails with ErrInvalid",
		err:  sys.ErrInvalid,
		test: func(fsys sys.FS) error { return fsys.Link("old", "/", fsys) },
	},
}

var testValidateSymlink = fsTestSuite{
	{
		name: "creating a symbolic link with an invalid target name fails with ErrNotExist",
		err:  sys.ErrNotExist,
		test: func(fsys sys.FS) error { return fsys.Symlink("old", "/") },
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
		test: func(fsys sys.FS) error { return fsys.Rename("/", "new", fsys) },
	},
	{
		name: "renaming a file with an invalid target name fails with ErrInvalid",
		err:  sys.ErrInvalid,
		test: func(fsys sys.FS) error { return fsys.Rename("old", "/", fsys) },
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
}

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
		name: "writing bytes to a file open with O_RDONLY fails with ErrNotSupported",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotSupported,
		test: testOpenFile("test", sys.O_RDONLY, 0, func(f sys.File) error {
			_, err := f.Write(make([]byte, 1))
			return err
		}),
	},

	{
		name: "writing a string to a file open with O_RDONLY fails with ErrNotSupported",
		base: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		want: fstest.MapFS{"test": &fstest.MapFile{Mode: 0644}},
		err:  sys.ErrNotSupported,
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
