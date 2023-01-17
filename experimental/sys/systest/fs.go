package systest

import (
	"errors"
	"io/fs"
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
