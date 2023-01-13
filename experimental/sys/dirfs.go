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
	return Open(fsys, name)
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

func (f *dirFile) Fd() uintptr {
	if f.base != nil {
		return f.base.Fd()
	}
	return ^uintptr(0)
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

func (f *dirFile) Mkdir(name string, perm fs.FileMode) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		err = f.mkdir(name, perm)
	}
	if err != nil {
		err = makePathError("mkdir", name, err)
	}
	return err
}

func (f *dirFile) Rmdir(name string) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		err = f.rmdir(name)
	}
	if err != nil {
		err = makePathError("rmdir", name, err)
	}
	return err
}

func (f *dirFile) Unlink(name string) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		err = f.unlink(name)
	}
	if err != nil {
		err = f.makePathError("unlink", err)
	}
	return err
}

func (f *dirFile) Symlink(oldName, newName string) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else if !ValidPath(newName) {
		err = ErrNotExist
	} else {
		err = f.symlink(oldName, newName)
	}
	if err != nil {
		err = makePathError("symlink", newName, err)
	}
	return err
}

func (f *dirFile) Link(oldName string, newDir Directory, newName string) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else if !ValidPath(oldName) {
		err = ErrNotExist
	} else if !ValidPath(newName) {
		err = ErrInvalid
	} else {
		err = f.link(oldName, newDir.Fd(), newName)
	}
	if err != nil {
		err = makePathError("link", newName, err)
	}
	return err
}

func (f *dirFile) Rename(oldName string, newDir Directory, newName string) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else if !ValidPath(oldName) {
		err = ErrNotExist
	} else if !ValidPath(newName) {
		err = ErrInvalid
	} else {
		err = f.rename(oldName, newDir.Fd(), newName)
	}
	if err != nil {
		err = makePathError("rename", newName, err)
	}
	return err
}

func (f *dirFile) makePathError(op string, err error) error {
	return makePathError(op, f.name, err)
}

func (f *dirFile) FS() FS { return dirFileFS{f} }

type dirFileFS struct{ *dirFile }

func (d dirFileFS) Open(name string) (fs.File, error) {
	return Open(d, name)
}

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

var (
	_ io.ReaderFrom   = (*dirFile)(nil)
	_ io.StringWriter = (*dirFile)(nil)
)
