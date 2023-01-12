package sys

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func (d dirFileFS) openFile(name string, flags int, perm fs.FileMode) (File, error) {
	fsPath, ok := join(d.name, name)
	if !ok {
		return nil, ErrInvalid
	}
	osPath := filepath.Join(d.fsys.root, filepath.FromSlash(fsPath))
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

func (d dirFileFS) chtimes(name string, atime, mtime time.Time) error {
	return syscall.Futimesat(d.fd(), name, []syscall.Timeval{
		syscall.NsecToTimeval(atime.UnixNano()),
		syscall.NsecToTimeval(mtime.UnixNano()),
	})
}
