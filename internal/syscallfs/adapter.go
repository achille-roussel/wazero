package syscallfs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	pathutil "path"
	"syscall"
)

// Adapt returns a read-only FS unless the input is already one.
func Adapt(fs fs.FS) FS {
	if sys, ok := fs.(FS); ok {
		return sys
	}
	return &adapter{fs}
}

type adapter struct {
	fs fs.FS
}

// Open implements the same method as documented on fs.FS
func (ro *adapter) Open(name string) (fs.File, error) {
	panic(fmt.Errorf("unexpected to call fs.FS.Open(%s)", name))
}

// Path implements FS.Path
func (ro *adapter) Path() string {
	return "/"
}

// OpenFile implements FS.OpenFile
func (ro *adapter) OpenFile(path string, flag int, perm fs.FileMode) (File, error) {
	if flag != 0 && flag != os.O_RDONLY {
		return nil, syscall.ENOSYS
	}

	path = cleanPath(path)
	f, err := ro.fs.Open(path)
	if err != nil {
		// wrapped is fine while FS.OpenFile emulates os.OpenFile vs syscall.OpenFile.
		return nil, err
	}
	return &adapterFile{f}, nil
}

func cleanPath(name string) string {
	if len(name) == 0 {
		return name
	}
	// fs.ValidFile cannot be rooted (start with '/')
	cleaned := name
	if name[0] == '/' {
		cleaned = name[1:]
	}
	cleaned = pathutil.Clean(cleaned) // e.g. "sub/." -> "sub"
	return cleaned
}

// Mkdir implements FS.Mkdir
func (ro *adapter) Mkdir(path string, perm fs.FileMode) error {
	return syscall.ENOSYS
}

// Rename implements FS.Rename
func (ro *adapter) Rename(from, to string) error {
	return syscall.ENOSYS
}

// Rmdir implements FS.Rmdir
func (ro *adapter) Rmdir(path string) error {
	return syscall.ENOSYS
}

// Unlink implements FS.Unlink
func (ro *adapter) Unlink(path string) error {
	return syscall.ENOSYS
}

// Utimes implements FS.Utimes
func (ro *adapter) Utimes(path string, atimeNsec, mtimeNsec int64) error {
	return syscall.ENOSYS
}

type adapterFile struct{ fs.File }

func (f *adapterFile) ReadAt(b []byte, off int64) (int, error) {
	if r, ok := f.File.(io.ReaderAt); ok {
		return r.ReadAt(b, off)
	}
	return 0, syscall.ENOSYS
}

func (f *adapterFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if d, ok := f.File.(fs.ReadDirFile); ok {
		return d.ReadDir(n)
	}
	// TOOD: return ENOTDIR if called on a file which is not a directory
	return nil, syscall.ENOSYS
}

func (f *adapterFile) Seek(offset int64, whence int) (int64, error) {
	if s, ok := f.File.(io.Seeker); ok {
		return s.Seek(offset, whence)
	}
	return 0, syscall.ENOSYS
}

func (f *adapterFile) Write([]byte) (int, error) {
	return 0, syscall.ENOSYS
}

func (f *adapterFile) WriteAt([]byte, int64) (int, error) {
	return 0, syscall.ENOSYS
}
