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

func handleEBADF(err error) error {
	if err != nil && errors.Is(err, syscall.EBADF) {
		err = ErrPermission
	}
	return err
}

func (f dirFile) Read(b []byte) (int, error) {
	n, err := f.File.Read(b)
	return n, handleEBADF(err)
}

func (f dirFile) ReadAt(b []byte, off int64) (int, error) {
	n, err := f.File.ReadAt(b, off)
	return n, handleEBADF(err)
}

func (f dirFile) ReadFrom(r io.Reader) (int64, error) {
	n, err := f.readFrom(r)
	return n, handleEBADF(err)
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
	return n, handleEBADF(err)
}

func (f dirFile) WriteAt(b []byte, off int64) (int, error) {
	n, err := f.File.WriteAt(b, off)
	return n, handleEBADF(err)
}

func (f dirFile) WriteString(s string) (int, error) {
	n, err := f.File.WriteString(s)
	return n, handleEBADF(err)
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
	return linkat(f.fd(), oldName, dirfd(newDir), newName, __AT_SYMLINK_FOLLOW)
}

func (f dirFile) Rename(oldName string, newDir Directory, newName string) error {
	return renameat(f.fd(), oldName, dirfd(newDir), newName)
}

func (f dirFile) fd() int {
	return int(f.File.Fd())
}

func dirfd(d Directory) int {
	if f, _ := d.Sys().(*os.File); f != nil {
		return int(f.Fd())
	}
	return -1
}
