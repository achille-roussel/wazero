package sys

import (
	"fmt"
	"io"
	"io/fs"
	"time"
)

// ReadOnlyFS constructs a FS from a FS. The returned file system supports
// only read operations, and will return ErrReadOnly on any method call which
// attempts to mutate the state of the file system.
func ReadOnlyFS(fsys FS) FS {
	return FuncFS(func(_ FS, name string, flags int, perm fs.FileMode) (File, error) {
		return openReadOnlyFile(fsys.OpenFile, name, flags, perm)
	})
}

type readOnlyFS struct{ *file[readOnlyFile] }

func (r readOnlyFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	return openReadOnlyFile(r.file.OpenFile, name, flags, perm)
}

func openReadOnlyFile(open openFileFunc, name string, flags int, perm fs.FileMode) (File, error) {
	if !hasReadOnlyFlags(flags) {
		return nil, ErrReadOnly
	}
	f, err := open(name, flags, perm)
	if err != nil {
		return nil, err
	}
	switch r := f.(type) {
	case readOnlyFS:
		return r, nil
	case *file[readOnlyFile]:
		return readOnlyFS{r}, nil
	default:
		return readOnlyFS{newFile(readOnlyFile{file: r})}, nil
	}
}

// ReadOnlyFile constructs a read-only file.
//
// The returned file does not allow any mutations but if it is a directory,
// files opened from it may not be read-only (e.g. if they are opened in write
// mode). To have a fully recursive read-only view of a file system, use
// ReadOnlyFS instead.
func ReadOnlyFile(f File) File {
	switch r := f.(type) {
	case *file[readOnlyFile]:
		return r
	default:
		return newFile(readOnlyFile{file: r})
	}
}

// readOnlyFile is an adapter which ensures that only read operations are
// allowed on a File instance. This is always wrapped by a file[T] to ensure
// proper behavior and wrapping of the errors.
type readOnlyFile struct {
	ReadOnly
	file File
}

func (f readOnlyFile) GoString() string {
	return fmt.Sprintf("%#v", f.file)
}

func (f readOnlyFile) Name() string {
	return f.file.Name()
}

func (f readOnlyFile) Sys() any {
	return f.file.Sys()
}

func (f readOnlyFile) Close() error {
	return f.file.Close()
}

func (f readOnlyFile) Read(b []byte) (int, error) {
	return f.file.Read(b)
}

func (f readOnlyFile) ReadAt(b []byte, off int64) (int, error) {
	return f.file.ReadAt(b, off)
}

func (f readOnlyFile) ReadDir(n int) ([]fs.DirEntry, error) {
	return f.file.ReadDir(n)
}

func (f readOnlyFile) Readlink() (string, error) {
	return f.file.Readlink()
}

func (f readOnlyFile) Seek(offset int64, whence int) (int64, error) {
	return f.file.Seek(offset, whence)
}

func (f readOnlyFile) Stat() (fs.FileInfo, error) {
	return f.file.Stat()
}

func (f readOnlyFile) Access(name string, mode fs.FileMode) error {
	if (mode & 0b010) == 0 {
		return f.file.Access(name, mode)
	}
	return ErrPermission
}

func (f readOnlyFile) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	if !hasReadOnlyFlags(flags) {
		return nil, ErrReadOnly
	}
	file, err := f.file.OpenFile(name, flags, perm)
	if err != nil {
		return nil, err
	}
	return ReadOnlyFile(file), nil
}

// ReadOnly is a helper type declaring methods of the File interface for
// implementations that only allow read operations.
type ReadOnly struct{}

func (ReadOnly) ReadFrom(io.Reader) (int64, error) {
	return 0, ErrPermission
}

func (ReadOnly) Write([]byte) (int, error) {
	return 0, ErrPermission
}

func (ReadOnly) WriteAt([]byte, int64) (int, error) {
	return 0, ErrPermission
}

func (ReadOnly) WriteString(string) (int, error) {
	return 0, ErrPermission
}

func (ReadOnly) Chmod(fs.FileMode) error {
	return ErrReadOnly
}

func (ReadOnly) Chtimes(time.Time, time.Time) error {
	return ErrReadOnly
}

func (ReadOnly) Truncate(size int64) (err error) {
	return ErrReadOnly
}

func (ReadOnly) Sync() error {
	return ErrReadOnly
}

func (ReadOnly) Datasync() error {
	return ErrReadOnly
}

func (ReadOnly) Mknod(string, fs.FileMode, Device) error {
	return ErrReadOnly
}

func (ReadOnly) Mkdir(string, fs.FileMode) error {
	return ErrReadOnly
}

func (ReadOnly) Rmdir(string) error {
	return ErrReadOnly
}

func (ReadOnly) Unlink(string) error {
	return ErrReadOnly
}

func (ReadOnly) Symlink(string, string) error {
	return ErrReadOnly
}

func (ReadOnly) Link(string, Directory, string) error {
	return ErrReadOnly
}

func (ReadOnly) Rename(string, Directory, string) error {
	return ErrReadOnly
}
