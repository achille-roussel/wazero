package sysfs

import (
	"errors"
	"io"
	"io/fs"
	fspath "path"
	"path/filepath"
	"syscall"
	"time"
)

// File is an interface representing nodes in a file system tree.
//
// File instances may be regular files, directories, or any other type of files
// supported by the file system that they were obtained from.
//
// For methods that accept a path as argument, the path is always resolved
// relative to the location of the receiver in the file system. All paths must
// be valid according to the rules implemented by fs.ValidPath, and when given
// an invalid path, the methods return an error matching fs.ErrInvalid.
//
// Except for io.EOF, all errors returned by methods of this interface must be
// of type *fs.PathError, wrapping the underlying error cause.
//
// After closing a file, all methods return an error matching fs.ErrClosed.
type File interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Writer
	io.WriterAt
	io.Seeker
	fs.ReadDirFile
	// Returns the name of the file in the file system.
	//
	// For the root directory, this method returns "." so the value returned by
	// this method is always a valid argument to Open.
	Name() string
	// Changes the file permissions.
	Chmod(perm fs.FileMode) error
	// Changes the file access and modification times.
	Chtimes(atim, mtim time.Time) error
	// Opens a file at the given path.
	//
	// The flags must be a bitwise combination of the O_* constants defined in
	// this package.
	//
	// The receiver must be a directory.
	Open(path string, flags int, perm fs.FileMode) (File, error)
	// Removes the file at the given path.
	Unlink(path string) error
	// Moves a file from oldPath to newPath.
	Rename(oldPath, newPath string) error
	// Creates a link from oldPath to newPath.
	Link(oldPath, newPath string) error
	// Creates a symbolic link to oldPath at newPath.
	Symlink(oldPath, newPath string) error
	// Returns the value of the symbolic link at the given path.
	Readlink(path string) (string, error)
	// Creates a directory at the given path.
	Mkdir(path string, perm fs.FileMode) error
	// Removes the directory at the given path. The directory must be empty
	// prior to calling this method.
	Rmdir(path string) error
	// Returns a fs.FileInfo containing metadata about the receiver.
	Stat() (fs.FileInfo, error)
	// Blocks until all cached changes have been successfully written to
	// persistent storage.
	Sync() error
	// Blocks until all cached data changes have been successfully written to
	// persistent storage.
	Datasync() error
}

// FS is an extension of the standard fs.FS allowing programs to open writable
// files and directories.
type FS interface {
	fs.FS
	// OpenFile is a method similar to Open but it accepts a bitwise combination
	// of O_* constants to configure how the file is open, and a permission mask
	// used when creating a file.
	OpenFile(name string, flags int, perm fs.FileMode) (File, error)
}

// NewFS constructs a FS from a standard fs.FS value.
//
// Since fs.FS only supports read-only operations, all files opened from the
// returned file system only permit read operations as well.
func NewFS(base fs.FS) FS { return &fsFS{base: base} }

type fsFS struct{ base fs.FS }

func (fsys *fsFS) Open(name string) (fs.File, error) { return fsys.OpenFile(name, 0, 0) }

func (fsys *fsFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	if (flags & (O_WRONLY | O_RDWR | O_CREATE | O_TRUNC | O_APPEND)) != 0 {
		return nil, makePathError("open", name, fs.ErrInvalid)
	}
	f, err := fsys.base.Open(name)
	if err != nil {
		return nil, err
	}
	if (flags & O_DIRECTORY) != 0 {
		if err := checkIsDir(f); err != nil {
			f.Close()
			return nil, err
		}
	}
	return &fsFile{fsys: fsys, base: f, name: name}, nil
}

type fsFile struct {
	fsys *fsFS
	base fs.File
	name string
}

func (f *fsFile) makeFileError(op string, err error) error {
	return makePathError(op, f.name, err)
}

func (f *fsFile) makePathError(op, path string, err error) error {
	return makePathError(op, f.pathTo(path), err)
}

func (f *fsFile) pathTo(path string) string {
	return fspath.Join(f.name, path)
}

