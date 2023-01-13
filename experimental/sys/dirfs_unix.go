package sys

import (
	"io/fs"
	"path/filepath"
	"syscall"
)

func (d dirFileFS) fd() int {
	return int(d.base.Fd())
}

func (d dirFileFS) mkdir(name string, perm fs.FileMode) error {
	return mkdirat(d.fd(), name, uint32(perm))
}

func (d dirFileFS) rmdir(name string) error {
	return unlinkat(d.fd(), name, __AT_REMOVEDIR)
}

func (d dirFileFS) unlink(name string) error {
	return unlinkat(d.fd(), name, 0)
}

func (d dirFileFS) link(oldName, newName string, d2 dirFileFS) error {
	return linkat(d.fd(), oldName, d2.fd(), newName, __AT_SYMLINK_FOLLOW)
}

func (d dirFileFS) symlink(oldName, newName string) error {
	return symlinkat(oldName, d.fd(), newName)
}

func (d dirFileFS) readlink(name string) (string, error) {
	return readlinkat(d.fd(), name)
}

func (d dirFileFS) rename(oldName, newName string, d2 dirFileFS) error {
	return renameat(d.fd(), oldName, d2.fd(), newName)
}

func (d dirFileFS) truncate(name string, size int64) error {
	f, err := openat(d.fd(), name, syscall.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(f)
	return syscall.Ftruncate(f, size)
}

func (d dirFileFS) stat(name string) (fs.FileInfo, error) {
	info := &fileInfo{
		name: filepath.Base(name),
	}
	if err := fstatat(d.fd(), name, &info.stat, 0); err != nil {
		return nil, err
	}
	return info, nil
}
