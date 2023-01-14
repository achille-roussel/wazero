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

func (fsys *readOnlyFS) openFile(name string, flags int, perm fs.FileMode) (File, error) {
	if !ValidPath(name) {
		return nil, ErrNotExist
	}
	if !hasReadOnlyFlags(flags) {
		return nil, ErrReadOnly
	}
	f, err := fsys.base.OpenFile(name, flags, perm)
	if err != nil {
		return nil, err
	}
	return ReadOnlyFile(f, name, fsys), nil
}

// ReadOnlyFile constructs a read-only file.
//
// The FS is used to open files when the file's OpenFile method is called.
// The FS might be nil, in which case the file attempts to fallback to its own
// OpenFile method (if any exists), or fails with ErrNotSupported.
func ReadOnlyFile(file fs.File, name string, fsys FS) File {
	return &readOnlyFile{fsys: fsys, base: file, name: name}
}

type readOnlyFile struct {
	fsys FS
	base fs.File
	name string
}

func (f *readOnlyFile) Fd() uintptr {
	if file, ok := f.base.(interface{ Fd() uintptr }); ok {
		return file.Fd()
	}
	return ^uintptr(0)
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
	if f.base == nil {
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

func (f *readOnlyFile) OpenFile(name string, flags int, perm fs.FileMode) (file File, err error) {
	if f.fsys == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else if !hasReadOnlyFlags(flags) {
		err = ErrReadOnly
	} else if f.fsys != nil {
		path := JoinPath(f.name, name)
		fsys := readOnlyFS{f.fsys}
		file, err = fsys.OpenFile(path, flags, perm)
	} else if dir, ok := f.base.(interface {
		OpenFile(string, int, fs.FileMode) (File, error)
	}); ok {
		file, err = dir.OpenFile(name, flags, perm)
	} else {
		err = ErrNotSupported
	}
	if err != nil {
		err = makePathError("open", name, err)
	}
	return file, err
}

func (f *readOnlyFile) fail(op string, err error) error {
	if f.base == nil {
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

func hasReadOnlyFlags(flags int) bool {
	return (flags & ^openFileReadOnlyFlags) == 0
}