func (f *fsFile) Name() string {
	return f.name
}

func (f *fsFile) Close() (err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else {
		base := f.base
		f.base = nil
		err = base.Close()
	}
	if err != nil {
		err = f.makeFileError("close", err)
	}
	return err
}

func (f *fsFile) ReadDir(n int) (ent []fs.DirEntry, err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else if d, ok := f.base.(fs.ReadDirFile); ok {
		ent, err = d.ReadDir(n)
	} else {
		err = coalesce(checkIsDir(f.base), fs.ErrInvalid)
	}
	if err != nil && err != io.EOF {
		err = f.makeFileError("readdir", err)
	}
	return ent, err
}

func (f *fsFile) Read(b []byte) (n int, err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else {
		n, err = f.base.Read(b)
	}
	if err != nil && err != io.EOF {
		if f.base != nil {
			err = coalesce(checkIsFile(f.base), err)
		}
		err = f.makeFileError("read", err)
	}
	return n, err
}

func (f *fsFile) ReadAt(b []byte, off int64) (n int, err error) {
	switch r := f.base.(type) {
	case io.ReaderAt:
		n, err = r.ReadAt(b, off)
	case io.ReadSeeker:
		oldOffset, err := r.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, f.makeFileError("read", err)
		}
		if _, err := r.Seek(off, io.SeekStart); err != nil {
			return 0, f.makeFileError("read", err)
		}
		n, err = r.Read(b)
		_, seekErr := r.Seek(oldOffset, io.SeekStart)
		if seekErr != nil {
			err = seekErr
		}
	case nil:
		err = fs.ErrClosed
	default:
		err = fs.ErrInvalid
	}
	if err != nil && err != io.EOF {
		if f.base != nil {
			err = coalesce(checkIsFile(f.base), err)
		}
		err = f.makeFileError("read", err)
	}
	return n, err
}

func (f *fsFile) Write(b []byte) (n int, err error) {
	switch {
	case f.base == nil:
		err = fs.ErrClosed
	case checkIsFile(f.base) != nil:
		err = syscall.EBADF
	default:
		err = fs.ErrInvalid
	}
	return 0, f.makeFileError("write", err)
}

func (f *fsFile) WriteAt(b []byte, off int64) (n int, err error) {
	switch {
	case f.base == nil:
		err = fs.ErrClosed
	case checkIsFile(f.base) != nil:
		err = syscall.EBADF
	default:
		err = fs.ErrInvalid
	}
	return 0, f.makeFileError("write", err)
}

func (f *fsFile) Seek(offset int64, whence int) (int64, error) {
	var err error
	switch s := f.base.(type) {
	case io.Seeker:
		offset, err = s.Seek(offset, whence)
	case nil:
		err = fs.ErrClosed
	default:
		err = fs.ErrInvalid
	}
	if err != nil {
		if f.base != nil {
			err = coalesce(checkIsFile(f.base), err)
		}
		err = f.makeFileError("seek", err)
	}
	return offset, err
}

func (f *fsFile) Chmod(perm fs.FileMode) (err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else {
		err = fs.ErrInvalid
	}
	return f.makeFileError("chmod", err)
}

func (f *fsFile) Chtimes(atim, mtim time.Time) (err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else {
		err = fs.ErrInvalid
	}
	return f.makeFileError("chtimes", err)
}

func (f *fsFile) Open(path string, flags int, perm fs.FileMode) (open File, err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else if path = fspath.Clean(path); path == "." {
		// If the program asks to open the file itself, it must only succeed
		// when it is a directory.
		err = checkIsDir(f.base)
	}
	if err == nil {
		open, err = f.fsys.OpenFile(f.pathTo(path), flags, perm)
	}
	if err != nil {
		// Some file systems may return this error when attempting to open
		// a file which is not a directory, so we normalize the error.
		if errors.Is(err, fs.ErrNotExist) {
			err = coalesce(checkIsDir(f.base), err)
		}
		err = f.makePathError("open", path, err)
	}
	return open, err
}

