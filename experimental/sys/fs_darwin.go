package sys

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
	"unsafe"
)

const (
	O_RDONLY    = syscall.O_RDONLY
	O_WRONLY    = syscall.O_WRONLY
	O_RDWR      = syscall.O_RDWR
	O_APPEND    = syscall.O_APPEND
	O_CREATE    = syscall.O_CREAT
	O_EXCL      = syscall.O_EXCL
	O_SYNC      = syscall.O_SYNC
	O_TRUNC     = syscall.O_TRUNC
	O_NOFOLLOW  = syscall.O_NOFOLLOW
	O_DIRECTORY = syscall.O_DIRECTORY
	O_NONBLOCK  = syscall.O_NONBLOCK
	O_DIRECT    = syscall.O_DIRECT
	O_SYMLINK   = syscall.O_SYMLINK

	// Darwin does not have O_DSYNC/O_RSYNC, so fallback to O_SYNC.
	O_DSYNC = syscall.O_SYNC
	O_RSYNC = syscall.O_SYNC

	__AT_SYMLINK_FOLLOW = 0x0040
	__AT_REMOVEDIR      = 0x0080

	__SYS_OPENAT     = 463
	__SYS_RENAMEAT   = 465
	__SYS_LINKAT     = 471
	__SYS_UNLINKAT   = 472
	__SYS_READLINKAT = 473
	__SYS_SYMLINKAT  = 474
	__SYS_MKDIRAT    = 475
	__SYS_FREADLINK  = 551

	// We add O_NONBLOCK to prevent open from blocking if it is called on a named
	// pipe which has no writer.
	openFlagsCreate    = O_CREATE | O_NOFOLLOW
	openFlagsWriteOnly = O_WRONLY
	openFlagsReadOnly  = O_RDONLY
	openFlagsDirectory = O_DIRECTORY
	openFlagsDevice    = O_NONBLOCK | O_NOFOLLOW
	openFlagsSymlink   = O_SYMLINK
	openFlagsFile      = O_NONBLOCK | O_NOFOLLOW
	openFlagsChmod     = O_RDONLY | O_NONBLOCK
	openFlagsChtimes   = O_RDONLY | O_NONBLOCK
	openFlagsLstat     = O_RDONLY | O_NONBLOCK | O_NOFOLLOW
	openFlagsStat      = O_RDONLY | O_NONBLOCK
	openFlagsReadlink  = O_RDONLY | O_SYMLINK
	openFlagsTruncate  = O_WRONLY

	openFileReadOnlyFlags = O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_NONBLOCK | O_DIRECT
)

// https://github.com/apple/darwin-xnu/blob/main/bsd/sys/types.h#L151
type dev_t int32

func makedev(major, minor int) dev_t { return dev_t(major)<<8 | dev_t(minor)&0xFF }
func major(dev dev_t) int            { return int(dev >> 8) }
func minor(dev dev_t) int            { return int(dev & 0xFF) }

func openFile(path string, flags int, mode fs.FileMode) (*os.File, error) {
	f, err := os.OpenFile(path, flags, mode)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) && ((flags & O_NOFOLLOW) != 0) {
			flags &= ^O_NOFOLLOW
			flags |= syscall.O_SYMLINK
			f, err = os.OpenFile(path, flags, mode)
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

func openat(fd int, path string, flags int, mode uint32) (int, error) {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return -1, err
	}
	r, _, e := syscall.Syscall6(
		uintptr(__SYS_OPENAT),
		uintptr(fd),
		uintptr(unsafe.Pointer(p)),
		uintptr(flags),
		uintptr(mode),
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

func mkdirat(fd int, path string, mode uint32) error {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	_, _, e := syscall.Syscall(
		uintptr(__SYS_MKDIRAT),
		uintptr(fd),
		uintptr(unsafe.Pointer(p)),
		uintptr(mode),
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
