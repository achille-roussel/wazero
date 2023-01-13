package sys

import (
	"io/fs"
	"os"
	"path"
	"syscall"
)

func (f *dirFile) openFile(name string, flags int, perm fs.FileMode) (*os.File, string, error) {
	fsPath := path.Join(f.name, name)
	osPath := path.Join(f.fsys.root, fsPath)
	fd, err := openat(f.fd(), name, flags, uint32(perm))
	if err != nil {
		if err == syscall.ELOOP && ((flags & O_NOFOLLOW) != 0) {
			flags &= ^O_NOFOLLOW
			flags |= syscall.O_SYMLINK
			fd, err = openat(f.fd(), name, flags, uint32(perm))
		}
	}
	if err != nil {
		return nil, "", err
	}
	return os.NewFile(uintptr(fd), osPath), fsPath, nil
}
