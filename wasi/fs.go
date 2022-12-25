package wasi

import (
	"errors"
	"io"
	"io/fs"
	"os"
	fspath "path"
	"path/filepath"
	"time"
)

var (
	// ErrNotImplemented is returned by FS or File methods when the underlying
	// type does not provide an implementation for the method being called.
	ErrNotImplemented = errors.New("not implemented")
	// ErrReadOnly is returned by FS or File methods when the underlying file
	// system only allows read operations.
	ErrReadOnly = errors.New("read only")
)

// File is an interface implemented by files opened by FS instsances.
//
// The interfance is similar to fs.File, it may represent different types of
// files, including regular files and directories.
//
// See FS for more details.
type File interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Writer
	io.WriterAt
	io.Seeker
	fs.ReadDirFile
	// Name returns the base name of the receiver.
	Name() string
	// OpenFile opens a file at the given path, relative to the receiver.
	OpenFile(path string, flags int, perm fs.FileMode) (File, error)
	// StatFile returns information about the file at the given path, relative
	// to the receiver.
	StatFile(path string, flags int) (fs.FileInfo, error)
	// MakeDir creates a file at the given path, relative to the receiver.
	MakeDir(path string, perm fs.FileMode) error
	// Chtimes sets the access and modification time of the receiver.
	Chtimes(atim, mtim time.Time) error
	// ChtimesFile sets the access and modification time of the file at the given
	// path, relative to the reciever.
	ChtimesFile(path string, flags int, atim, mtim time.Time) error
}

// FS is an interface satisfied by types that implement file systems compatible
// with the WASI standard.
//
// FS is similar to the standard fs.FS but makes a few different trade offs:
//
// - Instead of having a very small interface and offering extensions by
//   implementing extra interfaces, the FS and File interfaces declare methods
//   for each WASI function featuring WASI file system capabilities.
//   Implementations of the interfaces which do not support certain methods must
//   return ErrNotImplemented. This design decision helps leverage the Go type
//   system to verify that all WASI functions are implemented throughout layers
//   of abstraction.
//
// - While fs.FS only defines support for read-only use cases, the FS interface
//   is intended to also support write use cases since programs targetting WASI
//   may need to perform write operations to their file system.
//
// Note that FS also implements the fs.FS interface as a compatibility mechanism
// with code designed to work with the standard file system interface.
type FS interface {
	fs.StatFS
	// OpenFile is a method similar to Open but it returns a wasi.File which may
	// allow write operations (depending on flags).
	OpenFile(path string, flags int, perm fs.FileMode) (File, error)
	// StatFile is a method similar to Stat but it allows passing flags to
	// configure the behavior of the path lookup.
	StatFile(path string, flags int) (fs.FileInfo, error)
	// MakeDir creates a directory at the given path.
	MakeDir(path string, perm fs.FileMode) error
	// ChtimesFile sets the access and modification time at the given path.
	Chtimes(path string, flags int, atim, mtim time.Time) error
}

// NewFS constructs a FS from a standard fs.FS which only permits read
// operations.
//
// If base is nil, the returned file system contains nothing and returns
// fs.ErrNotExist on all attempts to open files.
func NewFS(base fs.FS) FS { return &fsFS{base: base} }

type fsFS struct{ base fs.FS }

func (fsys *fsFS) Open(path string) (fs.File, error) {
	return fsys.OpenFile(path, 0, 0)
}

func (fsys *fsFS) OpenFile(path string, flags int, perm fs.FileMode) (File, error) {
	if (flags & (O_WRONLY | O_RDWR | O_CREATE | O_TRUNC | O_APPEND)) != 0 {
		return nil, ErrReadOnly
	}
	if fsys.base == nil {
		return nil, fs.ErrNotExist
	}
	f, err := fsys.base.Open(path)
	if err != nil {
		return nil, err
	}
	return &fsFile{fsys: fsys, base: f, path: path}, nil
}

func (fsys *fsFS) Stat(path string) (fs.FileInfo, error) {
	return fsys.StatFile(path, 0)
}

