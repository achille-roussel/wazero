package sys

import (
	"os"
	"syscall"
	"time"
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
