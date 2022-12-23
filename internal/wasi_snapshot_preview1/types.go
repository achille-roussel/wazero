package wasi_snapshot_preview1

import (
	"encoding/binary"
	"errors"
	"io"
	"io/fs"

	"github.com/tetratelabs/wazero/wasi"
)

const (
	// MaxPathLen is a constant representing the maximum supported length of
	// file system paths.
	MaxPathLen = 1024
)

type Device uint64

type Size uint32

type Linkcount uint64

type Filedelta int64

type Filesize uint64

type Filestat struct {
	Dev      Device
	Ino      Inode
	Filetype Filetype
	Nlink    Linkcount
	Size     Filesize
	Atim     Timestamp
	Mtim     Timestamp
	Ctim     Timestamp
}

func (s *Filestat) Marshal() (b [64]byte) {
	binary.LittleEndian.PutUint64(b[0:], uint64(s.Dev))
	binary.LittleEndian.PutUint64(b[8:], uint64(s.Ino))
	binary.LittleEndian.PutUint64(b[16:], uint64(s.Filetype))
	binary.LittleEndian.PutUint64(b[24:], uint64(s.Nlink))
	binary.LittleEndian.PutUint64(b[32:], uint64(s.Size))
	binary.LittleEndian.PutUint64(b[40:], uint64(s.Atim))
	binary.LittleEndian.PutUint64(b[48:], uint64(s.Mtim))
	binary.LittleEndian.PutUint64(b[56:], uint64(s.Ctim))
	return b
}

func (s *Filestat) Unmarshal(b [64]byte) {
	s.Dev = Device(binary.LittleEndian.Uint64(b[0:]))
	s.Ino = Inode(binary.LittleEndian.Uint64(b[0:]))
	s.Filetype = Filetype(binary.LittleEndian.Uint64(b[0:]))
	s.Nlink = Linkcount(binary.LittleEndian.Uint64(b[0:]))
	s.Size = Filesize(binary.LittleEndian.Uint64(b[0:]))
	s.Atim = Timestamp(binary.LittleEndian.Uint64(b[0:]))
	s.Mtim = Timestamp(binary.LittleEndian.Uint64(b[0:]))
	s.Ctim = Timestamp(binary.LittleEndian.Uint64(b[0:]))
}

type Filetype uint8

const (
	Unknown Filetype = iota
	BlockDevice
	CharacterDevice
	Directory
	RegularFile
	SocketDgram
	SocketStream
	SymbolicLink
)

type Timestamp uint64

type Inode uint64

type Dircookie uint64

type Dirnamlen uint32

type Dirent struct {
	Next    Dircookie
	Ino     Inode
	Namelen Dirnamlen
	Type    Filetype
}

func (d *Dirent) Size() Size { return 24 + Size(d.Namelen) }

func (d *Dirent) Marshal() (b [24]byte) {
	binary.LittleEndian.PutUint64(b[0:], uint64(d.Next))
	binary.LittleEndian.PutUint64(b[8:], uint64(d.Ino))
	binary.LittleEndian.PutUint32(b[16:], uint32(d.Namelen))
	binary.LittleEndian.PutUint32(b[20:], uint32(d.Type))
	return b
}

func (d *Dirent) Unmarshal(b [24]byte) {
	d.Next = Dircookie(binary.LittleEndian.Uint64(b[0:]))
	d.Ino = Inode(binary.LittleEndian.Uint64(b[8:]))
	d.Namelen = Dirnamlen(binary.LittleEndian.Uint32(b[16:]))
	d.Type = Filetype(binary.LittleEndian.Uint32(b[20:]))
}

type Whence uint8

const (
	Set Whence = iota
	Cur
	End
)

type Fd uint32

const (
	None Fd = ^Fd(0)
)

type Lookupflags uint32

const (
	SymlinkFollow Lookupflags = 1 << iota
)

func (f Lookupflags) Has(flags Lookupflags) bool { return (f & flags) == flags }

type Oflags uint16

const (
	O_CREAT Oflags = 1 << iota
	O_DIRECTORY
	O_EXCL
	O_TRUNC
)

func (f Oflags) Has(flags Oflags) bool { return (f & flags) == flags }

type Fdflags uint16

const (
	F_APPEND Fdflags = 1 << iota
	F_DSYNC
	F_NONBLOCK
	F_RSYNC
	F_SYNC
)

func (f Fdflags) Has(flags Fdflags) bool { return (f & flags) == flags }

type Rights uint64

const (
	FD_DATASYNC Rights = 1 << iota
	FD_READ
	FD_SEEK
	FD_FDSTAT_SET_FLAGS
	FD_SYNC
	FD_TELL
	FD_WRITE
	FD_ADVISE
	FD_ALLOCATE
	PATH_CREATE_DIRECTORY
	PATH_CREATE_FILE
	PATH_LINK_SOURCE
	PATH_LINK_TARGET
	PATH_OPEN
	FD_READDIR
	PATH_READLINK
	PATH_RENAME_SOURCE
	PATH_RENAME_TARGET
	PATH_FILESTAT_GET
	PATH_FILESTAT_SET_SIZE
	PATH_FILESTAT_SET_TIMES
	FD_FILESTAT_GET
	FD_FILESTAT_SET_SIZE
	FD_FILESTAT_SET_TIMES
	PATH_SYMLINK
	PATH_REMOVE_DIRECTORY
	PATH_UNLINK_FILE
	POLL_FD_READWRITE
	SOCK_SHUTDOWN
	SOCK_ACCEPT
)

func (r Rights) Has(rights Rights) bool { return (r & rights) == rights }

