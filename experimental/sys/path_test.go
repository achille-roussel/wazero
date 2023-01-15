package sys_test

import (
	"reflect"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
)

func TestPathContains(t *testing.T) {
	for _, test := range [...]struct {
		base string
		path string
		want bool
	}{

		{
			base: ".",
			path: ".",
			want: true,
		},

		{
			base: ".",
			path: "whatever",
			want: true,
		},

		{
			base: "a",
			path: "a",
			want: true,
		},

		{
			base: "a",
			path: "a/b",
			want: true,
		},

		{
			base: "a/b",
			path: "a/b",
			want: true,
		},

		{
			base: "a/b",
			path: "a/b/c",
			want: true,
		},

		{
			base: "..",
			path: "../a",
			want: true,
		},

		{
			base: "a",
			path: "b",
			want: false,
		},

		{
			base: "a/b",
			path: "a/c",
			want: false,
		},

		{
			base: "a/b",
			path: "a",
			want: false,
		},

		{
			base: "a/b",
			path: ".",
			want: false,
		},
	} {
		if sys.PathContains(test.base, test.path) != test.want {
			t.Errorf("sys.PathContains(%q, %q) => %t, but want %t", test.base, test.path, !test.want, test.want)
		}
	}
}

func TestJoinPath(t *testing.T) {
	for _, test := range [...]struct {
		base string
		name string
		path string
	}{
		{
			base: ".",
			name: ".",
			path: ".",
		},

		{
			base: ".",
			name: "..",
			path: ".",
		},

		{
			base: ".",
			name: "../..",
			path: ".",
		},

		{
			base: ".",
			name: "../../..",
			path: ".",
		},

		{
			base: "hello",
			name: ".",
			path: "hello",
		},

		{
			base: "hello",
			name: "..",
			path: ".",
		},

		{
			base: "hello",
			name: "../..",
			path: ".",
		},

		{
			base: "hello/world",
			name: "..",
			path: "hello",
		},

		{
			base: "hello/world",
			name: "../..",
			path: ".",
		},

		{
			base: "hello/world",
			name: "../../..",
			path: ".",
		},
	} {
		if path := sys.JoinPath(test.base, test.name); path != test.path {
			t.Errorf("join(%q,%q): want=%q got=%q", test.base, test.name, test.path, path)
		}
	}
}

func TestValidPath(t *testing.T) {
	valid := [...]string{
		".",
		"hello",
		"hello/world",
		"hello/world/!",
		"..",
		"../hello",
		"../hello/world",
		"../hello/world/!",
		"../..",
		"../../hello",
		"../../hello/world",
		"../../hello/world/!",
	}

	invalid := [...]string{
		// absolute paths are not allowed, the root of a fs.FS is "."
		"/",
		"/hello",
		"/hello/world",
		"/hello/world/!",
		// relative paths are not allowed, they must be cleaned and not start
		// with "./"
		"./",
		"./hello",
		"./hello/world",
		// non-clean paths are not allowed, even if they would be valid after
		// being claned
		"hello/.",
		"hello/./",
		"hello/./world",
		"hello/..",
		"hello/../",
		"hello/../world",
		// valid relative paths but not clean
		"../",
		"../../",
		"../../../",
	}

	for _, path := range valid {
		if !sys.ValidPath(path) {
			t.Errorf("path should be valid: %s", path)
		}
	}

	for _, path := range invalid {
		if sys.ValidPath(path) {
			t.Errorf("path should not be valid: %s", path)
		}
	}
}

func TestSplitPath(t *testing.T) {
	for _, test := range [...]struct {
		path string
		dir  string
		file string
	}{
		{
			path: ".",
			dir:  ".",
			file: ".",
		},

		{
			path: "..",
			dir:  ".",
			file: "..",
		},

		{
			path: "hello",
			dir:  ".",
			file: "hello",
		},

		{
			path: "hello/world",
			dir:  "hello",
			file: "world",
		},

		{
			path: "hello/world/!",
			dir:  "hello/world",
			file: "!",
		},

		{
			path: "../hello",
			dir:  "..",
			file: "hello",
		},
	} {
		dir, file := sys.SplitPath(test.path)
		if dir != test.dir || file != test.file {
			t.Errorf("%s: want=(%q,%q) got=(%q,%q)", test.path, test.dir, test.file, dir, file)
		}
	}
}

func TestWalkPath(t *testing.T) {
	walk := []string{}

	for _, test := range [...]struct {
		base string
		path string
		root string
		name string
		walk []string
	}{
		{
			base: ".",
			path: ".",
			root: ".",
			name: ".",
			walk: []string{},
		},

		{
			base: ".",
			path: "hello",
			root: ".",
			name: "hello",
			walk: []string{},
		},

		{
			base: ".",
			path: "hello/world",
			root: "hello",
			name: "world",
			walk: []string{"hello"},
		},

		{
			base: ".",
			path: "hello/world/!",
			root: "hello/world",
			name: "!",
			walk: []string{"hello", "world"},
		},

		{
			base: ".",
			path: "..",
			root: ".",
			name: ".",
			walk: []string{},
		},

		{
			base: ".",
			path: "../..",
			root: ".",
			name: ".",
			walk: []string{},
		},

		{
			base: ".",
			path: "../../..",
			root: ".",
			name: ".",
			walk: []string{},
		},

		{
			base: "hello",
			path: "..",
			root: ".",
			name: ".",
			walk: []string{".."},
		},

		{
			base: "hello/world",
			path: "..",
			root: "hello",
			name: ".",
			walk: []string{".."},
		},

		{
			base: "hello/world",
			path: "../answer",
			root: ".",
			name: "answer",
			walk: []string{".."},
		},

		{
			base: "hello/world",
			path: "../..",
			root: ".",
			name: ".",
			walk: []string{"..", ".."},
		},

		{
			base: "hello/world",
			path: "../../answer",
			root: ".",
			name: "answer",
			walk: []string{"..", ".."},
		},

		{
			base: "sub/symlink-to-root-1",
			path: "../..",
			root: ".",
			name: ".",
			walk: []string{"..", ".."},
		},
	} {
		walk = walk[:0]
		if root, name, err := sys.WalkPath(test.base, test.path, func(name string) error {
			walk = append(walk, name)
			return nil
		}); err != nil {
			t.Error("unexpected error:", err)
		} else if root != test.root || name != test.name || !reflect.DeepEqual(walk, test.walk) {
			t.Errorf("walk(%q,%q): want=%q,%q,%q got=%q,%q,%q", test.base, test.path, test.root, test.name, test.walk, root, name, walk)
		}
	}
}

func TestMkdirAll(t *testing.T) {
	testFS := sys.DirFS(t.TempDir())

	mkdirAll := func(path string) {
		if err := sys.MkdirAll(testFS, path, 0755); err != nil {
			t.Error(err)
		}
	}

	mkdirAll(".")
	mkdirAll("a")
	mkdirAll("a/b/c")
	mkdirAll("a/b/c")
	mkdirAll("a/b/c/d")
	mkdirAll("a/b/c")
}
