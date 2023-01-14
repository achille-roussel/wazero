package sys

import (
	"errors"
	"io/fs"
	"strings"
)

// RootFS wraps a file system to ensure that path resolutions are not allowed
// to escape the root of the file system (e.g. following symbolic links).
func RootFS(root FS) FS { return &rootFS{root} }

type rootFS struct{ root FS }

func (fsys *rootFS) Open(name string) (fs.File, error) { return Open(fsys, name) }

func (fsys *rootFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	f, err := fsys.openFile(name, flags, perm)
	if err != nil {
		return nil, makePathError("open", name, err)
	}
	return f, nil
}

func (fsys *rootFS) openFile(name string, flags int, perm fs.FileMode) (*rootFile, error) {
	if !ValidPath(name) {
		return nil, ErrNotExist
	}
	d, err := OpenRoot(fsys.root)
	if err != nil {
		return nil, err
	}
	if name == "." {
		return fsys.newFile(d, name), nil
	}
	defer d.Close()
	f, err := fsys.openFileAt(d, ".", name, flags, perm)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (fsys *rootFS) openFileAt(dir File, base, path string, flags int, perm fs.FileMode) (*rootFile, error) {
	dir = nopClose{dir} // don't close the first directory received as argument
	defer func() { dir.Close() }()
	// Capture these input values because we need them to compute the file name
	// if it is successfully opened.
	openBase := base
	openPath := path

	setCurrentDirectory := func(d File) {
		dir.Close()
		dir = d
	}

	setSymbolicLink := func(link string) error {
		if link = CleanPath(link); strings.HasPrefix(link, "/") {
			// The symbolic link contained an absolute path starting with a "/".
			// We go back to the root and start resolving paths back from there.
			r, err := fsys.root.OpenFile(".", O_DIRECTORY, 0)
			if err != nil {
				return err
			}
			setCurrentDirectory(r)
			base = "."
			path = link[1:]
		} else if path != "" {
			// There are trailing path components to lookup after resolving the
			// symbolic link, which means the link represented a directory; we
			// walk up the the parent because relative paths will be resolved
			// from there, and append the remaining path components to the link
			// target in order to form the full path to lookup.
			base, _ = SplitPath(base)
			path = CleanPath(link + "/" + path)
		} else {
			// The path was empty, which indicates that we had fully resolved
			// the symbolic link and are now pointing at the right location.
			path = link
		}
		return nil
	}

	var link string
	var loop int
	var err error
resolvePath:
	if loop++; loop == 40 {
		return nil, ErrLoop
	}

	base, path, err = WalkPath(base, path, func(dirname string) error {
		f, err := dir.OpenFile(dirname, rootfsOpenFileFlags, 0)
		if err != nil {
			return err
		}
		defer func() {
			if f != nil {
				f.Close()
			}
		}()

		s, err := f.Stat()
		if err != nil {
			return err
		}

		switch s.Mode().Type() {
		case fs.ModeDir:
			setCurrentDirectory(f)
			f = nil
			return nil

		case fs.ModeSymlink:
			s, err := f.Readlink()
			if err != nil {
				return err
			}
			link = s
			return errResolveSymlink

		default:
			return ErrNotDirectory
		}
	})

	switch err {
	case nil:
	case errResolveSymlink:
		if err := setSymbolicLink(link); err != nil {
			return nil, err
		}
		goto resolvePath
	default:
		return nil, err
	}

	// If O_DIRECTORY is passed, it already enforces O_NOFOLLOW since we are
	// explicitly saying that we want to open a directory and not a symbolic
	// link. In every other case, we add O_NOFOLLOW so we can perform the
	// symbolic link resolution (if any).
	openFlags := flags
	if (openFlags & O_DIRECTORY) == 0 {
		openFlags |= O_NOFOLLOW
	}

	f, err := dir.OpenFile(path, openFlags, perm)
	if err != nil {
		return nil, err
	}

	s, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	if ((flags & O_NOFOLLOW) == 0) && s.Mode().Type() == fs.ModeSymlink {
		s, err := f.Readlink()
		f.Close()
		if err != nil {
			return nil, err
		}
		path = ""
		if err := setSymbolicLink(s); err != nil {
			return nil, err
		}
		goto resolvePath
	}

	name := JoinPath(openBase, openPath)
	return fsys.newFile(f, name), nil
}

func (fsys *rootFS) newFile(file File, name string) *rootFile {
	return &rootFile{root: fsys, name: name, File: file}
}

type rootFile struct {
	root *rootFS
	name string
	File
}

func (f *rootFile) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	if !ValidPath(name) {
		return nil, makePathError("open", name, ErrNotExist)
	}
	newFile, err := f.root.openFileAt(f.File, f.name, name, flags, perm)
	if err != nil {
		return nil, makePathError("open", name, err)
	}
	return newFile, nil
}

type nopClose struct{ File }

func (nopClose) Close() error { return nil }

// sentinel error value used to break out of WalkPath when resolving symbolic links
var errResolveSymlink = errors.New("resolve symlink")
