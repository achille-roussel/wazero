package sys

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

func (f dirFile) openFile(name string, flags int, perm fs.FileMode) (*os.File, error) {
	fd, err := openat(f.fd(), name, flags, uint32(perm))
	if err != nil {
		if err == syscall.ELOOP && ((flags & O_NOFOLLOW) != 0) {
			flags &= ^O_NOFOLLOW
			flags |= syscall.O_SYMLINK
			fd, err = openat(f.fd(), name, flags, uint32(perm))
		}
	}
	if err != nil {
		return nil, err
	}
	path := filepath.Join(f.Name(), name)
	file := os.NewFile(uintptr(fd), path)
	return file, nil
}
