package sys

import (
	"io"
	"io/fs"
	"path"
	"time"
)

// ReadFS is a subset of the FS interface implemented by file systems which
// support only read operations.
type ReadFS interface {
	fs.FS
	OpenFile(name string, flags int, perm fs.FileMode) (File, error)
}

// ReadOnlyFS constructs a FS from a ReadFS. The returned file system supports
// only read operations, and will return ErrReadOnly on any method call which
// attempts to mutate the state of the file system.
func ReadOnlyFS(base ReadFS) FS { return &readOnlyFS{base} }

type readOnlyFS struct{ base ReadFS }

func (fsys *readOnlyFS) Open(name string) (fs.File, error) {
	return Open(fsys, name)
}

func (fsys *readOnlyFS) OpenFile(name string, flags int, _ fs.FileMode) (File, error) {
	f, err := fsys.openFile(name, flags)
	if err != nil {
		return nil, makePathError("open", name, err)
	}
	return f, nil
}

func (fsys *readOnlyFS) openFile(name string, flags int) (*readOnlyFile, error) {
	if !ValidPath(name) {
		return nil, ErrNotExist
	}
	if (flags & ^openFileReadOnlyFlags) != 0 {
		return nil, ErrReadOnly
	}
	f, err := fsys.base.OpenFile(name, flags, 0)
	if err != nil {
		return nil, err
	}
	if r, ok := f.(*readOnlyFile); ok {
		r.fsys = fsys
		r.name = name
		return r, nil
	}
	return &readOnlyFile{fsys: fsys, base: f, name: name}, nil
}

func (fsys *readOnlyFS) Link(oldName, newName string, newFS FS) error {
	return fail2("link", oldName, newName, ErrReadOnly)
}

func (fsys *readOnlyFS) Rename(oldName, newName string, newFS FS) error {
	return fail2("rename", oldName, newName, ErrReadOnly)
}

func (fsys *readOnlyFS) Symlink(oldName, newName string) error {
	return fail("symlink", newName, ErrReadOnly)
}

func (fsys *readOnlyFS) fail(op, name string, err error) error {
	if !ValidPath(name) {
		err = ErrNotExist
	}
	return makePathError(op, name, err)
}

func (fsys *readOnlyFS) fail2(op, oldName, newName string, err error) error {
	var name string
	if !ValidPath(newName) {
		name, err = newName, ErrInvalid
	} else if !ValidPath(oldName) {
		name, err = oldName, ErrNotExist
	} else {
		name = newName
	}
	return makePathError(op, name, err)
}

type readOnlyFile struct {
	fsys *readOnlyFS
	base fs.File
	name string
}

