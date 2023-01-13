package sys

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

func (d dirFileFS) openFile(name string, flags int, perm fs.FileMode) (File, error) {
	fsPath := filepath.Join(d.name, name)
	osPath := filepath.Join(d.fsys.root, fsPath)
	f, err := openat(d.fd(), name, flags, uint32(perm))
	if err != nil {
		// see openFile in fs_linux.go
		if err == syscall.ELOOP {
			if (flags & (O_DIRECTORY | O_NOFOLLOW | O_PATH)) == O_NOFOLLOW {
				f, err = openat(d.fd(), name, flags|O_PATH, uint32(perm))
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return d.fsys.newFile(os.NewFile(uintptr(f), osPath), fsPath), nil
}
