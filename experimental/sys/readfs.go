package sys

import (
	"io"
	"io/fs"
	"time"
)

// ReadOnlyFS constructs a FS from a FS. The returned file system supports
// only read operations, and will return ErrReadOnly on any method call which
// attempts to mutate the state of the file system.
func ReadOnlyFS(base FS) FS { return &readOnlyFS{base} }

type readOnlyFS struct{ base FS }

func (fsys *readOnlyFS) Open(name string) (fs.File, error) { return Open(fsys, name) }

func (fsys *readOnlyFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	f, err := fsys.openFile(name, flags, perm)
	if err != nil {
		return nil, makePathError("open", name, err)
	}
	return f, nil
}

func (fsys *readOnlyFS) openFile(name string, flags int, perm fs.FileMode) (*readOnlyFile, error) {
	if !ValidPath(name) {
		return nil, ErrNotExist
	}
	if (flags & ^openFileReadOnlyFlags) != 0 {
		return nil, ErrReadOnly
	}
	f, err := fsys.base.OpenFile(name, flags, perm)
	if err != nil {
		return nil, err
	}
	return fsys.newFile(f, name), nil
}

func (fsys *readOnlyFS) newFile(file File, name string) *readOnlyFile {
	return &readOnlyFile{fsys: fsys, name: name, File: file}
}

type readOnlyFile struct {
	fsys *readOnlyFS
	name string
	File
}

func (f *readOnlyFile) Close() (err error) {
	f.fsys = nil
	return f.File.Close()
}

func (f *readOnlyFile) Write([]byte) (int, error) {
	return 0, f.fail("write", ErrNotSupported)
}

func (f *readOnlyFile) WriteAt([]byte, int64) (int, error) {
	return 0, f.fail("write", ErrNotSupported)
}

func (f *readOnlyFile) WriteString(string) (int, error) {
	return 0, f.fail("write", ErrNotSupported)
}

func (f *readOnlyFile) ReadFrom(io.Reader) (int64, error) {
	return 0, f.fail("write", ErrNotSupported)
}

func (f *readOnlyFile) Chmod(fs.FileMode) error {
	return f.fail("chmod", ErrReadOnly)
}

func (f *readOnlyFile) Chtimes(time.Time, time.Time) error {
	return f.fail("chtimes", ErrReadOnly)
}

func (f *readOnlyFile) Truncate(size int64) (err error) {
	if f.fsys == nil {
		err = ErrClosed
	} else if size < 0 {
		err = ErrInvalid
	} else {
		err = ErrReadOnly
	}
	return f.makePathError("truncate", err)
}

func (f *readOnlyFile) Sync() error {
	return f.fail("sync", ErrReadOnly)
}

func (f *readOnlyFile) Datasync() error {
	return f.fail("datasync", ErrReadOnly)
}

func (f *readOnlyFile) Mkdir(string, fs.FileMode) error {
	return f.fail("mkdir", ErrReadOnly)
}

func (f *readOnlyFile) Rmdir(string) error {
	return f.fail("rmdir", ErrReadOnly)
}

func (f *readOnlyFile) Unlink(string) error {
	return f.fail("unlink", ErrReadOnly)
}

func (f *readOnlyFile) Symlink(string, string) error {
	return f.fail("symlink", ErrReadOnly)
}

func (f *readOnlyFile) Link(string, Directory, string) error {
	return f.fail("link", ErrReadOnly)
}

func (f *readOnlyFile) Rename(string, Directory, string) error {
	return f.fail("rename", ErrReadOnly)
}

func (f *readOnlyFile) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	if f.fsys == nil {
		return nil, makePathError("open", name, ErrClosed)
	}
	if (flags & ^openFileReadOnlyFlags) != 0 {
		return nil, makePathError("open", name, ErrReadOnly)
	}
	newFile, err := f.File.OpenFile(name, flags, perm)
	if err != nil {
		return nil, err
	}
	return f.fsys.newFile(newFile, JoinPath(f.name, name)), nil
}

func (f *readOnlyFile) fail(op string, err error) error {
	if f.fsys == nil {
		err = ErrClosed
	}
	return f.makePathError(op, err)
}

func (f *readOnlyFile) makePathError(op string, err error) error {
	return makePathError(op, f.name, err)
}

var (
	_ io.ReaderFrom   = (*readOnlyFile)(nil)
	_ io.StringWriter = (*readOnlyFile)(nil)
)