func (f *fsFile) Unlink(path string) (err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else {
		err = coalesce(checkIsDir(f.base), fs.ErrInvalid)
	}
	return f.makePathError("unlink", path, err)
}

func (f *fsFile) Rename(oldPath, newPath string) (err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else {
		err = coalesce(checkIsDir(f.base), fs.ErrInvalid)
	}
	return f.makePathError("rename", oldPath, err)
}

func (f *fsFile) Link(oldPath, newPath string) (err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else {
		err = coalesce(checkIsDir(f.base), fs.ErrInvalid)
	}
	return f.makePathError("link", oldPath, err)
}

func (f *fsFile) Symlink(oldPath, newPath string) (err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else {
		err = coalesce(checkIsDir(f.base), fs.ErrInvalid)
	}
	return f.makePathError("symlink", oldPath, err)
}

func (f *fsFile) Readlink(path string) (link string, err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else {
		err = coalesce(checkIsDir(f.base), fs.ErrInvalid)
	}
	return "", f.makePathError("readlink", path, err)
}

func (f *fsFile) Mkdir(path string, perm fs.FileMode) (err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else {
		err = coalesce(checkIsDir(f.base), fs.ErrInvalid)
	}
	return f.makePathError("mkdir", path, err)
}

func (f *fsFile) Rmdir(path string) (err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else {
		err = coalesce(checkIsDir(f.base), fs.ErrInvalid)
	}
	return f.makePathError("rmdir", path, err)
}

func (f *fsFile) Stat() (stat fs.FileInfo, err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else {
		stat, err = f.base.Stat()
	}
	if err != nil {
		err = f.makeFileError("stat", err)
	}
	return stat, err
}

func (f *fsFile) Sync() (err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else {
		err = fs.ErrInvalid
	}
	return f.makeFileError("sync", err)
}

func (f *fsFile) Datasync() (err error) {
	if f.base == nil {
		err = fs.ErrClosed
	} else {
		err = fs.ErrInvalid
	}
	return f.makeFileError("datasync", err)
}

// DirFS is similar to os.DirFS but returns a FS instance.
//
// The path given as argument is converted to an absolute path so later calls to
// Open and OpenFile are not dependent on the working directory.
//
// On POSIX platforms supporting it, the implementation uses *at versions of the
// syscalls to implement path resolution relative to the File instances obtained
// from the file system (e.g. openat, linkat, etc...). On platforms that do not
// have these functions, the behavior is emulated by performing path resolution
// in user-space and referencing files with their absolute path. Because of it,
// the program might behave differently if concurrent changes to the same
// directory tree are performed by other processes.
func DirFS(path string) (FS, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return &dirFS{root: path}, nil
}

type dirFS struct{ root string }

func (fsys *dirFS) Open(name string) (fs.File, error) { return fsys.OpenFile(name, 0, 0) }

func (fsys *dirFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	if !fs.ValidPath(name) {
		return nil, makePathError("open", name, fs.ErrInvalid)
	}
	root, err := opendir(fsys.root)
	if err != nil {
		return nil, makePathError("open", name, err)
	}
	if name == "." {
		return root, nil
	}
	defer root.Close()
	return root.Open(name, flags, perm)
}

func checkIsDir(f interface{ Stat() (fs.FileInfo, error) }) error {
	s, err := f.Stat()
	if err != nil {
		return err
	}
	if !s.IsDir() {
		return syscall.ENOTDIR
	}
	return nil
}

func checkIsFile(f interface{ Stat() (fs.FileInfo, error) }) error {
	s, err := f.Stat()
	if err != nil {
		return err
	}
	if s.IsDir() {
		return syscall.EISDIR
	}
	return nil
}

func coalesce(err1, err2 error) error {
	if err1 != nil {
		return err1
	}
	return err2
}

func makePathError(op, path string, err error) error {
	switch e := err.(type) {
	case *fs.PathError:
		if e.Op == op && e.Path == path {
			return e
		}
	}
	if cause := errors.Unwrap(err); cause != nil {
		err = cause
	}
	return &fs.PathError{Op: op, Path: path, Err: err}
}
