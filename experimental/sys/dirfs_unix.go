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
	if len(b) == 0 && int(f.Fd()) < 0 {
		return 0, f.wrap("read", ErrClosed)
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
	if len(b) == 0 && int(f.Fd()) < 0 {
		return 0, f.wrap("write", ErrClosed)
	}
	n, err := f.File.WriteAt(b, off)
	return n, f.handleEBADF(err)
}

func (f dirFile) WriteString(s string) (int, error) {
	n, err := f.File.WriteString(s)
	return n, f.handleEBADF(err)
}

func (f dirFile) Readlink() (string, error) {
	link, err := readlink(f.File)
	return link, f.wrap("readlink", err)
}

func (f dirFile) Chtimes(atime, mtime time.Time) error {
	return f.wrap("chtimes", chtimes(f.File, atime, mtime))
}

func (f dirFile) Datasync() error {
	return f.wrap("datasync", datasync(f.File))
}

func (f dirFile) Access(name string, mode fs.FileMode) error {
	return f.wrap("access", faccessat(f.fd(), name, sysinfo.FileMode(mode)&7, 0))
}

func (f dirFile) Mknod(name string, mode fs.FileMode, dev Device) error {
	return f.wrap("mknod", mknodat(f.fd(), name, sysinfo.FileMode(mode), int(dev)))
}

func (f dirFile) Mkdir(name string, perm fs.FileMode) error {
	return f.wrap("mkdir", mkdirat(f.fd(), name, sysinfo.FileMode(perm)))
}

func (f dirFile) Rmdir(name string) error {
	return f.wrap("rmdir", unlinkat(f.fd(), name, __AT_REMOVEDIR))
}

func (f dirFile) Unlink(name string) error {
	return f.wrap("unlink", unlinkat(f.fd(), name, 0))
}

func (f dirFile) Symlink(oldName, newName string) error {
	return f.wrap("symlink", symlinkat(oldName, f.fd(), newName))
}

func (f dirFile) Link(oldName string, newDir Directory, newName string) error {
	return f.wrap("link", linkat(f.fd(), oldName, dirfd(newDir), newName, __AT_SYMLINK_FOLLOW))
}

func (f dirFile) Rename(oldName string, newDir Directory, newName string) error {
	return f.wrap("rename", renameat(f.fd(), oldName, dirfd(newDir), newName))
}

func (f dirFile) openFile(name string, flags int, perm fs.FileMode) (*os.File, error) {
	file, err := openFileAt(int(f.fd()), f.Name(), name, flags, perm)
	return file, f.wrap("open", err)
}

func (f dirFile) fd() int {
	return int(f.File.Fd())
}

func (f dirFile) wrap(op string, err error) error {
	if err != nil {
		err = makePathError(op, f.Name(), f.handleEBADF(err))
	}
	return err
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