func (f *readOnlyFile) Close() (err error) {
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

func (f *readOnlyFile) Stat() (info fs.FileInfo, err error) {
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

func (f *readOnlyFile) Read(b []byte) (n int, err error) {
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

func (f *readOnlyFile) ReadAt(b []byte, offset int64) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else if r, ok := f.base.(io.ReaderAt); ok {
		n, err = r.ReadAt(b, offset)
	} else {
		err = ErrNotSupported
	}
	if err != nil && err != io.EOF {
		err = f.makePathError("read", err)
	}
	return n, err
}

func (f *readOnlyFile) Write(b []byte) (int, error) {
	return 0, f.fail("write", ErrNotSupported)
}

func (f *readOnlyFile) WriteAt(b []byte, offset int64) (int, error) {
	return 0, f.fail("write", ErrNotSupported)
}

func (f *readOnlyFile) WriteString(s string) (int, error) {
	return 0, f.fail("write", ErrNotSupported)
}

func (f *readOnlyFile) ReadFrom(r io.Reader) (int64, error) {
	return 0, f.fail("write", ErrNotSupported)
}

func (f *readOnlyFile) Seek(offset int64, whence int) (seek int64, err error) {
	if f.base == nil {
		err = ErrClosed
	} else if r, ok := f.base.(io.Seeker); ok {
		seek, err = r.Seek(offset, whence)
	} else {
		err = ErrNotSupported
	}
	if err != nil {
		err = f.makePathError("seek", err)
	}
	return seek, err
}

func (f *readOnlyFile) ReadDir(n int) (files []fs.DirEntry, err error) {
	if f.base == nil {
		err = ErrClosed
	} else if d, ok := f.base.(fs.ReadDirFile); ok {
		files, err = d.ReadDir(n)
	} else {
		err = ErrNotSupported
	}
	if err != nil && err != io.EOF {
		err = f.makePathError("readdir", err)
	}
	return files, err
}

func (f *readOnlyFile) Readlink() (link string, err error) {
	if f.base == nil {
		err = ErrClosed
	} else if r, ok := f.base.(interface{ Readlink() (string, error) }); ok {
		link, err = r.Readlink()
	} else if s, e := f.base.Stat(); e != nil {
		err = e
	} else if s.Mode().Type() != fs.ModeSymlink {
		err = ErrInvalid
	} else if b, e := io.ReadAll(f.base); e != nil {
		err = e
	} else {
		link = string(b)
	}
	if err != nil {
		err = f.makePathError("readlink", err)
	}
	return link, err
}

func (f *readOnlyFile) Chmod(mode fs.FileMode) error {
	return f.fail("chmod", ErrReadOnly)
}

func (f *readOnlyFile) Chtimes(atime, mtime time.Time) error {
	return f.fail("chtimes", ErrReadOnly)
}

func (f *readOnlyFile) Truncate(size int64) error {
	return f.do("truncate", func() error {
		if size < 0 {
			return ErrInvalid
		} else {
			return ErrReadOnly
		}
	})
}

func (f *readOnlyFile) Sync() error {
	return f.fail("sync", ErrReadOnly)
}

func (f *readOnlyFile) Datasync() error {
	return f.fail("datasync", ErrReadOnly)
}

func (f *readOnlyFile) Mkdir(name string, perm fs.FileMode) error {
	return f.fail("mkdir", ErrReadOnly)
}

func (f *readOnlyFile) Rmdir(name string) error {
	return f.fail("rmdir", ErrReadOnly)
}

func (f *readOnlyFile) Unlink(name string) error {
	return f.fail("unlink", ErrReadOnly)
}

func (f *readOnlyFile) fail(op string, err error) error {
	return f.do(op, func() error { return err })
}

func (f *readOnlyFile) do(op string, do func() error) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = do()
	}
	return f.makePathError(op, err)
}

func (f *readOnlyFile) makePathError(op string, err error) error {
	return makePathError(op, f.name, err)
}
func (f *readOnlyFile) FS() FS { return readOnlyFileFS{f} }

type readOnlyFileFS struct{ *readOnlyFile }

func (f readOnlyFileFS) Open(name string) (fs.File, error) {
	return f.OpenFile(name, O_RDONLY, 0)
}

func (f readOnlyFileFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	return callFS(f, "open", name, func(fsys *readOnlyFS, path string) (File, error) {
		return fsys.OpenFile(path, flags, perm)
	})
}

func (f readOnlyFileFS) Link(oldName, newName string, newFS FS) error {
	return fail2("link", oldName, newName, ErrReadOnly)
}

func (f readOnlyFileFS) Rename(oldName, newName string, newFS FS) error {
	return fail2("rename", oldName, newName, ErrReadOnly)
}

func (f readOnlyFileFS) Symlink(oldName, newName string) error {
	return fail("symlink", newName, ErrReadOnly)
}

func callFS[Func func(*readOnlyFS, string) (Ret, error), Ret any](f readOnlyFileFS, op, name string, do Func) (ret Ret, err error) {
	if f.fsys == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		ret, err = do(f.fsys, path.Join(f.name, name))
	}
	if err != nil {
		err = makePathError(op, name, err)
	}
	return ret, err
}

func fail(op, name string, err error) error {
	if !ValidPath(name) {
		err = ErrNotExist
	}
	return makePathError(op, name, err)
}

func fail2(op, oldName, newName string, err error) error {
	var name string
	if !ValidPath(newName) {
		name, err = newName, ErrInvalid
	} else if !ValidPath(oldName) {
		name, err = oldName, ErrNotExist
	} else {
		name = newName
	}
	return makePathError(op, name, err)
}

var (
	_ io.ReaderFrom   = (*readOnlyFile)(nil)
	_ io.StringWriter = (*readOnlyFile)(nil)
)
