package sys

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"github.com/tetratelabs/wazero/experimental/sys/sysinfo"
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
	O_DIRECTORY = syscall.O_DIRECTORY
	O_DSYNC     = syscall.O_DSYNC
	O_RSYNC     = syscall.O_RSYNC
	O_DIRECT    = syscall.O_DIRECT
	O_LARGEFILE = syscall.O_LARGEFILE
	O_NOATIME   = syscall.O_NOATIME
	O_NOCTTY    = syscall.O_NOCTTY
	O_NOFOLLOW  = syscall.O_NOFOLLOW
	O_NONBLOCK  = syscall.O_NONBLOCK
	O_ASYNC     = syscall.O_ASYNC
	O_CLOEXEC   = syscall.O_CLOEXEC

	// https://github.com/torvalds/linux/blob/master/include/uapi/asm-generic/fcntl.h
	O_PATH    = 010000000
	O_TMPFILE = 020000000

	__AT_FDCWD          = -100
	__AT_REMOVEDIR      = 0x200
	__AT_SYMLINK_FOLLOW = 0x400

	// We add O_NONBLOCK to prevent open from blocking if it is called on a named
	// pipe which has no writer.
	openFlagsCopy      = O_CREATE | O_EXCL | O_NOFOLLOW
	openFlagsCreate    = O_CREATE | O_RDWR | O_TRUNC
	openFlagsWriteOnly = O_WRONLY
	openFlagsReadOnly  = O_RDONLY
	openFlagsDirectory = O_DIRECTORY
	openFlagsNode      = O_NOFOLLOW | O_NONBLOCK
	openFlagsSymlink   = O_NOFOLLOW | O_PATH
	openFlagsReadlink  = O_NOFOLLOW | O_PATH
	openFlagsFile      = O_NOFOLLOW | O_NONBLOCK
	openFlagsChmod     = O_RDONLY | O_NONBLOCK
	openFlagsChtimes   = O_RDONLY | O_NONBLOCK
	openFlagsTruncate  = O_WRONLY | O_NONBLOCK
	openFlagsLstat     = O_RDONLY | O_NONBLOCK | O_NOFOLLOW
	openFlagsStat      = O_RDONLY | O_NONBLOCK
	openFlagsReadDir   = O_DIRECTORY
	openFlagsWriteFile = O_CREATE | O_NOFOLLOW | O_TRUNC | O_WRONLY
	openFlagsNoFollow  = O_NOFOLLOW
	openFlagsPath      = O_NOFOLLOW | O_PATH

	openFileReadOnlyFlags = O_RDONLY | O_DIRECTORY | O_DIRECT | O_LARGEFILE | O_NOATIME | O_NOCTTY | O_NOFOLLOW | O_NONBLOCK | O_PATH

	openFlagsCount = 32
)

func init() {
	setOpenFlag(O_RDONLY, "O_RDONLY")
	setOpenFlag(O_WRONLY, "O_WRONLY")
	setOpenFlag(O_RDWR, "O_RDWR")
	setOpenFlag(O_APPEND, "O_APPEND")
	setOpenFlag(O_CREATE, "O_CREATE")
	setOpenFlag(O_EXCL, "O_EXCL")
	setOpenFlag(O_SYNC, "O_SYNC")
	setOpenFlag(O_TRUNC, "O_TRUNC")
	setOpenFlag(O_DIRECTORY, "O_DIRECTORY")
	setOpenFlag(O_DSYNC, "O_DSYNC")
	setOpenFlag(O_RSYNC, "O_RSYNC")
	setOpenFlag(O_DIRECT, "O_DIRECT")
	setOpenFlag(O_LARGEFILE, "O_LARGEFILE")
	setOpenFlag(O_NOATIME, "O_NOATIME")
	setOpenFlag(O_NOCTTY, "O_NOCTTY")
	setOpenFlag(O_NOFOLLOW, "O_NOFOLLOW")
	setOpenFlag(O_NONBLOCK, "O_NONBLOCK")
	setOpenFlag(O_ASYNC, "O_PATH")
	setOpenFlag(O_CLOEXEC, "O_CLOEXEC")
	setOpenFlag(O_TMPFILE, "O_TMPFILE")
	setOpenFlag(O_PATH, "O_PATH")
}

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

func openFile(path string, flags int, perm fs.FileMode) (*os.File, error) {
	return openFileAt(__AT_FDCWD, "", path, flags, perm)
}

func openFileAt(dirfd int, dir, path string, flags int, perm fs.FileMode) (*os.File, error) {
	flags |= syscall.O_CLOEXEC
	mode := sysinfo.FileMode(perm)
	newfd, err := openat(dirfd, path, flags, mode)
	if err != nil {
		// Linux gives ELOOP if attempting to open a symbolic link without
		// passing the O_PATH flag.
		if err == syscall.ELOOP {
			if (flags & (O_DIRECTORY | O_NOFOLLOW | O_PATH)) == O_NOFOLLOW {
				newfd, err = openat(dirfd, path, flags|O_PATH, mode)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if dir != "" {
		path = filepath.Join(dir, path)
	}
	return os.NewFile(uintptr(newfd), path), nil
}

func readlink(file *os.File) (string, error) {
	return readlinkat(int(file.Fd()), "")
}

func datasync(file *os.File) error {
	fd := int(file.Fd())
	for {
		if err := syscall.Fdatasync(fd); err != syscall.EINTR {
			return err
		}
	}
}

func chtimes(file *os.File, atime, mtime time.Time) error {
	err := syscall.Futimes(int(file.Fd()), []syscall.Timeval{
		syscall.NsecToTimeval(atime.UnixNano()),
		syscall.NsecToTimeval(mtime.UnixNano()),
	})
	// This error may occur on Linux due to having futimes implemented as
	// chtimes on "/proc/fd/*". We normalize it back to EBADF since this
	// is the error that is returned on other unix platforms when making
	// a syscall with a closed or invalid file descriptor.
	if err == syscall.ENOENT {
		err = syscall.EBADF
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

func faccessat(dirfd int, path string, mode uint32, flags int) error {
	return syscall.Faccessat(dirfd, path, mode, flags)
}
