package sys

import (
	"io/fs"
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

func (d dirFileFS) link(oldName, newName string, d2 dirFileFS) error {
	return linkat(d.fd(), oldName, d2.fd(), newName, __AT_SYMLINK_FOLLOW)
}

func (d dirFileFS) symlink(oldName, newName string) error {
	return symlinkat(oldName, d.fd(), newName)
}

func (d dirFileFS) rename(oldName, newName string, d2 dirFileFS) error {
	return renameat(d.fd(), oldName, d2.fd(), newName)
}

func (f *dirFile) fd() int {
	return int(f.base.Fd())
}

func (f *dirFile) unlink(name string) error {
	return unlinkat(f.fd(), name, 0)
}
