package sys

import (
	"os"
	"syscall"
	"time"
)

const (
	// Darwin does not have O_DSYNC/O_RSYNC, so fallback to O_SYNC.
	O_DSYNC = syscall.O_SYNC
	O_RSYNC = syscall.O_SYNC
)

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

func (info *fileInfo) ModTime() time.Time {
	return time.Unix(info.stat.Mtimespec.Unix())
}
