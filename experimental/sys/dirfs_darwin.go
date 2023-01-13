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
		if err == syscall.ELOOP && ((flags & O_NOFOLLOW) != 0) {
			flags &= ^O_NOFOLLOW
			flags |= syscall.O_SYMLINK
			f, err = openat(d.fd(), name, flags, uint32(perm))
		}
	}
	if err != nil {
		return nil, err
	}
	return d.fsys.newFile(os.NewFile(uintptr(f), osPath), fsPath), nil
}
