package sys

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// DirFS constructs a FS instance using the given path as root on the host's
// file system.
//
// The path is first converted to an obsolute path on the file system in order
// to ensure that the behavior of the returned FS does not change if the working
// directory changes.
func DirFS(path string) FS {
	path, err := filepath.Abs(path)
	if err != nil {
		return ErrFS(err)
	}
	return &dirFS{root: path}
}

type dirFS struct{ root string }

func (fsys *dirFS) Open(name string) (fs.File, error) {
	return fsys.OpenFile(name, O_RDONLY, 0)
}

func (fsys *dirFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	f, err := fsys.openFile(name, flags, perm)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (fsys *dirFS) openFile(name string, flags int, perm fs.FileMode) (*dirFile, error) {
	var f *os.File
	err := fsys.do("open", name, func(path string) (err error) {
		f, err = openFile(path, flags, perm)
		return
	})
	if err != nil {
		return nil, err
	}
	return fsys.newFile(f, name), nil
}

func (fsys *dirFS) newFile(base *os.File, name string) *dirFile {
	if base == nil {
		panic("dirFile constructed from nil os.File")
	}
	return &dirFile{fsys: fsys, base: base, name: name}
}

func (fsys *dirFS) openRoot() (*dirFile, error) {
	return fsys.openFile(".", O_DIRECTORY, 0)
}

func (fsys *dirFS) Mkdir(name string, perm fs.FileMode) error {
	return fsys.do("mkdir", name, func(path string) error { return os.Mkdir(path, perm) })
}

func (fsys *dirFS) Rmdir(name string) error {
	return fsys.do("rmdir", name, rmdir)
}

func (fsys *dirFS) Unlink(name string) error {
	return fsys.do("unlink", name, unlink)
}

func (fsys *dirFS) Link(oldName, newName string, newFS FS) error {
	return fsys.linkOrRename("link", oldName, newName, newFS, FS.Link)
}

func (fsys *dirFS) Rename(oldName, newName string, newFS FS) error {
	return fsys.linkOrRename("rename", oldName, newName, newFS, FS.Rename)
}

func (fsys *dirFS) linkOrRename(op, oldName, newName string, newFS FS, method linkOrRename) (err error) {
	switch fsys2 := newFS.(type) {
	case *dirFS:
		return fsys.linkOrRenameFS(op, oldName, newName, fsys2, method)
	case dirFileFS:
		return fsys.linkOrRenameFile(op, oldName, newName, fsys2, method)
	default:
		return makePathError(op, newName, ErrInvalid)
	}
}

func (fsys *dirFS) linkOrRenameFS(op, oldName, newName string, fsys2 *dirFS, method linkOrRename) error {
	if !ValidPath(newName) {
		return makePathError(op, newName, ErrInvalid)
	}
	newRoot, err := fsys2.openRoot()
	if err != nil {
		return makePathError(op, newName, err)
	}
	defer newRoot.Close()
	return fsys.linkOrRenameFile(op, oldName, newName, dirFileFS{newRoot}, method)
}

func (fsys *dirFS) linkOrRenameFile(op, oldName, newName string, fsys2 dirFileFS, method linkOrRename) error {
	if !ValidPath(oldName) {
		return makePathError(op, oldName, ErrNotExist)
	}
	oldRoot, err := fsys.openRoot()
	if err != nil {
		return makePathError(op, oldName, err)
	}
	defer oldRoot.Close()
	return method(dirFileFS{oldRoot}, oldName, newName, fsys2)
}

func (fsys *dirFS) Symlink(oldName, newName string) error {
	newPath, err := fsys.join(newName)
	if err != nil {
		return makePathError("symlink", newName, ErrNotExist)
	}
	if err := os.Symlink(oldName, newPath); err != nil {
		return makePathError("symlink", oldName, err)
	}
	return nil
}

func (fsys *dirFS) Chmod(name string, perm fs.FileMode) error {
	return fsys.do("chmod", name, func(path string) error { return os.Chmod(path, perm) })
}

func (fsys *dirFS) Chtimes(name string, atime, mtime time.Time) error {
	return fsys.do("chtimes", name, func(path string) error { return os.Chtimes(path, atime, mtime) })
}

func (fsys *dirFS) Stat(name string) (info fs.FileInfo, err error) {
	err = fsys.do("stat", name, func(path string) (err error) {
		info, err = os.Stat(path)
		return
	})
	return info, err
}

func (fsys *dirFS) do(op, name string, do func(string) error) error {
	path, err := fsys.join(name)
	if err != nil {
		return makePathError(op, name, err)
	}
	if err := do(path); err != nil {
		return makePathError(op, name, err)
	}
	return nil
}

func (fsys *dirFS) join(name string) (string, error) {
	if !ValidPath(name) {
		return "", ErrNotExist
	}
	name = filepath.FromSlash(name)
	name = filepath.Join(fsys.root, name)
	return name, nil
}

type dirFile struct {
	fsys *dirFS
	base *os.File
	name string
}

func (f *dirFile) Close() (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = f.base.Close()
		f.fsys = nil
		f.base = nil
	}
	if err != nil {
		err = f.makePathError("close", err)
	}
	return err
}

