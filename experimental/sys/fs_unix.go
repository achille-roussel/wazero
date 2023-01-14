package sys

import (
	"io/fs"
	"os"
	"syscall"
	"time"
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
