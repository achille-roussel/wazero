package systest

import (
	"errors"
	"io"
	"io/fs"
	"sort"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero/experimental/sysfs"
)

// CloseFS is a function returned by MakeReadOnlyFS and MakeReadWriteFS which is
// used to tear down resources associated with a file system instance created
// during a test.
type CloseFS func()

func testFS(t *testing.T, fsys fs.FS, files fstest.MapFS) {
	t.Helper()
	t.Run("fstest", func(t *testing.T) {
		expected := make([]string, 0, len(files))
		for fileName := range files {
			expected = append(expected, fileName)
		}
		sort.Strings(expected)
		if err := fstest.TestFS(fsys, expected...); err != nil {
			t.Error(err)
		}
	})
}

func operationNameOf(testName string) string {
	i := strings.IndexByte(testName, ' ')
	if i >= 0 {
		testName = testName[:i]
	}
	switch testName {
	case "ReadAt":
		return "read"
	case "WriteAt":
		return "write"
	default:
		return strings.ToLower(testName)
	}
}

type fileTestCases map[string]func(sysfs.File) error

func testFilePathError(t *testing.T, file sysfs.File, want error, name string, tests fileTestCases) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				if err := test(file); !errors.Is(err, want) {
					t.Errorf("error mismatch\nwant: %v\ngot:  %v", want, err)
				} else if e, ok := err.(*fs.PathError); !ok {
					t.Errorf("wrong error type: want=fs.PathError got=%T", err)
				} else if op := operationNameOf(name); op != e.Op {
					t.Errorf("operation mismatch: want=%q got=%q", op, e.Op)
				}
			})
		}
	})
}

func testFileErrIsDir(t *testing.T, file sysfs.File) {
	t.Helper()

	testFilePathError(t, file, syscall.EISDIR, "EISDIR", fileTestCases{
		"Read": func(file sysfs.File) error {
			_, err := file.Read(make([]byte, 1))
			return err
		},

		"Read (empty)": func(file sysfs.File) error {
			_, err := file.Read(nil)
			return err
		},

		"ReadAt": func(file sysfs.File) error {
			_, err := file.ReadAt(make([]byte, 1), 0)
			return err
		},

		"ReadAt (empty)": func(file sysfs.File) error {
			_, err := file.ReadAt(nil, 0)
			return err
		},
	})

	// These syscalls historically return EBADF rather than EISDIR when called
	// with a file descriptor opened on a directory.
	testFilePathError(t, file, syscall.EBADF, "EBADF", fileTestCases{
		"Write": func(file sysfs.File) error {
			_, err := file.Write(make([]byte, 1))
			return err
		},

		"Write (empty)": func(file sysfs.File) error {
			_, err := file.Write(nil)
			return err
		},

		"WriteAt": func(file sysfs.File) error {
			_, err := file.WriteAt(make([]byte, 1), 0)
			return err
		},

		"WriteAt (empty)": func(file sysfs.File) error {
			_, err := file.WriteAt(nil, 0)
			return err
		},
	})
}

func testFileErrNotDir(t *testing.T, file sysfs.File) {
	t.Helper()
	testFilePathError(t, file, syscall.ENOTDIR, "ENOTDIR", fileTestCases{
		"ReadDir": func(file sysfs.File) error {
			_, err := file.ReadDir(0)
			return err
		},

		"Open (self)": func(file sysfs.File) error {
			_, err := file.Open(".", 0, 0)
			return err
		},

		"Open (file)": func(file sysfs.File) error {
			_, err := file.Open("foo", 0, 0)
			return err
		},

		"Unlink": func(file sysfs.File) error {
			err := file.Unlink("foo")
			return err
		},

		"Rename": func(file sysfs.File) error {
			err := file.Rename("foo", "bar")
			return err
		},

		"Link": func(file sysfs.File) error {
			err := file.Link("foo", "bar")
			return err
		},

		"Symlink": func(file sysfs.File) error {
			err := file.Symlink("foo", "bar")
			return err
		},

		"Readlink": func(file sysfs.File) error {
			_, err := file.Readlink("foo")
			return err
		},

		"Mkdir": func(file sysfs.File) error {
			err := file.Mkdir("tmp", 0777)
			return err
		},

		"Rmdir": func(file sysfs.File) error {
			err := file.Rmdir("tmp")
			return err
		},
	})
}

func testFileErrClosed(t *testing.T, file sysfs.File) {
	t.Helper()

	if err := file.Close(); err != nil {
		t.Error("closing:", err)
	}

	testFilePathError(t, file, fs.ErrClosed, "ErrClosed", fileTestCases{
		"Close": func(file sysfs.File) error {
			err := file.Close()
			return err
		},

		"ReadDir": func(file sysfs.File) error {
			_, err := file.ReadDir(0)
			return err
		},

		"Read": func(file sysfs.File) error {
			_, err := file.Read(nil)
			return err
		},

		"ReadAt": func(file sysfs.File) error {
			_, err := file.ReadAt(nil, 0)
			return err
		},

		"Write": func(file sysfs.File) error {
			_, err := file.Write(nil)
			return err
		},

		"WriteAt": func(file sysfs.File) error {
			_, err := file.WriteAt(nil, 0)
			return err
		},

		"Seek": func(file sysfs.File) error {
			_, err := file.Seek(0, io.SeekCurrent)
			return err
		},

		"Chmod": func(file sysfs.File) error {
			err := file.Chmod(0600)
			return err
		},

		"Chtimes": func(file sysfs.File) error {
			now := time.Now()
			err := file.Chtimes(now, now)
			return err
		},

		"Open": func(file sysfs.File) error {
			_, err := file.Open("foo", 0, 0)
			return err
		},

		"Unlink": func(file sysfs.File) error {
			err := file.Unlink("foo")
			return err
		},

		"Rename": func(file sysfs.File) error {
			err := file.Rename("foo", "bar")
			return err
		},

		"Link": func(file sysfs.File) error {
			err := file.Link("foo", "bar")
			return err
		},

		"Symlink": func(file sysfs.File) error {
			err := file.Symlink("foo", "bar")
			return err
		},

		"Readlink": func(file sysfs.File) error {
			_, err := file.Readlink("foo")
			return err
		},

		"Mkdir": func(file sysfs.File) error {
			err := file.Mkdir("tmp", 0777)
			return err
		},

		"Rmdir": func(file sysfs.File) error {
			err := file.Rmdir("tmp")
			return err
		},

		"Stat": func(file sysfs.File) error {
			_, err := file.Stat()
			return err
		},

		"Sync": func(file sysfs.File) error {
			err := file.Sync()
			return err
		},

		"Datasync": func(file sysfs.File) error {
			err := file.Datasync()
			return err
		},
	})
}
