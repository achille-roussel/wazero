package wasi

import (
	"errors"
	"io"
	"io/fs"
	"os"

	"github.com/tetratelabs/wazero/wasi/syscall"
)

const (
	O_RDONLY = os.O_RDONLY
	O_WRONLY = os.O_WRONLY
	O_RDWR   = os.O_RDWR
	O_APPEND = os.O_APPEND
	O_CREATE = os.O_CREATE
	O_EXCL   = os.O_EXCL
	O_SYNC   = os.O_SYNC
	O_TRUNC  = os.O_TRUNC

	// TODO: figure out O_DIRECTORY and other flags
)

func makeErrno(err error) syscall.Errno {
	if errno, ok := err.(syscall.Errno); ok {
		return errno
	}
	switch {
	case errors.Is(err, nil):
		return syscall.ESUCCESS
	case errors.Is(err, io.EOF):
		return syscall.ESUCCESS
	case errors.Is(err, fs.ErrInvalid):
		return syscall.EINVAL
	case errors.Is(err, fs.ErrPermission):
		return syscall.EPERM
	case errors.Is(err, fs.ErrExist):
		return syscall.EEXIST
	case errors.Is(err, fs.ErrNotExist):
		return syscall.ENOENT
	case errors.Is(err, fs.ErrClosed):
		return syscall.EBADF
	case errors.Is(err, ErrNotImplemented):
		return syscall.ENOSYS
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno
	}
	return syscall.ENOTCAPABLE
}

func makeError(errno syscall.Errno) error {
	switch errno {
	case syscall.ESUCCESS:
		return nil
	case syscall.EINVAL:
		return fs.ErrInvalid
	case syscall.EPERM:
		return fs.ErrPermission
	case syscall.EEXIST:
		return fs.ErrExist
	case syscall.ENOENT:
		return fs.ErrNotExist
	case syscall.EBADF:
		return fs.ErrClosed
	case syscall.ENOSYS:
		return ErrNotImplemented
	default:
		return errno
	}
}

func makeOpenFileFlags(dirflags syscall.Lookupflags, oflags syscall.Oflags, fsRightsBase, fsRightsInheriting syscall.Rights, fdflags syscall.Fdflags) (flags int, perm fs.FileMode) {
	if (oflags & syscall.O_CREAT) != 0 {
		flags |= O_CREATE
	}
	if (oflags & syscall.O_EXCL) != 0 {
		flags |= O_EXCL
	}
	if (oflags & syscall.O_TRUNC) != 0 {
		flags |= O_TRUNC
	}
	if (fdflags & syscall.F_APPEND) != 0 {
		flags |= O_APPEND
	}
	if (fdflags & (syscall.F_DSYNC | syscall.F_RSYNC | syscall.F_SYNC)) != 0 {
		flags |= O_SYNC
	}
	switch {
	case (fsRightsBase & (syscall.FD_READ | syscall.FD_WRITE)) == (syscall.FD_READ | syscall.FD_WRITE):
		flags |= O_RDWR
	case (fsRightsBase & syscall.FD_WRITE) != 0:
		flags |= O_WRONLY
	default:
		flags |= O_RDONLY
	}
	perm = 0644
	return
}

func makePathOpenFlags(flags int, perm fs.FileMode) (dirflags syscall.Lookupflags, oflags syscall.Oflags, fsRightsBase, fsRightsInheriting syscall.Rights, fdflags syscall.Fdflags) {
	const (
		defaultRights = syscall.FD_SEEK | syscall.FD_TELL | syscall.FD_FILESTAT_GET | syscall.PATH_OPEN
		readRights    = syscall.FD_READ | syscall.FD_READDIR
		writeRights   = syscall.FD_WRITE
	)

	switch {
	case (flags & O_RDWR) != 0:
		fsRightsBase = defaultRights | readRights | writeRights
	case (flags & O_WRONLY) != 0:
		fsRightsBase = defaultRights | writeRights
	default:
		fsRightsBase = defaultRights | readRights
	}

	if perm != 0 {
		if (perm & 0400) == 0 {
			fsRightsBase &= ^readRights
		}
		if (perm & 0200) == 0 {
			fsRightsBase &= ^writeRights
		}
	}

	if (flags & O_APPEND) != 0 {
		fdflags |= syscall.F_APPEND
	}
	if (flags & O_CREATE) != 0 {
		oflags |= syscall.O_CREAT
	}
	if (flags & O_EXCL) != 0 {
		oflags |= syscall.O_EXCL
	}
	if (flags & O_SYNC) != 0 {
		fdflags |= syscall.F_SYNC
	}
	if (flags & O_TRUNC) != 0 {
		oflags |= syscall.O_TRUNC
	}

	fsRightsInheriting = ^syscall.Rights(0)
	dirflags = makeLookupflags(flags)
	return
}

func makeLookupflags(flags int) (dirflags syscall.Lookupflags) {
	// TODO
	return
}

func makeStatFileFlags(dirflags syscall.Lookupflags) (flags int) {
	// TODO
	return
}

func makeFilestat(info fs.FileInfo) syscall.Filestat {
	return syscall.Filestat{
		Dev:      syscall.Device(0), // TODO?
		Ino:      syscall.Inode(0),  // TODO?
		Filetype: makeFiletype(info.Mode()),
		Nlink:    syscall.Linkcount(0), // TODO?
		Size:     syscall.Filesize(info.Size()),
		Atim:     syscall.Timestamp(0), // TODO?
		Mtim:     syscall.Timestamp(info.ModTime().UnixNano()),
		Ctim:     syscall.Timestamp(0), // TODO?
	}
}

func makeFileMode(typ syscall.Filetype) fs.FileMode {
	switch typ {
	case syscall.BlockDevice:
		return fs.ModeDevice
	case syscall.CharacterDevice:
		return fs.ModeCharDevice
	case syscall.Directory:
		return fs.ModeDir
	case syscall.RegularFile:
		return 0
	case syscall.SocketDgram, syscall.SocketStream:
		return fs.ModeSocket
	case syscall.SymbolicLink:
		return fs.ModeSymlink
	default: // Unknown
		return fs.ModeIrregular
	}
}

func makeFiletype(mode fs.FileMode) syscall.Filetype {
	switch mode & fs.ModeType {
	case fs.ModeDir:
		return syscall.Directory
	case fs.ModeSymlink:
		return syscall.SymbolicLink
	case fs.ModeDevice:
		return syscall.BlockDevice
	case fs.ModeSocket:
		return syscall.SocketStream
	case fs.ModeCharDevice:
		return syscall.CharacterDevice
	case fs.ModeNamedPipe, fs.ModeIrregular:
		return syscall.Unknown
	default:
		return syscall.RegularFile
	}
}
