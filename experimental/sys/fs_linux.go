package sys

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

const (
	O_DSYNC = syscall.O_DSYNC
	O_RSYNC = syscall.O_RSYNC

	__AT_REMOVEDIR      = 0x200
	__AT_SYMLINK_FOLLOW = 0x400
)

func datasync(file *os.File) error {
	fd := int(file.Fd())
	return ignoringEINTR(func() error { return syscall.Fdatasync(fd) })
}

func unlink(path string) (err error) {
	if err = syscall.Unlink(path); err != nil {
		// Linux is not complient with POSIX and gives EISDIR instead of EPERM
		// when attenting to unlink a directory.
		if err == syscall.EISDIR {
			err = syscall.EPERM
		}
	}
	return err
}

func unlinkat(fd int, path string, flags int) error {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	_, _, e := syscall.Syscall(
		uintptr(syscall.SYS_UNLINKAT),
		uintptr(fd),
		uintptr(unsafe.Pointer(p)),
		uintptr(flags),
	)
	if e != 0 {
		if e == syscall.EISDIR {
			e = syscall.EPERM
		}
		return e
	}
	return nil
}

func linkat(oldfd int, oldpath string, newfd int, newpath string, flags int) error {
	p0, err := syscall.BytePtrFromString(oldpath)
	if err != nil {
		return err
	}
	p1, err := syscall.BytePtrFromString(newpath)
	if err != nil {
		return err
	}
	_, _, e := syscall.Syscall6(
		uintptr(syscall.SYS_LINKAT),
		uintptr(oldfd),
		uintptr(unsafe.Pointer(p0)),
		uintptr(newfd),
		uintptr(unsafe.Pointer(p1)),
		uintptr(flags),
		uintptr(0),
	)
	if e != 0 {
		return e
	}
	return nil
}

func symlinkat(target string, fd int, path string) error {
	p0, err := syscall.BytePtrFromString(target)
	if err != nil {
		return err
	}
	p1, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	_, _, e := syscall.Syscall(
		uintptr(syscall.SYS_SYMLINKAT),
		uintptr(unsafe.Pointer(p0)),
		uintptr(fd),
		uintptr(unsafe.Pointer(p1)),
	)
	if e != 0 {
		return e
	}
	return nil
}

func readlinkat(fd int, path string, buf []byte) (int, error) {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return 0, err
	}
	n, _, e := syscall.Syscall6(
		uintptr(syscall.SYS_READLINKAT),
		uintptr(fd),
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(0),
		uintptr(0),
	)
	if e != 0 {
		return int(n), e
	}
	return int(n), nil
}

func renameat(oldfd int, oldpath string, newfd int, newpath string) error {
	err := syscall.Renameat(oldfd, oldpath, newfd, newpath)
	if err != nil {
		// renameat behaves differently from rename and gives EISDIR instead of
		// EEXIST when the destination is an existing directory but the source
		// is not.
		if err == syscall.EISDIR {
			err = syscall.EEXIST
		}
	}
	return err
}

func fstatat(fd int, path string, stat *syscall.Stat_t) error {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	_, _, e := syscall.Syscall(
		uintptr(syscall.SYS_NEWFSTATAT),
		uintptr(fd),
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(stat)),
	)
	if e != 0 {
		return e
	}
	return nil
}

func (d dirFileFS) fd() int { return int(d.base.Fd()) }

func (d dirFileFS) openFile(name string, flags int, perm fs.FileMode) (File, error) {
	fsPath, err := join(d.name, name)
	if err != nil {
		return nil, err
	}
	osPath := filepath.Join(d.fsys.root, filepath.FromSlash(fsPath))
	f, err := syscall.Openat(d.fd(), name, flags, uint32(perm))
	if err != nil {
		return nil, err
	}
	file := &dirFile{
		fsys: d.fsys,
		base: os.NewFile(uintptr(f), osPath),
		name: fsPath,
		mode: makeDirFileMode(flags),
	}
	return file, nil
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

func (d dirFileFS) link(oldName, newName string) error {
	fd := d.fd()
	return linkat(fd, oldName, fd, newName, __AT_SYMLINK_FOLLOW)
}

func (d dirFileFS) symlink(oldName, newName string) error {
	return symlinkat(oldName, d.fd(), newName)
}

func (d dirFileFS) readlink(name string) (string, error) {
	buffer := [1025]byte{}
	n, err := readlinkat(d.fd(), name, buffer[:])
	if err != nil {
		return "", err
	}
	if n == len(buffer) {
		return "", syscall.ENAMETOOLONG
	}
	return string(buffer[:n]), nil
}

func (d dirFileFS) rename(oldName, newName string) error {
	fd := d.fd()
	return renameat(fd, oldName, fd, newName)
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

func (info *fileInfo) ModTime() time.Time {
	return time.Unix(info.stat.Mtim.Unix())
}
