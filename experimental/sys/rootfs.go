package sys

import (
	"errors"
	"io/fs"
	"strings"
)

// RootFS wraps a file system to ensure that path resolutions are not allowed
// to escape the root of the file system (e.g. following symbolic links).
func RootFS(root FS) FS {
	return FuncFS(func(_ FS, name string, flags int, perm fs.FileMode) (File, error) {
		d, err := OpenRoot(root)
		if err != nil {
			return nil, err
		}
		root := &sharedFile{File: d}
		root.ref()
		defer root.unref()
		if name == "." {
			root.ref() // +1 because we keep 2 references to it
			return newRootFile(root, sharedFileRef{root}, "."), nil
		}
		f, err := lookup(d, d, ".", name, flags, perm)
		if err != nil {
			return nil, err
		}
		return newRootFile(root, f, name), nil
	})
}

func newRootFile(root *sharedFile, file File, name string) *rootFile {
	root.ref()
	return &rootFile{root: root, name: name, File: file}
}

type rootFile struct {
	root *sharedFile
	name string
	File
}

func (f *rootFile) Close() error {
	f.root.unref()
	f.root = nil
	return f.File.Close()
}

func (f *rootFile) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	newFile, err := f.openFile(name, flags, perm)
	if err != nil {
		return nil, err
	}
	return newRootFile(f.root, newFile, JoinPath(f.name, name)), nil
}

func (f *rootFile) openFile(name string, flags int, perm fs.FileMode) (File, error) {
	return lookup(f.root.File, f.File, f.name, name, flags, perm)
}

func (f *rootFile) Mkdir(name string, perm fs.FileMode) error {
	return lookupDir(f, "mkdir", name, func(dir Directory, name string) error {
		return dir.Mkdir(name, perm)
	})
}

func (f *rootFile) Rmdir(name string) error {
	return lookupDir(f, "rmdir", name, Directory.Rmdir)
}

func (f *rootFile) Unlink(name string) error {
	return lookupDir(f, "unlink", name, Directory.Unlink)
}

func (f *rootFile) Symlink(oldName, newName string) error {
	return lookupDir(f, "symlink", newName, func(dir Directory, newName string) error {
		return dir.Symlink(oldName, newName)
	})
}

func (f *rootFile) Link(oldName string, newDir Directory, newName string) error {
	return lookupDir2(f, "link", oldName, newName, Directory.Link)
}

func (f *rootFile) Rename(oldName string, newDir Directory, newName string) error {
	return lookupDir2(f, "rename", oldName, newName, Directory.Rename)
}

type nopClose struct{ File }

func (nopClose) Close() error { return nil }

// sentinel error value used to break out of WalkPath when resolving symbolic links
var errResolveSymlink = errors.New("resolve symlink")

func lookup(root, dir File, base, path string, flags int, perm fs.FileMode) (File, error) {
	dir = nopClose{dir} // don't close the first directories received as arguments
	defer func() { dir.Close() }()

	setCurrentDirectory := func(d File) {
		dir.Close()
		dir = d
	}

	setSymbolicLink := func(link string) error {
		if link = CleanPath(link); strings.HasPrefix(link, "/") {
			// The symbolic link contained an absolute path starting with a "/".
			// We go back to the root and start resolving paths back from there.
			setCurrentDirectory(nopClose{root})
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

	return f, nil
}

func lookupDir(f *rootFile, op, name string, do func(Directory, string) error) error {
	dir, base := SplitPath(name)
	if dir == "." {
		return do(f.File, base)
	}
	d, err := f.openFile(dir, O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer d.Close()
	return do(d, base)
}

func lookupDir2(f *rootFile, op, name1, name2 string, do func(Directory, string, Directory, string) error) error {
	arg1 := Directory(f.File)
	arg2 := Directory(f.File)
	dir1, base1 := SplitPath(name1)
	dir2, base2 := SplitPath(name2)
	if dir1 != "." {
		d1, err := f.openFile(dir1, O_DIRECTORY, 0)
		if err != nil {
			return err
		}
		defer d1.Close()
		arg1 = d1
	}
	if dir2 != "." {
		d2, err := f.openFile(dir2, O_DIRECTORY, 0)
		if err != nil {
			return err
		}
		defer d2.Close()
		arg2 = d2
	}
	return do(arg1, base1, arg2, base2)
}