func makeErrno(err error) Errno {
	if errno, ok := err.(Errno); ok {
		return errno
	}
	switch {
	case errors.Is(err, nil):
		return ESUCCESS
	case errors.Is(err, io.EOF):
		return ESUCCESS
	case errors.Is(err, fs.ErrInvalid):
		return EINVAL
	case errors.Is(err, fs.ErrPermission):
		return EPERM
	case errors.Is(err, fs.ErrExist):
		return EEXIST
	case errors.Is(err, fs.ErrNotExist):
		return ENOENT
	case errors.Is(err, fs.ErrClosed):
		return EBADF
	case errors.Is(err, wasi.ErrNotImplemented):
		return ENOSYS
	}
	var errno Errno
	if errors.As(err, &errno) {
		return errno
	}
	return ENOTCAPABLE
}

func makeError(errno Errno) error {
	switch errno {
	case ESUCCESS:
		return nil
	case EINVAL:
		return fs.ErrInvalid
	case EPERM:
		return fs.ErrPermission
	case EEXIST:
		return fs.ErrExist
	case ENOENT:
		return fs.ErrNotExist
	case EBADF:
		return fs.ErrClosed
	case ENOSYS:
		return wasi.ErrNotImplemented
	default:
		return errno
	}
}

func makeOpenFileFlags(dirflags Lookupflags, oflags Oflags, fsRightsBase, fsRightsInheriting Rights, fdflags Fdflags) (flags int, perm fs.FileMode) {
	flags = makeDefaultFlags(dirflags)
	if (oflags & O_CREAT) != 0 {
		flags |= wasi.O_CREATE
	}
	if (oflags & O_EXCL) != 0 {
		flags |= wasi.O_EXCL
	}
	if (oflags & O_TRUNC) != 0 {
		flags |= wasi.O_TRUNC
	}
	if (fdflags & F_APPEND) != 0 {
		flags |= wasi.O_APPEND
	}
	if (fdflags & F_DSYNC) != 0 {
		flags |= wasi.O_DSYNC
	}
	if (fdflags & F_RSYNC) != 0 {
		flags |= wasi.O_RSYNC
	}
	if (fdflags & F_SYNC) != 0 {
		flags |= wasi.O_SYNC
	}
	switch {
	case (fsRightsBase & (FD_READ | FD_WRITE)) == (FD_READ | FD_WRITE):
		flags |= wasi.O_RDWR
	case (fsRightsBase & FD_WRITE) != 0:
		flags |= wasi.O_WRONLY
	default:
		flags |= wasi.O_RDONLY
	}
	perm = 0644
	return
}

func makePathOpenFlags(flags int, perm fs.FileMode) (dirflags Lookupflags, oflags Oflags, fsRightsBase, fsRightsInheriting Rights, fdflags Fdflags) {
	const (
		defaultRights = FD_SEEK | FD_TELL | FD_FILESTAT_GET | PATH_OPEN
		readRights    = FD_READ | FD_READDIR
		writeRights   = FD_WRITE
	)

	switch {
	case (flags & wasi.O_RDWR) != 0:
		fsRightsBase = defaultRights | readRights | writeRights
	case (flags & wasi.O_WRONLY) != 0:
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

	if (flags & wasi.O_APPEND) != 0 {
		fdflags |= F_APPEND
	}
	if (flags & wasi.O_CREATE) != 0 {
		oflags |= O_CREAT
	}
	if (flags & wasi.O_EXCL) != 0 {
		oflags |= O_EXCL
	}
	if (flags & wasi.O_SYNC) != 0 {
		fdflags |= F_SYNC
	}
	if (flags & wasi.O_TRUNC) != 0 {
		oflags |= O_TRUNC
	}

	fsRightsInheriting = ^Rights(0)
	dirflags = makeLookupflags(flags)
	return
}

func makeLookupflags(flags int) (dirflags Lookupflags) {
	if (flags & wasi.O_NOFOLLOW) == 0 {
		dirflags |= SymlinkFollow
	}
	return
}

func makeDefaultFlags(dirflags Lookupflags) (flags int) {
	if (dirflags & SymlinkFollow) == 0 {
		dirflags |= wasi.O_NOFOLLOW
	}
	return
}

func makeFilestat(info fs.FileInfo) Filestat {
	return Filestat{
		Dev:      Device(0), // TODO?
		Ino:      Inode(0),  // TODO?
		Filetype: makeFiletype(info.Mode()),
		Nlink:    Linkcount(0), // TODO?
		Size:     Filesize(info.Size()),
		Atim:     Timestamp(0), // TODO?
		Mtim:     Timestamp(info.ModTime().UnixNano()),
		Ctim:     Timestamp(0), // TODO?
	}
}

func makeFileMode(typ Filetype) fs.FileMode {
	switch typ {
	case BlockDevice:
		return fs.ModeDevice
	case CharacterDevice:
		return fs.ModeCharDevice
	case Directory:
		return fs.ModeDir
	case RegularFile:
		return 0
	case SocketDgram, SocketStream:
		return fs.ModeSocket
	case SymbolicLink:
		return fs.ModeSymlink
	default: // Unknown
		return fs.ModeIrregular
	}
}

func makeFiletype(mode fs.FileMode) Filetype {
	switch mode & fs.ModeType {
	case fs.ModeDir:
		return Directory
	case fs.ModeSymlink:
		return SymbolicLink
	case fs.ModeDevice:
		return BlockDevice
	case fs.ModeSocket:
		return SocketStream
	case fs.ModeCharDevice:
		return CharacterDevice
	case fs.ModeNamedPipe, fs.ModeIrregular:
		return Unknown
	default:
		return RegularFile
	}
}