func (f *dirFile) Stat() (info fs.FileInfo, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		info, err = f.base.Stat()
	}
	if err != nil {
		err = f.makePathError("stat", err)
	}
	return info, err
}

func (f *dirFile) Read(b []byte) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		n, err = f.base.Read(b)
	}
	if err != nil && err != io.EOF {
		err = f.makePathError("read", err)
	}
	return n, err
}

func (f *dirFile) ReadAt(b []byte, offset int64) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		n, err = f.base.ReadAt(b, offset)
	}
	if err != nil && err != io.EOF {
		err = f.makePathError("read", err)
	}
	return n, err
}

func (f *dirFile) ReadFrom(r io.Reader) (n int64, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		n, err = f.base.ReadFrom(r)
	}
	if err != nil {
		err = f.makePathError("write", err)
	}
	return n, err
}

func (f *dirFile) Write(b []byte) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		n, err = f.base.Write(b)
	}
	if err != nil {
		err = f.makePathError("write", err)
	}
	return n, err
}

func (f *dirFile) WriteAt(b []byte, offset int64) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		n, err = f.base.WriteAt(b, offset)
	}
	if err != nil {
		err = f.makePathError("write", err)
	}
	return n, err
}

func (f *dirFile) WriteString(s string) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		n, err = f.base.WriteString(s)
	}
	if err != nil {
		err = f.makePathError("write", err)
	}
	return n, err
}

func (f *dirFile) Seek(offset int64, whence int) (seek int64, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		seek, err = f.base.Seek(offset, whence)
	}
	if err != nil {
		err = f.makePathError("seek", err)
	}
	return seek, err
}

func (f *dirFile) ReadDir(n int) (files []fs.DirEntry, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		files, err = f.base.ReadDir(n)
	}
	if err != nil && err != io.EOF {
		err = f.makePathError("readdir", err)
	}
	return files, err
}

func (f *dirFile) Readlink() (link string, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		link, err = readlink(f.base)
	}
	if err != nil {
		err = f.makePathError("readlink", err)
	}
	return link, err
}

func (f *dirFile) Chmod(perm fs.FileMode) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = f.base.Chmod(perm)
	}
	if err != nil {
		err = f.makePathError("chmod", err)
	}
	return err
}

func (f *dirFile) Chtimes(atime, mtime time.Time) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = chtimes(f.base, atime, mtime)
	}
	if err != nil {
		err = f.makePathError("chtimes", err)
	}
	return err
}

func (f *dirFile) Truncate(size int64) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = f.base.Truncate(size)
	}
	if err != nil {
		err = f.makePathError("truncate", err)
	}
	return err
}

