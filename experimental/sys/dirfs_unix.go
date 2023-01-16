package sys

import (
	"io/fs"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys/sysinfo"
)

func (f dirFile) fd() int {
	return int(f.File.Fd())
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
	return linkat(f.fd(), oldName, int(newDir.Fd()), newName, __AT_SYMLINK_FOLLOW)
}

func (f dirFile) Rename(oldName string, newDir Directory, newName string) error {
	return renameat(f.fd(), oldName, int(newDir.Fd()), newName)
}

func (f dirFile) GetXAttr(name string) (string, bool, error) {
	return fgetxattr(f.fd(), name)
}

func (f dirFile) SetXAttr(name, value string, flags int) error {
	return fsetxattr(f.fd(), name, value, flags)
}

func (f dirFile) ListXAttr() ([]string, error) {
	return flistxattr(f.fd())
}
