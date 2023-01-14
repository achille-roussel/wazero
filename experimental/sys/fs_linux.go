package sys

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
	"unsafe"
)

const (
	O_DSYNC = syscall.O_DSYNC
	O_RSYNC = syscall.O_RSYNC

	// https://github.com/torvalds/linux/blob/master/include/uapi/asm-generic/fcntl.h
	O_PATH = 010000000

	__AT_REMOVEDIR      = 0x200
	__AT_SYMLINK_FOLLOW = 0x400

	openFileReadOnlyFlags = O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_PATH
)

type dev_t uint64

func makedev(major, minor int) (dev dev_t) {
	maj := dev_t(major)
	min := dev_t(minor)
	// use glibc's encoding, see:
	// https://stackoverflow.com/questions/9635702/in-posix-how-is-type-dev-t-getting-used
	dev |= (min & 0x000000FF)
	dev |= (maj & 0x00000FFF) << 8
	dev |= (min & 0xFFFFFF00) << 12
	dev |= (maj & 0xFFFFF000) << 32
	return dev
}

func major(dev dev_t) int {
	maj := dev_t(0)
	maj |= (dev & 0x00000000000FFF00) >> 8
	maj |= (dev & 0xFFFFF00000000000) >> 32
	return int(maj)
}

func minor(dev dev_t) int {
	min := dev_t(0)
	min |= (dev & 0x00000000000000FF) >> 0
	min |= (dev & 0x00000FFFFFF00000) >> 12
	return int(min)
}

func openFile(path string, flags int, mode fs.FileMode) (*os.File, error) {
	f, err := os.OpenFile(path, flags, mode)
	if err != nil {
		// Linux gives ELOOP if attempting to open a symbolic link without
		// passing the O_PATH flag.
		if errors.Is(err, syscall.ELOOP) {
			if (flags & (O_DIRECTORY | O_NOFOLLOW | O_PATH)) == O_NOFOLLOW {
				f, err = os.OpenFile(path, flags|O_PATH, mode)
			}
		}
	}
	return f, err
}

func readlink(file *os.File) (string, error) {
	return readlinkat(int(file.Fd()), "")
}

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

func openat(fd int, path string, flags int, mode uint32) (int, error) {
	return syscall.Openat(fd, path, flags, mode)
}

func mknodat(fd int, path string, mode uint32, dev int) error {
	return syscall.Mknodat(fd, path, mode, dev)
}

func mkdirat(fd int, path string, mode uint32) error {
	return syscall.Mkdirat(fd, path, mode)
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

func readlinkat(fd int, path string) (string, error) {
	buf := [1025]byte{}
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return "", err
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
		// When readlinkat is called with no path, we mean to read the link
		// target of the symbolic link that fd is opened on, in which case the
		// error codes are somewhat different than when there is a non-empty
		// path and the fd is opened on a directory.
		if path == "" {
			switch e {
			case syscall.ENOENT:
				e = syscall.EINVAL
			}
		}
		return "", e
	}
	if int(n) == len(buf) {
		return "", syscall.ENAMETOOLONG
	}
	return string(buf[:n]), nil
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
