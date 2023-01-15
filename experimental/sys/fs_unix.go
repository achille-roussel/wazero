package sys

import (
	"io/fs"
	"os"
	"strings"
	"syscall"
	"time"
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
)

func rmdir(path string) error {
	return syscall.Rmdir(path)
}

func ignoringEINTR(do func() error) error {
	for {
		if err := do(); err != syscall.EINTR {
			return err
		}
	}
}

func chtimes(file *os.File, atime, mtime time.Time) error {
	fd := int(file.Fd())
	tv := []syscall.Timeval{
		syscall.NsecToTimeval(atime.UnixNano()),
		syscall.NsecToTimeval(mtime.UnixNano()),
	}
	return syscall.Futimes(fd, tv)
}

func makeMode(fileMode fs.FileMode) (mode uint32) {
	mode = uint32(fileMode.Perm())
	switch fileMode.Type() {
	case fs.ModeDevice:
		mode |= syscall.S_IFBLK
	case fs.ModeDevice | fs.ModeCharDevice:
		mode |= syscall.S_IFCHR
	case fs.ModeDir:
		mode |= syscall.S_IFDIR
	case fs.ModeNamedPipe:
		mode |= syscall.S_IFIFO
	case fs.ModeSymlink:
		mode |= syscall.S_IFLNK
	case fs.ModeSocket:
		mode |= syscall.S_IFSOCK
	default:
		mode |= syscall.S_IFREG
	}
	if (fileMode & fs.ModeSetgid) != 0 {
		mode |= syscall.S_ISGID
	}
	if (fileMode & fs.ModeSetuid) != 0 {
		mode |= syscall.S_ISUID
	}
	if (fileMode & fs.ModeSticky) != 0 {
		mode |= syscall.S_ISVTX
	}
	return mode
}

func fgetxattr(fd int, name string) (string, bool, error) {
	p, err := syscall.BytePtrFromString(name)
	if err != nil {
		return "", false, err
	}
	buf := make([]byte, 64)
	for {
		r, _, e := syscall.Syscall6(
			uintptr(syscall.SYS_FGETXATTR),
			uintptr(fd),
			uintptr(unsafe.Pointer(p)),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
			uintptr(0),
			uintptr(0),
		)
		switch e {
		case 0:
			return string(buf[:r]), true, nil
		case syscall.ENODATA:
			return "", false, nil
		case syscall.ERANGE:
			buf = make([]byte, 2*len(buf))
		default:
			return "", false, e
		}
	}
}

func fsetxattr(fd int, name, value string, flags int) error {
	p, err := syscall.BytePtrFromString(name)
	if err != nil {
		return err
	}
	_, _, e := syscall.Syscall6(
		uintptr(syscall.SYS_FSETXATTR),
		uintptr(fd),
		uintptr(unsafe.Pointer(p)),
		uintptr(*(*unsafe.Pointer)(unsafe.Pointer(&value))),
		uintptr(len(value)),
		uintptr(flags),
		uintptr(0),
	)
	if e != 0 {
		return e
	}
	return nil
}

func flistxattr(fd int) ([]string, error) {
	buf := make([]byte, 1024)
	for {
		r, _, e := syscall.Syscall(
			uintptr(syscall.SYS_FLISTXATTR),
			uintptr(fd),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
		)
		switch e {
		case 0:
			if r == 0 {
				return nil, nil
			}
			names := strings.TrimSuffix(string(buf[:r]), "\x00")
			return strings.Split(names, "\x00"), nil
		case syscall.ERANGE:
			buf = make([]byte, 2*len(buf))
		default:
			return nil, e
		}
	}
}
