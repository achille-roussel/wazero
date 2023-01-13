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
	fs.StatFS
	Readlink(name string) (string, error)
}

// ReadOnlyFS constructs a FS from a ReadFS. The returned file system supports
// only read operations, and will return ErrReadOnly on any method call which
// attempts to mutate the state of the file system.
func ReadOnlyFS(base ReadFS) FS { return &readOnlyFS{base} }

type readOnlyFS struct{ base ReadFS }

func (fsys *readOnlyFS) Open(name string) (fs.File, error) {
	return fsys.OpenFile(name, O_RDONLY, 0)
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

	f, err := fsys.base.Open(name)
	if err != nil {
		return nil, err
	}

	if (flags & O_DIRECTORY) != 0 {
		s, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		if !s.IsDir() {
			f.Close()
			return nil, ErrNotDirectory
		}
	}

	return &readOnlyFile{fsys: fsys, base: f, name: name}, nil
}

func (fsys *readOnlyFS) Readlink(name string) (link string, err error) {
	return call1(fsys.base, "readlink", name, ReadFS.Readlink)
}

func (fsys *readOnlyFS) Stat(name string) (info fs.FileInfo, err error) {
	return call1(fsys.base, "stat", name, ReadFS.Stat)
}

func (fsys *readOnlyFS) Mkdir(name string, perm fs.FileMode) error {
	return fail("mkdir", name, ErrReadOnly)
}

func (fsys *readOnlyFS) Rmdir(name string) error {
	return fail("rmdir", name, ErrReadOnly)
}

func (fsys *readOnlyFS) Unlink(name string) error {
	return fail("unlink", name, ErrReadOnly)
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

func (fsys *readOnlyFS) Chmod(name string, mode fs.FileMode) error {
	return fail("chmod", name, ErrReadOnly)
}

func (fsys *readOnlyFS) Chtimes(name string, atime, mtime time.Time) error {
	return fail("chtimes", name, ErrReadOnly)
}

func (fsys *readOnlyFS) Truncate(name string, size int64) error {
	return call(fsys, "truncate", name, func(*readOnlyFS, string) error {
		if size < 0 {
			return ErrInvalid
		} else {
			return ErrReadOnly
		}
	})
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
	} else {
		link, err = f.fsys.Readlink(f.name)
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

func (f readOnlyFileFS) Readlink(name string) (link string, err error) {
	return callFS(f, "readlink", name, (*readOnlyFS).Readlink)
}

func (f readOnlyFileFS) Stat(name string) (info fs.FileInfo, err error) {
	return callFS(f, "stat", name, (*readOnlyFS).Stat)
}

func (f readOnlyFileFS) Mkdir(name string, perm fs.FileMode) error {
	return fail("mkdir", name, ErrReadOnly)
}

func (f readOnlyFileFS) Rmdir(name string) error {
	return fail("rmdir", name, ErrReadOnly)
}

func (f readOnlyFileFS) Unlink(name string) error {
	return fail("unlink", name, ErrReadOnly)
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

func (f readOnlyFileFS) Chmod(name string, mode fs.FileMode) error {
	return fail("chmod", name, ErrReadOnly)
}

func (f readOnlyFileFS) Chtimes(name string, atime, mtime time.Time) error {
	return fail("chtimes", name, ErrReadOnly)
}

func (f readOnlyFileFS) Truncate(name string, size int64) (err error) {
	if f.fsys == nil {
		err = ErrClosed
	} else if size < 0 {
		err = ErrInvalid
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		err = ErrReadOnly
	}
	return makePathError("truncate", name, err)
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