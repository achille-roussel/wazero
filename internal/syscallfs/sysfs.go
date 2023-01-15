package syscallfs

import (
	"errors"
	"io/fs"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
)

func SysFS(base sys.FS) FS { return sysFS{base} }

type sysFS struct{ root sys.FS }

func (fsys sysFS) GuestDir() string { return "/" }

func (fsys sysFS) Open(path string) (fs.File, error) {
	f, err := fsys.root.Open(sysFSPath(path))
	return f, syscallError(err)
}

func (fsys sysFS) OpenFile(path string, flags int, perm fs.FileMode) (fs.File, error) {
	f, err := fsys.root.OpenFile(sysFSPath(path), flags, perm)
	return f, syscallError(err)
}

func (fsys sysFS) Mkdir(path string, perm fs.FileMode) error {
	return syscallError(sys.Mkdir(fsys.root, sysFSPath(path), perm))
}

func (fsys sysFS) Rename(from, to string) error {
	return syscallError(sys.Rename(fsys.root, sysFSPath(from), sysFSPath(to)))
}

func (fsys sysFS) Rmdir(path string) error {
	return syscallError(sys.Rmdir(fsys.root, sysFSPath(path)))
}

func (fsys sysFS) Unlink(path string) error {
	return syscallError(sys.Unlink(fsys.root, sysFSPath(path)))
}

func (fsys sysFS) Utimes(path string, atimeNsec, mtimeNsec int64) error {
	atime := time.Unix(0, atimeNsec)
	mtime := time.Unix(0, mtimeNsec)
	return syscallError(sys.Chtimes(fsys.root, sysFSPath(path), atime, mtime))
}

func sysFSPath(name string) string {
	return strings.TrimPrefix(path.Clean(name), "/")
}

func syscallError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, sys.ErrClosed):
		return syscall.EBADF
	case errors.Is(err, sys.ErrInvalid):
		return syscall.EINVAL
	case errors.Is(err, sys.ErrNotExist):
		return syscall.ENOENT
	case errors.Is(err, sys.ErrNotEmpty):
		return syscall.ENOTEMPTY
	case errors.Is(err, sys.ErrNotImplemented):
		return syscall.ENOSYS
	case errors.Is(err, sys.ErrNotSupported):
		return syscall.EBADF
	case errors.Is(err, sys.ErrPermission):
		return syscall.EPERM
	case errors.Is(err, sys.ErrReadOnly):
		return syscall.EROFS
	case errors.Is(err, sys.ErrLoop):
		return syscall.ELOOP
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno
	}
	return syscall.EIO
}