func (fsys *fsFS) StatFile(path string, flags int) (fs.FileInfo, error) {
	if flags != 0 {
		return nil, fs.ErrInvalid
	}
	if fsys.base == nil {
		return nil, fs.ErrNotExist
	}
	return fs.Stat(fsys.base, path)
}

func (fsys *fsFS) MakeDir(path string, perm fs.FileMode) error {
	return ErrReadOnly
}

func (fsys *fsFS) Chtimes(path string, flags int, atim, mtim time.Time) error {
	return ErrReadOnly
}

type fsFile struct {
	fsys FS
	base fs.File
	path string
}

func (f *fsFile) Name() string { return fspath.Base(f.path) }

func (f *fsFile) Close() error { return f.base.Close() }

func (f *fsFile) OpenFile(path string, flags int, perm fs.FileMode) (File, error) {
	return f.fsys.OpenFile(f.pathTo(path), flags, perm)
}

func (f *fsFile) Read(b []byte) (int, error) { return f.base.Read(b) }

func (f *fsFile) ReadAt(b []byte, off int64) (int, error) {
	if r, ok := f.base.(io.ReaderAt); ok {
		return r.ReadAt(b, off)
	}
	return 0, ErrNotImplemented
}

func (f *fsFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if r, ok := f.base.(fs.ReadDirFile); ok {
		return r.ReadDir(n)
	}
	return nil, ErrNotImplemented
}

func (f *fsFile) Stat() (fs.FileInfo, error) { return f.base.Stat() }

func (f *fsFile) StatFile(path string, flags int) (fs.FileInfo, error) {
	return f.fsys.StatFile(f.pathTo(path), flags)
}

func (f *fsFile) Chtimes(atim, mtim time.Time) error { return ErrReadOnly }

func (f *fsFile) ChtimesFile(path string, flags int, atim, mtim time.Time) error {
	return f.fsys.Chtimes(f.pathTo(path), flags, atim, mtim)
}

func (f *fsFile) Seek(offset int64, whence int) (int64, error) {
	if s, ok := f.base.(io.Seeker); ok {
		return s.Seek(offset, whence)
	}
	return 0, ErrNotImplemented
}

func (f *fsFile) Write([]byte) (int, error) { return 0, ErrReadOnly }

func (f *fsFile) WriteAt([]byte, int64) (int, error) { return 0, ErrReadOnly }

func (f *fsFile) MakeDir(path string, perm fs.FileMode) error { return ErrReadOnly }

func (f *fsFile) pathTo(path string) string { return fspath.Join(f.path, path) }

// DirFS returns a file system backed by the given root directory.
//
// This function is similar in behavior to os.DirFS, and therefore does not
// provide a strong isolation model; the application will be able to access
// files outside of the root by following symlinks or opening the parent
// directory.
func DirFS(root string) (FS, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &dirFS{root: root}, nil
}

type dirFS struct{ root string }

func (fsys *dirFS) Open(path string) (fs.File, error) {
	return fsys.OpenFile(path, 0, 0)
}

func (fsys *dirFS) Stat(path string) (fs.FileInfo, error) {
	return fsys.StatFile(path, 0)
}

func (fsys *dirFS) OpenFile(path string, flags int, perm fs.FileMode) (File, error) {
	if !fs.ValidPath(path) {
		return nil, fs.ErrInvalid
	}
	return fsys.openFile(fsys.pathTo(path), flags, perm)
}

func (fsys *dirFS) StatFile(path string, flags int) (fs.FileInfo, error) {
	if !fs.ValidPath(path) {
		return nil, fs.ErrInvalid
	}
	return fsys.statFile(fsys.pathTo(path), flags)
}

func (fsys *dirFS) MakeDir(path string, perm fs.FileMode) error {
	if !fs.ValidPath(path) {
		return fs.ErrInvalid
	}
	return fsys.createDir(fsys.pathTo(path), perm)
}

func (fsys *dirFS) Chtimes(path string, flags int, atim, mtim time.Time) error {
	if !fs.ValidPath(path) {
		return fs.ErrInvalid
	}
	return fsys.chtimes(fsys.pathTo(path), flags, atim, mtim)
}

