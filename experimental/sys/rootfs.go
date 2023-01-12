package sys

import (
	"errors"
	"io/fs"
	"strings"
	"time"
)

// RootFS wraps a file system to ensure that path resolutions are not allowed
// to escape the root of the file system (e.g. following symbolic links).
func RootFS(root FS) FS { return &rootFS{root: root} }

type rootFS struct{ root FS }

func (fsys *rootFS) lookup(op, name string, flags int, do func(File) error) error {
	if !ValidPath(name) {
		return makePathError(op, name, ErrNotExist)
	}
	f, err := fsys.openFile(name, flags, 0)
	if err != nil {
		return makePathError(op, name, err)
	}
	defer f.Close()
	return do(f)
}

func (fsys *rootFS) lookup1(op, name string, do func(FS, string) error) error {
	if !ValidPath(name) {
		return makePathError(op, name, ErrNotExist)
	}
	dir, base := SplitPath(name)
	f, err := fsys.openFile(dir, O_DIRECTORY, 0)
	if err != nil {
		return makePathError(op, name, err)
	}
	defer f.Close()
	return do(f.FS(), base)
}

func (fsys *rootFS) lookup2(op, name1, name2 string, do func(FS, string, FS, string) error) error {
	if !ValidPath(name1) {
		return makePathError(op, name1, ErrNotExist)
	}
	if !ValidPath(name2) {
		return makePathError(op, name2, ErrInvalid)
	}
	dir1, base1 := SplitPath(name1)
	dir2, base2 := SplitPath(name2)
	d1, err := fsys.openFile(dir1, O_DIRECTORY, 0)
	if err != nil {
		return makePathError(op, name1, err)
	}
	defer d1.Close()
	d2, err := fsys.openFile(dir2, O_DIRECTORY, 0)
	if err != nil {
		return makePathError(op, name2, err)
	}
	defer d2.Close()
	return do(d1.FS(), base1, d2.FS(), base2)
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

func (fsys *rootFS) Open(name string) (fs.File, error) {
	return fsys.OpenFile(name, O_RDONLY, 0)
}

func (fsys *rootFS) Mkdir(name string, perm fs.FileMode) error {
	return fsys.lookup1("mkdir", name, func(dir FS, name string) error {
		return dir.Mkdir(name, perm)
	})
}

func (fsys *rootFS) Rmdir(name string) error {
	return fsys.lookup1("rmdir", name, FS.Rmdir)
}

func (fsys *rootFS) Unlink(name string) error {
	return fsys.lookup1("unlink", name, FS.Unlink)
}

func (fsys *rootFS) Link(oldName, newName string, newFS FS) error {
	return fsys.lookup2("link", oldName, newName,
		func(oldDir FS, oldName string, newDir FS, newName string) error {
			return oldDir.Link(oldName, newName, newDir)
		},
	)
}

func (fsys *rootFS) Rename(oldName, newName string, newFS FS) error {
	return fsys.lookup2("rename", oldName, newName,
		func(oldDir FS, oldName string, newDir FS, newName string) error {
			return oldDir.Rename(oldName, newName, newDir)
		},
	)
}

func (fsys *rootFS) Symlink(oldName, newName string) error {
	return fsys.lookup1("symlink", newName, func(dir FS, name string) error {
		return dir.Symlink(oldName, name)
	})
}

func (fsys *rootFS) Readlink(name string) (link string, err error) {
	err = fsys.lookup("readlink", name, rootfsReadlinkFlags, func(file File) (err error) {
		link, err = file.Readlink()
		return
	})
	return link, err
}

func (fsys *rootFS) Chmod(name string, mode fs.FileMode) error {
	return fsys.lookup("chmod", name, O_RDONLY, func(file File) error {
		return file.Chmod(mode)
	})
}

func (fsys *rootFS) Chtimes(name string, atime, mtime time.Time) error {
	return fsys.lookup("chtimes", name, O_RDONLY, func(file File) error {
		return file.Chtimes(atime, mtime)
	})
}

func (fsys *rootFS) Truncate(name string, size int64) error {
	return fsys.lookup("truncate", name, O_WRONLY, func(file File) error {
		return file.Truncate(size)
	})
}

func (fsys *rootFS) Stat(name string) (info fs.FileInfo, err error) {
	err = fsys.lookup("stat", name, O_RDONLY, func(file File) (err error) {
		info, err = file.Stat()
		return
	})
	return info, err
}

type rootFile struct {
	root *rootFS
	name string
	File
}

func (f *rootFile) FS() FS { return rootFileFS{f} }

type rootFileFS struct{ *rootFile }

func (d rootFileFS) lookup(op, name string, flags int, do func(File) error) error {
	if !ValidPath(name) {
		return makePathError(op, name, ErrNotExist)
	}
	f, err := d.openFile(name, flags, 0)
	if err != nil {
		return makePathError(op, name, err)
	}
	defer f.Close()
	return do(f)
}

func (d rootFileFS) lookup1(op, name string, do func(FS, string) error) error {
	if !ValidPath(name) {
		return makePathError(op, name, ErrNotExist)
	}
	dir, base := SplitPath(name)
	f, err := d.openFile(dir, O_DIRECTORY, 0)
	if err != nil {
		return makePathError(op, name, err)
	}
	defer f.Close()
	return do(f.FS(), base)
}

func (d rootFileFS) lookup2(op, name1, name2 string, do func(FS, string, FS, string) error) error {
	if !ValidPath(name1) {
		return makePathError(op, name1, ErrNotExist)
	}
	if !ValidPath(name2) {
		return makePathError(op, name2, ErrInvalid)
	}
	dir1, base1 := SplitPath(name1)
	dir2, base2 := SplitPath(name2)
	d1, err := d.openFile(dir1, O_DIRECTORY, 0)
	if err != nil {
		return makePathError(op, name1, err)
	}
	defer d1.Close()
	d2, err := d.openFile(dir2, O_DIRECTORY, 0)
	if err != nil {
		return makePathError(op, name2, err)
	}
	defer d2.Close()
	return do(d1.FS(), base1, d2.FS(), base2)
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

func (d rootFileFS) Open(name string) (fs.File, error) {
	return d.OpenFile(name, O_RDONLY, 0)
}

func (d rootFileFS) Mkdir(name string, perm fs.FileMode) error {
	return d.lookup1("mkdir", name, func(dir FS, name string) error {
		return dir.Mkdir(name, perm)
	})
}

func (d rootFileFS) Rmdir(name string) error {
	return d.lookup1("rmdir", name, FS.Rmdir)
}

func (d rootFileFS) Unlink(name string) error {
	return d.lookup1("unlink", name, FS.Unlink)
}

func (d rootFileFS) Link(oldName, newName string, newFS FS) error {
	return d.lookup2("link", oldName, newName,
		func(oldDir FS, oldName string, newDir FS, newName string) error {
			return oldDir.Link(oldName, newName, newDir)
		},
	)
}

func (d rootFileFS) Rename(oldName, newName string, newFS FS) error {
	return d.lookup2("rename", oldName, newName,
		func(oldDir FS, oldName string, newDir FS, newName string) error {
			return oldDir.Rename(oldName, newName, newDir)
		},
	)
}

func (d rootFileFS) Symlink(oldName, newName string) error {
	return d.lookup1("symlink", newName, func(dir FS, name string) error {
		return dir.Symlink(oldName, name)
	})
}

func (d rootFileFS) Readlink(name string) (link string, err error) {
	err = d.lookup("readlink", name, rootfsReadlinkFlags, func(file File) (err error) {
		link, err = file.Readlink()
		return
	})
	return link, err
}

func (d rootFileFS) Chmod(name string, mode fs.FileMode) error {
	return d.lookup("chmod", name, O_RDONLY, func(file File) error {
		return file.Chmod(mode)
	})
}

func (d rootFileFS) Chtimes(name string, atime, mtime time.Time) error {
	return d.lookup("chtimes", name, O_RDONLY, func(file File) error {
		return file.Chtimes(atime, mtime)
	})
}

func (d rootFileFS) Truncate(name string, size int64) error {
	return d.lookup("truncate", name, O_WRONLY, func(file File) error {
		return file.Truncate(size)
	})
}

func (d rootFileFS) Stat(name string) (info fs.FileInfo, err error) {
	err = d.lookup("stat", name, O_RDONLY, func(file File) (err error) {
		info, err = file.Stat()
		return
	})
	return info, err
}
