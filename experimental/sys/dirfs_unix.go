package sys

import (
	"io/fs"
)

func (f *dirFile) fd() int {
	return int(f.base.Fd())
}

func (f *dirFile) mkdir(name string, perm fs.FileMode) error {
	return mkdirat(f.fd(), name, uint32(perm))
}

func (f *dirFile) rmdir(name string) error {
	return unlinkat(f.fd(), name, __AT_REMOVEDIR)
}

func (f *dirFile) unlink(name string) error {
	return unlinkat(f.fd(), name, 0)
}

func (f *dirFile) symlink(oldName, newName string) error {
	return symlinkat(oldName, f.fd(), newName)
}

func (f *dirFile) link(oldName string, newDir uintptr, newName string) error {
	return linkat(f.fd(), oldName, int(newDir), newName, __AT_SYMLINK_FOLLOW)
}

func (f *dirFile) rename(oldName string, newDir uintptr, newName string) error {
	return renameat(f.fd(), oldName, int(newDir), newName)
}
