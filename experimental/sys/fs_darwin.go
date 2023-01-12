package sys

import (
	"io/fs"
	"os"
	"syscall"
	"time"
)

const (
	// Darwin does not have O_DSYNC/O_RSYNC, so fallback to O_SYNC.
	O_DSYNC = syscall.O_SYNC
	O_RSYNC = syscall.O_SYNC
)

func openFile(path string, flags int, perm fs.FileMode) (*os.File, error) {
	if (flags & (O_DIRECTORY | O_NOFOLLOW)) == O_NOFOLLOW {
		// Darwin requires that open be given explicit permission to open
		// symbolic links. We inject the flag here when O_NOFOLLOW is given
		// and the appliation is not trying to open a directory.
		flags |= syscall.O_SYMLINK
	}
	return os.OpenFile(path, flags, perm)
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

func (info *fileInfo) ModTime() time.Time {
	return time.Unix(info.stat.Mtimespec.Unix())
}
