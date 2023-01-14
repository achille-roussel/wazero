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
		// see openFile in fs_linux.go
		if err == syscall.ELOOP {
			if (flags & (O_DIRECTORY | O_NOFOLLOW | O_PATH)) == O_NOFOLLOW {
				fd, err = openat(f.fd(), name, flags|O_PATH, uint32(perm))
			}
		}
	}
	if err != nil {
		return nil, err
	}
	path := filepath.Join(f.Name(), name)
	file := os.NewFile(uintptr(fd), path)
	return file, nil
}
