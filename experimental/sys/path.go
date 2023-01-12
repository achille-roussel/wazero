package sys

import (
	"io/fs"
	"path"
	"strings"
)

// CleanPath cleans the given file system name. The returned value is always
// a valid input to ValidPath, which might contain leading parent directory
// lookups (".." or "../"). If the input is empty, the function returns ".".
func CleanPath(name string) string {
	if name == "" {
		return "."
	}
	return path.Clean(name)
}

// JoinPath joins base and name to form an absolute file system path.
func JoinPath(base, name string) string {
	join := path.Join("/", base, name)
	if join == "/" {
		join = "."
	} else {
		join = strings.TrimPrefix(join, "/")
	}
	return join
}

// SplitPath spearates the directory and file name of the given path name.
// Both returned values are valid inputs to ValidPath.
func SplitPath(name string) (dir, file string) {
	dir, file = path.Split(name)
	if dir == "" {
		dir = "."
	} else {
		dir = strings.TrimSuffix(dir, "/")
	}
	return dir, file
}

// ValidPath validates that the given path name is clean and is a valid input
// to methods of FS instances.
//
// The function is a superset of fs.ValidPath which accepts leading parent
// directory lookups like ".." or "../".
func ValidPath(name string) bool {
	if name == "" || strings.HasSuffix(name, "/") {
		return false
	}
	for {
		if name == "" {
			return true
		}
		if name == ".." {
			return true
		}
		if strings.HasPrefix(name, "../") {
			name = name[3:]
		} else {
			return fs.ValidPath(name)
		}
	}
}

// WalkPath walks through the path components of path, as if the path was
// resolved from the given base path, calling do for each path component.
//
// If path contains leading lookups to parent directories, do is called with
// the string ".." for each parent lookup up to consuming all parent lookups
// reaching the root of the base path.
//
// The function returns the new base directory that was reached after resolving
// the path, as well as the remaining last path component within this directory,
// which might be "." if the directory itself was represented by the path.
//
// The base must be a valid input to fs.ValidPath.
//
// The path must be a valid input to ValidPath.
//
// The function panics if base or path are invalid.
func WalkPath(base, path string, do func(string) error) (newBase, newPath string, err error) {
	if !fs.ValidPath(base) {
		panic("cannot walk path from invalid base: " + base)
	}
	if !ValidPath(path) {
		panic("cannot walk invalid path: " + path)
	}

	for path == ".." || strings.HasPrefix(path, "../") {
		path = strings.TrimPrefix(path[2:], "/")
		if base == "." {
			continue
		}
		base, _ = SplitPath(base)
		if err := do(".."); err != nil {
			if path == "" {
				path = "."
			}
			return base, path, err
		}
	}

	if path == "" {
		return base, ".", nil
	}

	i := 0
	for {
		j := strings.IndexByte(path[i:], '/')
		if j < 0 {
			break
		}
		name := path[i : i+j]
		i += j + 1
		if err = do(name); err != nil {
			break
		}
	}

	return CleanPath(path[:i]), path[i:], err
}
