package sys

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"syscall"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys/sysinfo"
)

func (f dirFile) Read(b []byte) (int, error) {
	n, err := f.File.Read(b)
	return n, f.handleEBADF(err)
}

func (f dirFile) ReadAt(b []byte, off int64) (int, error) {
	if len(b) == 0 && f.fd() < 0 {
		return 0, ErrClosed
	}
	n, err := f.File.ReadAt(b, off)
	return n, f.handleEBADF(err)
}

func (f dirFile) ReadFrom(r io.Reader) (int64, error) {
	n, err := f.readFrom(r)
	// On Linux the copy_file_range optimization may return a *os.SyscallError
	// wrapping an unexported error value when called on a closed file.
	normalizePathError(err, "use of closed file", ErrClosed)
	return n, f.handleEBADF(err)
}

func (f dirFile) readFrom(r io.Reader) (int64, error) {
	// Do our best to try to retrieve the underlying *os.File if one exists
	// because the copy between files is optimized by os.(*File).ReadFrom to
	// use copy_file_range on linux.
	if f2, ok := r.(interface{ Sys() any }); ok {
		if rr, ok := f2.Sys().(io.Reader); ok {
			return f.File.ReadFrom(rr)
		}
	}
	return io.Copy(f.File, r)
}

func (f dirFile) Write(b []byte) (int, error) {
	n, err := f.File.Write(b)
	return n, f.handleEBADF(err)
}

func (f dirFile) WriteAt(b []byte, off int64) (int, error) {
	if len(b) == 0 && f.fd() < 0 {
		return 0, ErrClosed
	}
	n, err := f.File.WriteAt(b, off)
	return n, f.handleEBADF(err)
}

func (f dirFile) WriteString(s string) (int, error) {
	n, err := f.File.WriteString(s)
	return n, f.handleEBADF(err)
}

func (f dirFile) Readlink() (string, error) {
	return readlink(f.File)
}

func (f dirFile) Chtimes(atime, mtime time.Time) error {
	return chtimes(f.File, atime, mtime)
}

func (f dirFile) Datasync() error {
	return datasync(f.File)
}

func (f dirFile) Access(name string, mode fs.FileMode) error {
	return faccessat(f.fd(), name, sysinfo.FileMode(mode&7), __AT_SYMLINK_NOFOLLOW)
}

func (f dirFile) Mknod(name string, mode fs.FileMode, dev Device) error {
	return mknodat(f.fd(), name, sysinfo.FileMode(mode), int(dev))
}

func (f dirFile) Mkdir(name string, perm fs.FileMode) error {
	return mkdirat(f.fd(), name, sysinfo.FileMode(perm))
}

func (f dirFile) Rmdir(name string) error {
	return unlinkat(f.fd(), name, __AT_REMOVEDIR)
}

func (f dirFile) Unlink(name string) error {
	return unlinkat(f.fd(), name, 0)
}

func (f dirFile) Symlink(oldName, newName string) error {
	return symlinkat(oldName, f.fd(), newName)
}

func (f dirFile) Link(oldName string, newDir Directory, newName string) error {
	return linkat(f.fd(), oldName, dirfd(newDir), newName, 0)
}

func (f dirFile) Rename(oldName string, newDir Directory, newName string) error {
	return renameat(f.fd(), oldName, dirfd(newDir), newName)
}

func (f dirFile) Lchmod(name string, mode fs.FileMode) error {
	return fchmodat(f.fd(), name, sysinfo.FileMode(mode), __AT_SYMLINK_NOFOLLOW)
}

func (f dirFile) Lstat(name string) (fs.FileInfo, error) {
	var stat syscall.Stat_t
	if err := fstatat(f.fd(), name, &stat, __AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, err
	}
	return sysinfo.NewFileInfo(name, &stat), nil
}

func (f dirFile) openFile(name string, flags int, perm fs.FileMode) (*os.File, error) {
	return openFileAt(f.fd(), f.Name(), name, flags, perm)
}

func (f dirFile) fd() int {
	return int(f.File.Fd())
}

func (f dirFile) handleEBADF(err error) error {
	if err != nil && errors.Is(err, syscall.EBADF) {
		if int(f.Fd()) < 0 {
			err = ErrClosed
		} else {
			err = ErrPermission
		}
	}
	return err
}

func dirfd(d Directory) int {
	if f, _ := d.Sys().(*os.File); f != nil {
		return int(f.Fd())
	}
	return -1
}
