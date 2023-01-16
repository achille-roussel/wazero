package sys

import (
	"io/fs"
	"os"
	"syscall"
	"time"
)

func openFile(path string, flags int, perm fs.FileMode) (*os.File, error) {
	return openFileAt(__AT_FDCWD, "", path, flags, perm)
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
