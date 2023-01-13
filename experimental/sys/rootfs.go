package sys

import (
	"errors"
	"io/fs"
	"strings"
)

// RootFS wraps a file system to ensure that path resolutions are not allowed
// to escape the root of the file system (e.g. following symbolic links).
func RootFS(root FS) FS { return &rootFS{root: root} }

type rootFS struct{ root FS }

func (fsys *rootFS) Open(name string) (fs.File, error) {
	return Open(fsys, name)
}

func (fsys *rootFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	if !ValidPath(name) {
		return nil, ErrNotExist
	}
	f, err := fsys.openFile(name, flags, perm)
	if err != nil {
		return nil, makePathError("open", name, err)
	}
	return fsys.newFile(f, name), nil
}

func (fsys *rootFS) newFile(file File, name string) *rootFile {
	return &rootFile{root: fsys, name: name, File: file}
}

func (fsys *rootFS) openFile(name string, flags int, perm fs.FileMode) (File, error) {
	root, err := fsys.root.OpenFile(".", O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	if name == "." {
		return root, nil
	}
	defer root.Close()
	return fsys.openFileAt(root, ".", name, flags, perm)
}

type nopClose struct{ File }

func (nopClose) Close() error { return nil }

var errResolveSymlink = errors.New("resolve symlink")

func (fsys *rootFS) openFileAt(dir File, base, path string, flags int, perm fs.FileMode) (File, error) {
	dir = nopClose{dir} // don't close the first directory received as argument
	dirFS := dir.FS()
	defer func() { dir.Close() }()

	setCurrentDirectory := func(d File) {
		dir.Close()
		dir, dirFS = d, d.FS()
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
		f, err := dirFS.OpenFile(dirname, rootfsOpenFileFlags, 0)
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

	f, err := dirFS.OpenFile(path, openFlags, perm)
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

func (fsys *rootFS) Mkdir(name string, perm fs.FileMode) error {
	return lookupDir(fsys.openFile, "mkdir", name, func(dir FS, name string) error {
		return dir.Mkdir(name, perm)
	})
}

func (fsys *rootFS) Rmdir(name string) error {
	return lookupDir(fsys.openFile, "rmdir", name, FS.Rmdir)
}

func (fsys *rootFS) Unlink(name string) error {
	return lookupDir(fsys.openFile, "unlink", name, FS.Unlink)
}

func (fsys *rootFS) Link(oldName, newName string, newFS FS) error {
	return lookupLinkOrRename(fsys.openFile, "link", oldName, newName, FS.Link)
}

func (fsys *rootFS) Rename(oldName, newName string, newFS FS) error {
	return lookupLinkOrRename(fsys.openFile, "rename", oldName, newName, FS.Rename)
}

func (fsys *rootFS) Symlink(oldName, newName string) error {
	return lookupDir(fsys.openFile, "symlink", newName, func(dir FS, name string) error {
		return dir.Symlink(oldName, name)
	})
}

type rootFile struct {
	root *rootFS
	name string
	File
}

func (f *rootFile) FS() FS { return rootFileFS{f} }

type rootFileFS struct{ *rootFile }

func (d rootFileFS) Open(name string) (fs.File, error) {
	return d.OpenFile(name, O_RDONLY, 0)
}

func (d rootFileFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	if !ValidPath(name) {
		return nil, makePathError("open", name, ErrNotExist)
	}
	f, err := d.openFile(name, flags, perm)
	if err != nil {
		return nil, makePathError("open", name, err)
	}
	return d.root.newFile(f, JoinPath(d.name, name)), nil
}

func (d rootFileFS) openFile(name string, flags int, perm fs.FileMode) (File, error) {
	return d.root.openFileAt(d.File, d.name, name, flags, perm)
}

func (d rootFileFS) Mkdir(name string, perm fs.FileMode) error {
	return lookupDir(d.openFile, "mkdir", name, func(dir FS, name string) error {
		return dir.Mkdir(name, perm)
	})
}

func (d rootFileFS) Rmdir(name string) error {
	return lookupDir(d.openFile, "rmdir", name, FS.Rmdir)
}

func (d rootFileFS) Unlink(name string) error {
	return lookupDir(d.openFile, "unlink", name, FS.Unlink)
}

func (d rootFileFS) Link(oldName, newName string, newFS FS) error {
	return lookupLinkOrRename(d.openFile, "link", oldName, newName, FS.Link)
}

func (d rootFileFS) Rename(oldName, newName string, newFS FS) error {
	return lookupLinkOrRename(d.openFile, "rename", oldName, newName, FS.Rename)
}

func (d rootFileFS) Symlink(oldName, newName string) error {
	return lookupDir(d.openFile, "symlink", newName, func(dir FS, name string) error {
		return dir.Symlink(oldName, name)
	})
}

func lookupFile(open openFileFunc, op, name string, flags int, do func(File) error) error {
	if !ValidPath(name) {
		return makePathError(op, name, ErrNotExist)
	}
	f, err := open(name, flags, 0)
	if err != nil {
		return makePathError(op, name, err)
	}
	defer f.Close()
	return do(f)
}

func lookupFile1[Func func(File) (Ret, error), Ret any](open openFileFunc, op, name string, flags int, do Func) (ret Ret, err error) {
	if !ValidPath(name) {
		return ret, makePathError(op, name, ErrNotExist)
	}
	f, err := open(name, flags, 0)
	if err != nil {
		return ret, makePathError(op, name, err)
	}
	defer f.Close()
	return do(f)
}

func lookupDir(open openFileFunc, op, name string, do func(FS, string) error) error {
	if !ValidPath(name) {
		return makePathError(op, name, ErrNotExist)
	}
	dir, base := SplitPath(name)
	f, err := open(dir, O_DIRECTORY, 0)
	if err != nil {
		return makePathError(op, name, err)
	}
	defer f.Close()
	return do(f.FS(), base)
}

func lookupLinkOrRename(open openFileFunc, op, name1, name2 string, do linkOrRename) error {
	if !ValidPath(name1) {
		return makePathError(op, name1, ErrNotExist)
	}
	if !ValidPath(name2) {
		return makePathError(op, name2, ErrInvalid)
	}
	dir1, base1 := SplitPath(name1)
	dir2, base2 := SplitPath(name2)
	d1, err := open(dir1, O_DIRECTORY, 0)
	if err != nil {
		return makePathError(op, name1, err)
	}
	defer d1.Close()
	d2, err := open(dir2, O_DIRECTORY, 0)
	if err != nil {
		return makePathError(op, name2, err)
	}
	defer d2.Close()
	return do(d1.FS(), base1, base2, d2.FS())
}
