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
		// see openFile in fs_linux.go
		if err == syscall.ELOOP {
			if (flags & (O_DIRECTORY | O_NOFOLLOW | O_PATH)) == O_NOFOLLOW {
				fd, err = openat(f.fd(), name, flags|O_PATH, uint32(perm))
			}
		}
	}
	if err != nil {
		return nil, "", err
	}
	return os.NewFile(uintptr(fd), osPath), fsPath, nil
}
