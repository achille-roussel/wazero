//go:build !purego && linux

package sysfs

import (
	"io"
	"io/fs"
	fspath "path"
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
	O_DSYNC     = syscall.O_DSYNC
	O_RSYNC     = syscall.O_RSYNC
)

type file struct {
	fd   int
	name string
}

func opendir(path string) (*file, error) {
	fd, err := syscall.Open(path, syscall.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	return &file{fd: fd, name: "."}, nil
}

func (f *file) makeFileError(op string, err error) error {
	return makePathError(op, f.name, err)
}

func (f *file) makePathError(op, path string, err error) error {
	return makePathError(op, f.pathTo(path), err)
}

func (f *file) pathTo(path string) string {
	return fspath.Join(f.name, path)
}

func (f *file) Name() string {
	return f.name
}

func (f *file) Close() (err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		fd := f.fd
		f.fd = -1
		for {
			if err = syscall.Close(fd); err != syscall.EINTR {
				break
			}
		}
	}
	if err != nil {
		err = f.makeFileError("close", err)
	}
	return err
}

func (f *file) ReadDir(n int) (dirent []fs.DirEntry, err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		err = syscall.ENOSYS
	}
	return nil, f.makeFileError("readdir", err)
}

func (f *file) Read(b []byte) (n int, err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		var r int
		for {
			r, err = syscall.Read(f.fd, b[n:])
			n += r
			if err != syscall.EINTR {
				if n < len(b) && err == nil {
					return n, io.EOF
				}
				break
			}
			if n == len(b) {
				break
			}
		}
	}
	if err != nil {
		err = f.makeFileError("read", err)
	}
	return n, err
}

func (f *file) ReadAt(b []byte, off int64) (n int, err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		var r int
		for {
			r, err = syscall.Pread(f.fd, b[n:], off+int64(n))
			n += r
			if err != syscall.EINTR {
				if n < len(b) && err == nil {
					return n, io.EOF
				}
				break
			}
			if n == len(b) {
				break
			}
		}
	}
	if err != nil {
		err = f.makeFileError("read", err)
	}
	return n, err
}

func (f *file) Write(b []byte) (n int, err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		var w int
		for {
			w, err = syscall.Write(f.fd, b[n:])
			n += w
			if err != syscall.EINTR {
				break
			}
			if n == len(b) {
				break
			}
		}
	}
	if err != nil {
		err = f.makeFileError("write", err)
	}
	return n, err
}

func (f *file) WriteAt(b []byte, off int64) (n int, err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		var w int
		for {
			w, err = syscall.Pwrite(f.fd, b[n:], off+int64(n))
			n += w
			if err != syscall.EINTR {
				break
			}
			if n == len(b) {
				break
			}
		}
	}
	if err != nil {
		err = f.makeFileError("write", err)
	}
	return n, err
}

func (f *file) Seek(offset int64, whence int) (seek int64, err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		seek, err = syscall.Seek(f.fd, offset, whence)
	}
	if err != nil {
		err = f.makeFileError("seek", err)
	}
	return seek, err
}

func (f *file) Chmod(perm fs.FileMode) (err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		err = syscall.Fchmod(f.fd, uint32(perm))
	}
	if err != nil {
		err = f.makeFileError("chmod", err)
	}
	return err
}

func (f *file) Chtimes(atim, mtim time.Time) (err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		tv := [2]syscall.Timeval{
			syscall.NsecToTimeval(atim.Unix()),
			syscall.NsecToTimeval(mtim.Unix()),
		}
		err = syscall.Futimes(f.fd, tv[:])
	}
	if err != nil {
		err = f.makeFileError("chtimes", err)
	}
	return err
}

func (f *file) Open(path string, flags int, perm fs.FileMode) (File, error) {
	var fd int
	var err error
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		for {
			fd, err = syscall.Openat(f.fd, path, flags, uint32(perm))
			if err != syscall.EINTR {
				break
			}
		}
	}
	if err != nil {
		return nil, f.makePathError("open", path, err)
	}
	return &file{fd: fd, name: fspath.Join(f.name, path)}, nil
}

func (f *file) Unlink(path string) (err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		err = unlinkat(f.fd, path, 0)
	}
	if err != nil {
		err = f.makePathError("unlink", path, err)
	}
	return err
}

func (f *file) Rename(oldPath, newPath string) (err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		err = syscall.Renameat(f.fd, oldPath, f.fd, newPath)
	}
	if err != nil {
		err = f.makePathError("rename", oldPath, err)
	}
	return err
}

func (f *file) Link(oldPath, newPath string) (err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		err = linkat(f.fd, oldPath, f.fd, newPath, 0)
	}
	if err != nil {
		err = f.makePathError("link", oldPath, err)
	}
	return err
}

