package sys

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
	"time"
)

func openFile(path string, flags int, perm fs.FileMode) (*os.File, error) {
	f, err := os.OpenFile(path, flags^O_DIRECTORY, perm)
	if err != nil {
		return nil, newPathError("open", name, errors.Unwrap(err))
	}

	if (flags & O_DIRECTORY) != 0 {
		s, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		if !s.IsDir() {
			f.Close()
			return nil, newPathError("open", name, ErrNotDirectory)
		}
	}

	return f, nil
}

func rmdir(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return syscall.RemoveDirectory(p)
}

func unlink(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return syscall.DeleteFile(p)
}

func chtimes(file *os.File, atime, mtime time.Time) (err error) {
	return syscall.Utimes(file.Name(), []syscall.Timeval{
		syscall.NsecToTimeval(atime.UnixNano()),
		syscall.NsecToTimeval(mtime.UnixNano()),
	})
}
