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

func openFile(path string, flags int, perm fs.FileMode) (*os.File, error) {
	// On unix platforms, the O_DIRECTORY flag is supported and can be handled
	// by the kernel, we don't need to emulate it by verifying that the opened
	// file is a directory.
	return os.OpenFile(path, flags, perm)
}

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

type fileInfo struct {
	stat syscall.Stat_t
	name string
}

func (info *fileInfo) Name() string {
	return info.name
}

func (info *fileInfo) Size() int64 {
	return info.stat.Size
}

func (info *fileInfo) Mode() (mode fs.FileMode) {
	mode = fs.FileMode(info.stat.Mode) & 0777
	switch info.stat.Mode & syscall.S_IFMT {
	case syscall.S_IFBLK:
		mode |= fs.ModeDevice
	case syscall.S_IFCHR:
		mode |= fs.ModeDevice | fs.ModeCharDevice
	case syscall.S_IFDIR:
		mode |= fs.ModeDir
	case syscall.S_IFIFO:
		mode |= fs.ModeNamedPipe
	case syscall.S_IFLNK:
		mode |= fs.ModeSymlink
	case syscall.S_IFREG:
		// nothing to do
	case syscall.S_IFSOCK:
		mode |= fs.ModeSocket
	}
	if info.stat.Mode&syscall.S_ISGID != 0 {
		mode |= fs.ModeSetgid
	}
	if info.stat.Mode&syscall.S_ISUID != 0 {
		mode |= fs.ModeSetuid
	}
	if info.stat.Mode&syscall.S_ISVTX != 0 {
		mode |= fs.ModeSticky
	}
	return mode
}

func (info *fileInfo) Sys() interface{} {
	return &info.stat
}

func (info *fileInfo) IsDir() bool {
	return info.Mode().IsDir()
}
