package sys

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func (d dirFileFS) fd() int { return int(d.base.Fd()) }

func (d dirFileFS) openFile(name string, flags int, perm fs.FileMode) (File, error) {
	fsPath, ok := join(d.name, name)
	if !ok {
		return nil, ErrInvalid
	}
	osPath := filepath.Join(d.fsys.root, filepath.FromSlash(fsPath))
	f, err := syscall.Openat(d.fd(), name, flags, uint32(perm))
	if err != nil {
		// see openFile in fs_linux.go
		if err == syscall.ELOOP {
			if (flags & (O_DIRECTORY | O_NOFOLLOW | O_PATH)) == O_NOFOLLOW {
				f, err = syscall.Openat(d.fd(), name, flags|O_PATH, uint32(perm))
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return d.fsys.newFile(os.NewFile(uintptr(f), osPath), fsPath), nil
}

func (d dirFileFS) mkdir(name string, perm fs.FileMode) error {
	return syscall.Mkdirat(d.fd(), name, uint32(perm))
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

func (d dirFileFS) chmod(name string, mode fs.FileMode) error {
	return syscall.Fchmodat(d.fd(), name, uint32(mode), 0)
}

func (d dirFileFS) chtimes(name string, atime, mtime time.Time) error {
	return syscall.Futimesat(d.fd(), name, []syscall.Timeval{
		syscall.NsecToTimeval(atime.UnixNano()),
		syscall.NsecToTimeval(mtime.UnixNano()),
	})
}

func (d dirFileFS) truncate(name string, size int64) error {
	f, err := syscall.Openat(d.fd(), name, syscall.O_WRONLY, 0)
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
	if err := fstatat(d.fd(), name, &info.stat); err != nil {
		return nil, err
	}
	return info, nil
}