func (f *file) Symlink(oldPath, newPath string) (err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		err = symlinkat(oldPath, f.fd, newPath)
	}
	if err != nil {
		err = f.makePathError("symlink", oldPath, err)
	}
	return err
}

func (f *file) Readlink(path string) (link string, err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		link, err = readlinkat(f.fd, path)
	}
	if err != nil {
		err = f.makePathError("readlink", path, err)
	}
	return link, err
}

func (f *file) Mkdir(path string, perm fs.FileMode) (err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		err = syscall.Mkdirat(f.fd, path, uint32(perm))
	}
	if err != nil {
		err = f.makePathError("mkdir", path, err)
	}
	return err
}

func (f *file) Rmdir(path string) (err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		err = unlinkat(f.fd, path, __AT_REMOVEDIR)
	}
	if err != nil {
		err = f.makePathError("rmdir", path, err)
	}
	return err
}

func (f *file) Stat() (stat fs.FileInfo, err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		err = syscall.ENOSYS
	}
	return nil, f.makeFileError("stat", err)
}

func (f *file) Sync() (err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		err = syscall.Fsync(f.fd)
	}
	if err != nil {
		err = f.makeFileError("sync", err)
	}
	return err
}

func (f *file) Datasync() (err error) {
	if f.fd < 0 {
		err = fs.ErrClosed
	} else {
		err = syscall.Fdatasync(f.fd)
	}
	if err != nil {
		err = f.makeFileError("datasync", err)
	}
	return err
}

const (
	// https://elixir.bootlin.com/linux/latest/source/include/uapi/linux/fcntl.h#L101
	__AT_REMOVEDIR = 0x200
	// https://elixir.bootlin.com/linux/latest/source/include/uapi/linux/limits.h#L13
	__PATH_MAX = 4096
)

// The syscall package does not have a function for some of the syscalls we rely
// on, so we provide implementations here.

func linkat(oldDirFd int, oldPath string, newDirFd int, newPath string, flags int) error {
	oldPathBytes, err := syscall.BytePtrFromString(oldPath)
	if err != nil {
		return err
	}
	newPathBytes, err := syscall.BytePtrFromString(newPath)
	if err != nil {
		return err
	}
	_, _, errno := syscall.Syscall6(
		uintptr(syscall.SYS_LINKAT),
		uintptr(oldDirFd),
		uintptr(unsafe.Pointer(oldPathBytes)),
		uintptr(newDirFd),
		uintptr(unsafe.Pointer(newPathBytes)),
		uintptr(flags),
		uintptr(0),
	)
	return syscallError(errno)
}

func symlinkat(target string, newDirFd int, linkPath string) error {
	targetBytes, err := syscall.BytePtrFromString(target)
	if err != nil {
		return err
	}
	linkPathBytes, err := syscall.BytePtrFromString(linkPath)
	if err != nil {
		return err
	}
	_, _, errno := syscall.Syscall(
		uintptr(syscall.SYS_SYMLINKAT),
		uintptr(unsafe.Pointer(targetBytes)),
		uintptr(newDirFd),
		uintptr(unsafe.Pointer(linkPathBytes)),
	)
	return syscallError(errno)
}

func readlinkat(dirFd int, path string) (string, error) {
	pathBytes, err := syscall.BytePtrFromString(path)
	if err != nil {
		return "", err
	}
	b := [__PATH_MAX]byte{}
	n, _, errno := syscall.Syscall6(
		uintptr(syscall.SYS_READLINKAT),
		uintptr(dirFd),
		uintptr(unsafe.Pointer(pathBytes)),
		uintptr(unsafe.Pointer(&b)),
		uintptr(len(b)),
		uintptr(0),
		uintptr(0),
	)
	if errno != 0 {
		return "", syscallError(errno)
	}
	return string(b[:n]), nil
}

// The syscall.Unlinkat function does not support passing flags as argument,
// which is necessary in order to implement remove directories at a path
// relative to a file descriptor.
func unlinkat(dirFd int, path string, flags int) error {
	pathBytes, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	_, _, errno := syscall.Syscall(
		uintptr(syscall.SYS_UNLINKAT),
		uintptr(dirFd),
		uintptr(unsafe.Pointer(pathBytes)),
		uintptr(flags),
	)
	return syscallError(errno)
}

func syscallError(errno syscall.Errno) error {
	switch errno {
	case 0:
		return nil
	case syscall.EAGAIN:
		return errEAGAIN
	case syscall.EINVAL:
		return errEINVAL
	case syscall.ENOENT:
		return errENOENT
	default:
		return errno
	}
}

// Cache the conversion of syscall.Errno constants to error; the syscsall
// package uses the same strategy.
var (
	errEAGAIN error = syscall.EAGAIN
	errEINVAL error = syscall.EINVAL
	errENOENT error = syscall.ENOENT
)