func (f *dirFile) Sync() (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = f.base.Sync()
	}
	if err != nil {
		err = f.makePathError("sync", err)
	}
	return err
}

func (f *dirFile) Datasync() (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = datasync(f.base)
	}
	if err != nil {
		err = f.makePathError("datasync", err)
	}
	return err
}

func (f *dirFile) makePathError(op string, err error) error {
	return makePathError(op, f.name, err)
}

func (f *dirFile) FS() FS { return dirFileFS{f} }

type dirFileFS struct{ *dirFile }

func (d dirFileFS) OpenFile(name string, flags int, perm fs.FileMode) (f File, err error) {
	if d.base == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		f, err = d.openFile(name, flags, perm)
	}
	if err != nil {
		err = makePathError("open", name, err)
	}
	return f, err
}

func (d dirFileFS) Open(name string) (fs.File, error) {
	return d.OpenFile(name, O_RDONLY, 0)
}

func (d dirFileFS) Mkdir(name string, perm fs.FileMode) (err error) {
	if d.base == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		err = d.mkdir(name, perm)
	}
	if err != nil {
		err = makePathError("mkdir", name, err)
	}
	return err
}

func (d dirFileFS) Rmdir(name string) (err error) {
	if d.base == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		err = d.rmdir(name)
	}
	if err != nil {
		err = makePathError("rmdir", name, err)
	}
	return err
}

func (d dirFileFS) Unlink(name string) (err error) {
	if d.base == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		err = d.unlink(name)
	}
	if err != nil {
		err = makePathError("unlink", name, err)
	}
	return err
}

func (d dirFileFS) Link(oldName, newName string, newFS FS) (err error) {
	if d.base == nil {
		err = ErrClosed
	} else if d2, ok := newFS.(dirFileFS); !ok {
		err = ErrInvalid
	} else if d2.base == nil {
		err = ErrInvalid
	} else if !ValidPath(oldName) {
		err = ErrNotExist
	} else if !ValidPath(newName) {
		err = ErrInvalid
	} else {
		err = d.link(oldName, newName, d2)
	}
	if err != nil {
		err = makePathError("link", newName, err)
	}
	return err
}

func (d dirFileFS) Rename(oldName, newName string, newFS FS) (err error) {
	if d.base == nil {
		err = ErrClosed
	} else if d2, ok := newFS.(dirFileFS); !ok {
		err = ErrInvalid
	} else if d2.base == nil {
		err = ErrInvalid
	} else if !ValidPath(oldName) {
		err = ErrNotExist
	} else if !ValidPath(newName) {
		err = ErrInvalid
	} else {
		err = d.rename(oldName, newName, d2)
	}
	if err != nil {
		err = makePathError("rename", newName, err)
	}
	return err
}

func (d dirFileFS) Symlink(oldName, newName string) (err error) {
	if d.base == nil {
		err = ErrClosed
	} else if !ValidPath(newName) {
		err = ErrNotExist
	} else {
		err = d.symlink(oldName, newName)
	}
	if err != nil {
		err = makePathError("symlink", newName, err)
	}
	return err
}

func (d dirFileFS) Chmod(name string, perm fs.FileMode) (err error) {
	if d.base == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		err = d.chmod(name, perm)
	}
	if err != nil {
		err = makePathError("chmod", name, err)
	}
	return err
}

func (d dirFileFS) Chtimes(name string, atime, mtime time.Time) (err error) {
	if d.base == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		err = d.chtimes(name, atime, mtime)
	}
	if err != nil {
		err = makePathError("chtimes", name, err)
	}
	return err
}

func (d dirFileFS) Stat(name string) (info fs.FileInfo, err error) {
	if d.base == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		info, err = d.stat(name)
	}
	if err != nil {
		err = makePathError("stat", name, err)
	}
	return info, err
}

var (
	_ io.ReaderFrom   = (*dirFile)(nil)
	_ io.StringWriter = (*dirFile)(nil)
)
