package systest

import (
	"errors"
	"io/fs"
	"path/filepath"
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
			defer func() {
				// Some tests will modify write permissions of some directories
				// and files, which may prevent them from being deleted. We do a
				// recursive pass to reset all the permissions to allow the files
				// to be cleaned up.
				tmp := sys.DirFS(filepath.Dir(t.TempDir()))
				err := fs.WalkDir(tmp, ".",
					func(path string, entry fs.DirEntry, err error) error {
						if err != nil {
							return err
						} else if entry.IsDir() {
							return sys.Chmod(tmp, path, 0755)
						} else {
							return sys.Chmod(tmp, path, 0644)
						}
					},
				)
				if err != nil {
					t.Log("cleanup:", err)
				}
			}()

			test := tests[name]
			base := test.base
			if base == nil {
				base = fstest.MapFS{}
			}
			fsys := makeFS(t, base)

			// Always validate that the initial state of the file system
			// corresponds to what we expects. If this invariant is invalidated,
			// some tests could end up passing even if they did not exhibit the
			// expected behavior.
			if err := sys.EqualFS(fsys, sys.NewFS(base)); err != nil {
				t.Fatalf("invalid initial state of the file system: %v", err)
			}

			if err := test.test(fsys); !errors.Is(err, test.err) {
				if errors.Is(err, sys.ErrNotImplemented) {
					t.Skip(err)
				} else {
					t.Errorf("error mismatch:\nwant = %[1]v (%[1]T)\ngot  = %[2]v (%[2]T)", test.err, err)
				}
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

/*
func touch(path string) func(sys.FS) error {
	return func(fsys sys.FS) error {
		return sys.Touch(fsys, path, time.Now())
	}
}

func chmod(path string, mode fs.FileMode) func(sys.FS) error {
	return func(fsys sys.FS) error {
		return sys.Chmod(fsys, path, mode)
	}
}

func read(path, want string) func(sys.FS) error {
	return func(fsys sys.FS) error {
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		if string(data) != want {
			return fmt.Errorf("read %s: file content mismatch: want=%q got=%q", path, want, data)
		}
		return nil
	}
}

func write(path, data string, mode fs.FileMode) func(sys.FS) error {
	return func(fsys sys.FS) error {
		return sys.WriteFile(fsys, path, []byte(data), mode)
	}
}

func expect(want error, cmd func(sys.FS) error) func(sys.FS) error {
	return func(fsys sys.FS) error {
		if err := cmd(fsys); !errors.Is(err, want) {
			return err
		}
		return nil
	}
}

func commands(cmds ...func(sys.FS) error) func(sys.FS) error {
	return func(fsys sys.FS) error {
		for _, cmd := range cmds {
			if err := cmd(fsys); err != nil {
				return err
			}
		}
		return nil
	}
}
*/