func (fsys *dirFS) pathTo(path string) string {
	return filepath.Join(fsys.root, filepath.FromSlash(path))
}

func (fsys *dirFS) openFile(path string, flags int, perm fs.FileMode) (File, error) {
	f, err := os.OpenFile(path, flags, perm)
	if err != nil {
		return nil, err
	}
	return &dirFile{fsys: fsys, File: f}, nil
}

func (fsys *dirFS) statFile(path string, flags int) (fs.FileInfo, error) {
	if (flags & O_NOFOLLOW) != 0 {
		return os.Lstat(path)
	} else {
		return os.Stat(path)
	}
}

func (fsys *dirFS) createDir(path string, perm fs.FileMode) error {
	return os.Mkdir(path, perm)
}

func (fsys *dirFS) chtimes(path string, flags int, atim, mtim time.Time) error {
	if flags != 0 {
		return fs.ErrInvalid
	}
	return os.Chtimes(path, atim, mtim)
}

type dirFile struct {
	fsys *dirFS
	*os.File
}

func (f *dirFile) Name() string {
	return filepath.Base(f.File.Name())
}

func (f *dirFile) OpenFile(path string, flags int, perm fs.FileMode) (File, error) {
	if !fs.ValidPath(path) {
		return nil, fs.ErrInvalid
	}
	return f.fsys.openFile(f.pathTo(path), flags, perm)
}

func (f *dirFile) StatFile(path string, flags int) (fs.FileInfo, error) {
	if !fs.ValidPath(path) {
		return nil, fs.ErrInvalid
	}
	return f.fsys.statFile(f.pathTo(path), flags)
}

func (f *dirFile) MakeDir(path string, perm fs.FileMode) error {
	if !fs.ValidPath(path) {
		return fs.ErrInvalid
	}
	return f.fsys.createDir(f.pathTo(path), perm)
}

func (f *dirFile) Chtimes(atim, mtim time.Time) error {
	return f.fsys.chtimes(f.File.Name(), O_NOFOLLOW, atim, mtim)
}

func (f *dirFile) ChtimesFile(path string, flags int, atim, mtim time.Time) error {
	if !fs.ValidPath(path) {
		return fs.ErrInvalid
	}
	return f.fsys.chtimes(f.pathTo(path), flags, atim, mtim)
}

func (f *dirFile) pathTo(path string) string {
	return filepath.Join(f.File.Name(), filepath.FromSlash(path))
}

// SubFS returns a file system based on fsys rooted at the given directory path.
//
// This function is similar in behavior to fs.Sub, and therefore does not
// provide a strong isolation model; the application will be able to access
// files outside of the root by following symlinks or opening the parent
// directory.
func Sub(fsys FS, dir string) (FS, error) {
	f, err := fsys.OpenFile(dir, 0, 0755)
	if err != nil {
		return nil, err
	}
	return &subFS{root: f}, nil
}

type subFS struct{ root File }

func (fsys *subFS) Open(path string) (fs.File, error) { return fsys.OpenFile(path, 0, 0) }

func (fsys *subFS) Stat(path string) (fs.FileInfo, error) { return fsys.StatFile(path, 0) }

func (fsys *subFS) OpenFile(path string, flags int, perm fs.FileMode) (File, error) {
	if !fs.ValidPath(path) {
		return nil, fs.ErrInvalid
	}
	return fsys.root.OpenFile(path, flags, perm)
}

func (fsys *subFS) StatFile(path string, flags int) (fs.FileInfo, error) {
	if !fs.ValidPath(path) {
		return nil, fs.ErrInvalid
	}
	return fsys.root.StatFile(path, flags)
}

func (fsys *subFS) MakeDir(path string, perm fs.FileMode) error {
	if !fs.ValidPath(path) {
		return fs.ErrInvalid
	}
	return fsys.root.MakeDir(path, perm)
}

func (fsys *subFS) Chtimes(path string, flags int, atim, mtim time.Time) error {
	if !fs.ValidPath(path) {
		return fs.ErrInvalid
	}
	return fsys.root.ChtimesFile(path, flags, atim, mtim)
}
