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
	O_NOFOLLOW  = syscall.O_NOFOLLOW
	O_DIRECTORY = syscall.O_DIRECTORY
	O_NONBLOCK  = syscall.O_NONBLOCK
	O_SYMLINK   = syscall.O_SYMLINK
	O_SHLOCK    = syscall.O_SHLOCK
	O_EXLOCK    = syscall.O_EXLOCK
	O_EVTONLY   = syscall.O_EVTONLY
	O_CLOEXEC   = syscall.O_CLOEXEC

	// Darwin does not have O_DSYNC/O_RSYNC, so fallback to O_SYNC.
	O_DSYNC = syscall.O_SYNC
	O_RSYNC = syscall.O_SYNC

	F_OK = 0
	X_OK = 1
	W_OK = 2
	R_OK = 4

	__AT_FDCWD          = -2
	__AT_SYMLINK_FOLLOW = 0x0040
	__AT_REMOVEDIR      = 0x0080

	// https://go.googlesource.com/sys/+/master/unix/zsysnum_darwin_amd64.go
	__SYS_OPENAT    = 463
	__SYS_RENAMEAT  = 465
	__SYS_FACCESSAT = 466
	__SYS_LINKAT    = 471
	__SYS_UNLINKAT  = 472
	__SYS_SYMLINKAT = 474
	__SYS_MKDIRAT   = 475
	__SYS_FREADLINK = 551

	// We add O_NONBLOCK to prevent open from blocking if it is called on a named
	// pipe which has no writer.
	openFlagsCopy      = O_CREATE | O_EXCL | O_NOFOLLOW
	openFlagsCreate    = O_CREATE | O_RDWR | O_TRUNC
	openFlagsWriteOnly = O_WRONLY
	openFlagsReadOnly  = O_RDONLY
	openFlagsDirectory = O_DIRECTORY
	openFlagsNode      = O_NONBLOCK | O_NOFOLLOW
	openFlagsSymlink   = O_SYMLINK | O_NOFOLLOW
	openFlagsReadlink  = O_SYMLINK | O_NOFOLLOW
	openFlagsFile      = O_NONBLOCK | O_NOFOLLOW
	openFlagsChmod     = O_RDONLY | O_NONBLOCK
	openFlagsChtimes   = O_RDONLY | O_NONBLOCK
	openFlagsTruncate  = O_WRONLY | O_NONBLOCK
	openFlagsLstat     = O_RDONLY | O_NONBLOCK | O_NOFOLLOW
	openFlagsStat      = O_RDONLY | O_NONBLOCK
	openFlagsReadDir   = O_DIRECTORY
	openFlagsWriteFile = O_CREATE | O_NOFOLLOW | O_TRUNC | O_WRONLY
	openFlagsNoFollow  = O_NOFOLLOW
	openFlagsPath      = O_RDONLY | O_NONBLOCK | O_NOFOLLOW

	openFileReadOnlyFlags = O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_NONBLOCK | O_SYMLINK

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
	setOpenFlag(O_NOFOLLOW, "O_NOFOLLOW")
	setOpenFlag(O_NONBLOCK, "O_NONBLOCK")
	setOpenFlag(O_SYMLINK, "O_SYMLINK")
	setOpenFlag(O_SHLOCK, "O_SHLOCK")
	setOpenFlag(O_EXLOCK, "O_EXLOCK")
	setOpenFlag(O_EVTONLY, "O_EVTONLY")
	setOpenFlag(O_CLOEXEC, "O_CLOEXEC")
}

// https://github.com/apple/darwin-xnu/blob/main/bsd/sys/types.h#L151
type dev_t uint32

func makedev(major, minor int) dev_t { return dev_t(major)<<8 | dev_t(minor)&0xFF }
func major(dev dev_t) int            { return int(dev >> 8) }
func minor(dev dev_t) int            { return int(dev & 0xFF) }

func openFile(path string, flags int, perm fs.FileMode) (*os.File, error) {
	return openFileAt(__AT_FDCWD, "", path, flags, perm)
}

func openFileAt(dirfd int, dir, path string, flags int, perm fs.FileMode) (*os.File, error) {
	// The combination of O_SYMLINNK and O_NOFOLLOW is invalid on Darwin,
	// but it helps to be flexible so we handle this here by removing the
	// O_NOFOLLOW flag if the code asked to open a symbolic link.
	if (flags & O_SYMLINK) != 0 {
		flags &= ^O_NOFOLLOW
	}
	flags |= syscall.O_CLOEXEC
	mode := sysinfo.FileMode(perm)
	newfd, err := openat(dirfd, path, flags, mode)
	if err != nil {
		if err == syscall.ELOOP {
			if (flags & (O_DIRECTORY | O_SYMLINK | O_NOFOLLOW)) == O_NOFOLLOW {
				flags &= ^O_NOFOLLOW
				flags |= O_SYMLINK
				newfd, err = openat(dirfd, path, flags, mode)
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

func chtimes(file *os.File, atime, mtime time.Time) error {
	return syscall.Futimes(int(file.Fd()), []syscall.Timeval{
		syscall.NsecToTimeval(atime.UnixNano()),
		syscall.NsecToTimeval(mtime.UnixNano()),
	})
}

func mknodat(fd int, path string, mode uint32, dev int) error {
	return syscall.ENOSYS
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

func symlinkat(target string, fd int, path string) error {
	if target == "" {
		// Unlike Linux, Darwin allows creating links with empty targets.
		return syscall.ENOENT
	}
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

func faccessat(fd int, path string, mode uint32, flags int) error {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	_, _, e := syscall.Syscall6(
		uintptr(__SYS_FACCESSAT),
		uintptr(fd),
		uintptr(unsafe.Pointer(p)),
		uintptr(mode),
		uintptr(flags),
		uintptr(0),
		uintptr(0),
	)
	if e != 0 {
		return e
	}
	return nil
}
