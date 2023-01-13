package sys

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
	"time"
	"unsafe"
)

const (
	// Darwin does not have O_DSYNC/O_RSYNC, so fallback to O_SYNC.
	O_DSYNC = syscall.O_SYNC
	O_RSYNC = syscall.O_SYNC

	__AT_SYMLINK_FOLLOW = 0x0040
	__AT_REMOVEDIR      = 0x0080

	__SYS_OPENAT     = 463
	__SYS_RENAMEAT   = 465
	__SYS_FSTATAT64  = 470
	__SYS_LINKAT     = 471
	__SYS_UNLINKAT   = 472
	__SYS_READLINKAT = 473
	__SYS_SYMLINKAT  = 474
	__SYS_MKDIRAT    = 475
	__SYS_FREADLINK  = 551

	openFileReadOnlyFlags = O_RDONLY | O_DIRECTORY | O_NOFOLLOW
)

func openFile(path string, flags int, perm fs.FileMode) (*os.File, error) {
	f, err := os.OpenFile(path, flags, perm)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) && ((flags & O_NOFOLLOW) != 0) {
			flags &= ^O_NOFOLLOW
			flags |= syscall.O_SYMLINK
			f, err = os.OpenFile(path, flags, perm)
		}
	}
	return f, err
}

func readlink(file *os.File) (string, error) {
	return freadlink(int(file.Fd()))
}

func datasync(file *os.File) error {
	fd := file.Fd()
	for {
		_, _, err := syscall.Syscall(syscall.SYS_FDATASYNC, fd, 0, 0)
		switch err {
		case syscall.EINTR:
		case 0:
			return nil
		default:
			return err
		}
	}
}

func openat(fd int, path string, flags int, perm uint32) (int, error) {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return -1, err
	}
	r, _, e := syscall.Syscall6(
		uintptr(__SYS_OPENAT),
		uintptr(fd),
		uintptr(unsafe.Pointer(p)),
		uintptr(flags),
		uintptr(perm),
		uintptr(0),
		uintptr(0),
	)
	if e != 0 {
		return -1, e
	}
	return int(r), nil
}

func renameat(oldfd int, oldpath string, newfd int, newpath string) error {
	p0, err := syscall.BytePtrFromString(oldpath)
	if err != nil {
		return err
	}
	p1, err := syscall.BytePtrFromString(newpath)
	if err != nil {
		return err
	}
	_, _, e := syscall.Syscall6(
		uintptr(__SYS_RENAMEAT),
		uintptr(oldfd),
		uintptr(unsafe.Pointer(p0)),
		uintptr(newfd),
		uintptr(unsafe.Pointer(p1)),
		uintptr(0),
		uintptr(0),
	)
	if e != 0 {
		if e == syscall.EISDIR {
			e = syscall.EEXIST
		}
		return e
	}
	return nil
}

func fstatat(fd int, path string, stat *syscall.Stat_t, flags int) error {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	_, _, e := syscall.Syscall6(
		uintptr(__SYS_FSTATAT64),
		uintptr(fd),
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(stat)),
		uintptr(flags),
		uintptr(0),
		uintptr(0),
	)
	if e != 0 {
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
		uintptr(__SYS_LINKAT),
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

func unlinkat(fd int, path string, flags int) error {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	_, _, e := syscall.Syscall(
		uintptr(__SYS_UNLINKAT),
		uintptr(fd),
		uintptr(unsafe.Pointer(p)),
		uintptr(flags),
	)
	if e != 0 {
		return e
	}
	return nil
}

func readlinkat(fd int, path string) (string, error) {
	buf := [1025]byte{}
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return "", err
	}
	n, _, e := syscall.Syscall6(
		uintptr(__SYS_READLINKAT),
		uintptr(fd),
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(0),
		uintptr(0),
	)
	if e != 0 {
		return "", e
	}
	if int(n) == len(buf) {
		return "", syscall.ENAMETOOLONG
	}
	return string(buf[:n]), nil
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
		uintptr(__SYS_SYMLINKAT),
		uintptr(unsafe.Pointer(p0)),
		uintptr(fd),
		uintptr(unsafe.Pointer(p1)),
	)
	if e != 0 {
		return e
	}
	return nil
}

func mkdirat(fd int, path string, perm uint32) error {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	_, _, e := syscall.Syscall(
		uintptr(__SYS_MKDIRAT),
		uintptr(fd),
		uintptr(unsafe.Pointer(p)),
		uintptr(perm),
	)
	if e != 0 {
		return e
	}
	return nil
}

func freadlink(fd int) (string, error) {
	buf := [1025]byte{}
	n, _, e := syscall.Syscall(
		uintptr(__SYS_FREADLINK),
		uintptr(fd),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if e != 0 {
		return "", e
	}
	if int(n) == len(buf) {
		return "", syscall.ENAMETOOLONG
	}
	return string(buf[:n]), nil
}

func (info *fileInfo) ModTime() time.Time {
	return time.Unix(info.stat.Mtimespec.Unix())
}
