// Package wasi_snapshot_preview1 is an internal helper to remove package
// cycles re-using errno
package wasi_snapshot_preview1

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"syscall"
	"time"

	"github.com/tetratelabs/wazero/wasi"
)

const ModuleName = "wasi_snapshot_preview1"

type Errno uint32 // neither uint16 nor an alias for parity with wasm.ValueType

const (
	ESUCCESS Errno = iota
	E2BIG
	EACCES
	EADDRINUSE
	EADDRNOTAVAIL
	EAFNOSUPPORT
	EAGAIN
	EALREADY
	EBADF
	EBADMSG
	EBUSY
	ECANCELED
	ECHILD
	ECONNABORTED
	ECONNREFUSED
	ECONNRESET
	EDEADLK
	EDESTADDRREQ
	EDOM
	EDQUOT
	EEXIST
	EFAULT
	EFBIG
	EHOSTUNREACH
	EIDRM
	EILSEQ
	EINPROGRESS
	EINTR
	EINVAL
	EIO
	EISCONN
	EISDIR
	ELOOP
	EMFILE
	EMLINK
	EMSGSIZE
	EMULTIHOP
	ENAMETOOLONG
	ENETDOWN
	ENETRESET
	ENETUNREACH
	ENFILE
	ENOBUFS
	ENODEV
	ENOENT
	ENOEXEC
	ENOLCK
	ENOLINK
	ENOMEM
	ENOMSG
	ENOPROTOOPT
	ENOSPC
	ENOSYS
	ENOTCONN
	ENOTDIR
	ENOTEMPTY
	ENOTRECOVERABLE
	ENOTSOCK
	ENOTSUP
	ENOTTY
	ENXIO
	EOVERFLOW
	EOWNERDEAD
	EPERM
	EPIPE
	EPROTO
	EPROTONOSUPPORT
	EPROTOTYPE
	ERANGE
	EROFS
	ESPIPE
	ESRCH
	ESTALE
	ETIMEDOUT
	ETXTBSY
	EXDEV
	ENOTCAPABLE
)

// Name returns the POSIX error code name, except ErrnoSuccess, which is not an
// error. e.g. Errno2big -> "E2BIG"
func (e Errno) Name() string {
	if int(e) < len(errnoToString) {
		return errnoToString[e]
	}
	return fmt.Sprintf("errno(%d)", int(e))
}

func (e Errno) Error() string { return e.Name() }

var errnoToString = [...]string{
	"ESUCCESS",
	"E2BIG",
	"EACCES",
	"EADDRINUSE",
	"EADDRNOTAVAIL",
	"EAFNOSUPPORT",
	"EAGAIN",
	"EALREADY",
	"EBADF",
	"EBADMSG",
	"EBUSY",
	"ECANCELED",
	"ECHILD",
	"ECONNABORTED",
	"ECONNREFUSED",
	"ECONNRESET",
	"EDEADLK",
	"EDESTADDRREQ",
	"EDOM",
	"EDQUOT",
	"EEXIST",
	"EFAULT",
	"EFBIG",
	"EHOSTUNREACH",
	"EIDRM",
	"EILSEQ",
	"EINPROGRESS",
	"EINTR",
	"EINVAL",
	"EIO",
	"EISCONN",
	"EISDIR",
	"ELOOP",
	"EMFILE",
	"EMLINK",
	"EMSGSIZE",
	"EMULTIHOP",
	"ENAMETOOLONG",
	"ENETDOWN",
	"ENETRESET",
	"ENETUNREACH",
	"ENFILE",
	"ENOBUFS",
	"ENODEV",
	"ENOENT",
	"ENOEXEC",
	"ENOLCK",
	"ENOLINK",
	"ENOMEM",
	"ENOMSG",
	"ENOPROTOOPT",
	"ENOSPC",
	"ENOSYS",
	"ENOTCONN",
	"ENOTDIR",
	"ENOTEMPTY",
	"ENOTRECOVERABLE",
	"ENOTSOCK",
	"ENOTSUP",
	"ENOTTY",
	"ENXIO",
	"EOVERFLOW",
	"EOWNERDEAD",
	"EPERM",
	"EPIPE",
	"EPROTO",
	"EPROTONOSUPPORT",
	"EPROTOTYPE",
	"ERANGE",
	"EROFS",
	"ESPIPE",
	"ESRCH",
	"ESTALE",
	"ETIMEDOUT",
	"ETXTBSY",
	"EXDEV",
	"ENOTCAPABLE",
}

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
	case errors.Is(err, wasi.ErrReadOnly):
		return EROFS
	}
	var errno Errno
	if errors.As(err, &errno) {
		return errno
	}
	return EIO
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
	case EROFS:
		return wasi.ErrReadOnly
	default:
		return errno
	}
}

// Oflags are open flags used by path_open.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-oflags-flagsu16
type Oflags uint16

const (
	// O_CREAT creates a file if it does not exist.
	O_CREAT Oflags = 1 << iota //nolint
	// O_DIRECTORY fails if not a directory.
	O_DIRECTORY
	// O_EXCL fails if file already exists.
	O_EXCL //nolint
	// O_TRUNC truncates the file to size 0.
	O_TRUNC //nolint
)

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
	if (fdflags & FD_APPEND) != 0 {
		flags |= wasi.O_APPEND
	}
	if (fdflags & FD_DSYNC) != 0 {
		flags |= wasi.O_DSYNC
	}
	if (fdflags & FD_RSYNC) != 0 {
		flags |= wasi.O_RSYNC
	}
	if (fdflags & FD_SYNC) != 0 {
		flags |= wasi.O_SYNC
	}
	switch {
	case fsRightsBase.Has(RIGHT_FD_READ | RIGHT_FD_WRITE):
		flags |= wasi.O_RDWR
	case fsRightsBase.Has(RIGHT_FD_WRITE):
		flags |= wasi.O_WRONLY
	default:
		flags |= wasi.O_RDONLY
	}
	perm = 0644
	return
}

func makePathOpenFlags(flags int, perm fs.FileMode) (dirflags Lookupflags, oflags Oflags, fsRightsBase, fsRightsInheriting Rights, fdflags Fdflags) {
	switch {
	case (flags & wasi.O_RDWR) != 0:
		fsRightsBase = RW
	case (flags & wasi.O_WRONLY) != 0:
		fsRightsBase = W
	default:
		fsRightsBase = R
	}

	if perm != 0 {
		if (perm & 0400) == 0 {
			fsRightsBase &= ^R
		}
		if (perm & 0200) == 0 {
			fsRightsBase &= ^W
		}
	}

	if (flags & wasi.O_APPEND) != 0 {
		fdflags |= FD_APPEND
	}
	if (flags & wasi.O_CREATE) != 0 {
		oflags |= O_CREAT
	}
	if (flags & wasi.O_EXCL) != 0 {
		oflags |= O_EXCL
	}
	if (flags & wasi.O_SYNC) != 0 {
		fdflags |= FD_SYNC
	}
	if (flags & wasi.O_TRUNC) != 0 {
		oflags |= O_TRUNC
	}

	fsRightsInheriting = ^Rights(0)
	dirflags = makeLookupflags(flags)
	return
}

type Fd uint32

const (
	None Fd = ^Fd(0)
)

// Fdflags are file descriptor flags.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fdflags
type Fdflags uint16

const (
	FD_APPEND Fdflags = 1 << iota //nolint
	FD_DSYNC
	FD_NONBLOCK
	FD_RSYNC
	FD_SYNC
)

type Fdstat struct {
	FsFiletype         Filetype
	FsFlags            Fdflags
	FsRightsBase       Rights
	FsRightsInheriting Rights
}

func (s *Fdstat) Marshal() (b [24]byte) {
	binary.LittleEndian.PutUint16(b[0:], uint16(s.FsFiletype))
	binary.LittleEndian.PutUint16(b[2:], uint16(s.FsFlags))
	binary.LittleEndian.PutUint64(b[8:], uint64(s.FsRightsBase))
	binary.LittleEndian.PutUint64(b[16:], uint64(s.FsRightsInheriting))
	return b
}

func (s *Fdstat) Unmarshal(b [24]byte) {
	s.FsFiletype = Filetype(binary.LittleEndian.Uint16(b[0:]))
	s.FsFlags = Fdflags(binary.LittleEndian.Uint16(b[2:]))
	s.FsRightsBase = Rights(binary.LittleEndian.Uint64(b[8:]))
	s.FsRightsInheriting = Rights(binary.LittleEndian.Uint64(b[16:]))
}

// Lookupflags define the behavior of path lookups.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#lookupflags
type Lookupflags uint32

const (
	// LOOKUP_SYMLINK_FOLLOW expands a path if it resolves into a symbolic
	// link.
	LOOKUP_SYMLINK_FOLLOW Lookupflags = 1 << iota //nolint
)

func makeLookupflags(flags int) (dirflags Lookupflags) {
	if (flags & wasi.O_NOFOLLOW) == 0 {
		dirflags |= LOOKUP_SYMLINK_FOLLOW
	}
	return
}

func makeDefaultFlags(dirflags Lookupflags) (flags int) {
	if (dirflags & LOOKUP_SYMLINK_FOLLOW) == 0 {
		dirflags |= wasi.O_NOFOLLOW
	}
	return
}

type Fstflags uint16

const (
	FST_ATIM Fstflags = 1 << iota //nolint
	FST_ATIM_NOW
	FST_MTIM
	FST_MTIM_NOW
)

func makeFileTimes(atim, mtim Timestamp, flags Fstflags) (a, m time.Time) {
	var now time.Time

	if (flags & (FST_ATIM_NOW | FST_MTIM_NOW)) != 0 {
		now = time.Now()
	}

	if (flags & FST_ATIM) != 0 {
		a = atim.Time()
	}
	if (flags & FST_ATIM_NOW) != 0 {
		a = now
	}
	if (flags & FST_MTIM) != 0 {
		m = mtim.Time()
	}
	if (flags & FST_MTIM_NOW) != 0 {
		m = now
	}
	return a, m
}

func makeTimestampsAndFstflags(atim, mtim time.Time) (a, m Timestamp, f Fstflags) {
	if !atim.IsZero() {
		f |= FST_ATIM
	}
	if !mtim.IsZero() {
		f |= FST_MTIM
	}
	a = makeTimestamp(atim)
	m = makeTimestamp(mtim)
	return a, m, f
}

// Rights define the list of permissions given to file descriptors.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-rights-flagsu64
type Rights uint64

func (r Rights) Has(rights Rights) bool { return (r & rights) == rights }

const (
	// RIGHT_FD_DATASYNC is the right to invoke fd_datasync. If RIGHT_PATH_OPEN
	// is set, includes the right to invoke path_open with FD_DSYNC.
	RIGHT_FD_DATASYNC Rights = 1 << iota //nolint

	// RIGHT_FD_READ is he right to invoke fd_read and sock_recv. If
	// RIGHT_FD_SYNC is set, includes the right to invoke fd_pread.
	RIGHT_FD_READ

	// RIGHT_FD_SEEK is the right to invoke fd_seek. This flag implies
	// RIGHT_FD_TELL.
	RIGHT_FD_SEEK

	// RIGHT_FDSTAT_SET_FLAGS is the right to invoke fd_fdstat_set_flags.
	RIGHT_FDSTAT_SET_FLAGS

	// RIGHT_FD_SYNC The right to invoke fd_sync. If path_open is set, includes
	// the right to invoke path_open with FD_RSYNC and FD_DSYNC.
	RIGHT_FD_SYNC

	// RIGHT_FD_TELL is the right to invoke fd_seek in such a way that the file
	// offset remains unaltered (i.e., whence::cur with offset zero), or to
	// invoke fd_tell.
	RIGHT_FD_TELL

	// RIGHT_FD_WRITE is the right to invoke fd_write and sock_send. If
	// RIGHT_FD_SEEK is set, includes the right to invoke fd_pwrite.
	RIGHT_FD_WRITE

	// RIGHT_FD_ADVISE is the right to invoke fd_advise.
	RIGHT_FD_ADVISE

	// RIGHT_FD_ALLOCATE is the right to invoke fd_allocate.
	RIGHT_FD_ALLOCATE

	// RIGHT_PATH_CREATE_DIRECTORY is the right to invoke
	// path_create_directory.
	RIGHT_PATH_CREATE_DIRECTORY

	// RIGHT_PATH_CREATE_FILE when RIGHT_PATH_OPEN is set, the right to invoke
	// path_open with O_CREATE.
	RIGHT_PATH_CREATE_FILE

	// RIGHT_PATH_LINK_SOURCE is the right to invoke path_link with the file
	// descriptor as the source directory.
	RIGHT_PATH_LINK_SOURCE

	// RIGHT_PATH_LINK_TARGET is the right to invoke path_link with the file
	// descriptor as the target directory.
	RIGHT_PATH_LINK_TARGET

	// RIGHT_PATH_OPEN is the right to invoke path_open.
	RIGHT_PATH_OPEN

	// RIGHT_FD_READDIR is the right to invoke fd_readdir.
	RIGHT_FD_READDIR

	// RIGHT_PATH_READLINK is the right to invoke path_readlink.
	RIGHT_PATH_READLINK

	// RIGHT_PATH_RENAME_SOURCE is the right to invoke path_rename with the
	// file descriptor as the source directory.
	RIGHT_PATH_RENAME_SOURCE

	// RIGHT_PATH_RENAME_TARGET is the right to invoke path_rename with the
	// file descriptor as the target directory.
	RIGHT_PATH_RENAME_TARGET

	// RIGHT_PATH_FILESTAT_GET is the right to invoke path_filestat_get.
	RIGHT_PATH_FILESTAT_GET

	// RIGHT_PATH_FILESTAT_SET_SIZE is the right to change a file's size (there
	// is no path_filestat_set_size). If RIGHT_PATH_OPEN is set, includes the
	// right to invoke path_open with O_TRUNC.
	RIGHT_PATH_FILESTAT_SET_SIZE

	// RIGHT_PATH_FILESTAT_SET_TIMES is the right to invoke
	// path_filestat_set_times.
	RIGHT_PATH_FILESTAT_SET_TIMES

	// RIGHT_FD_FILESTAT_GET is the right to invoke fd_filestat_get.
	RIGHT_FD_FILESTAT_GET

	// RIGHT_FD_FILESTAT_SET_SIZE is the right to invoke fd_filestat_set_size.
	RIGHT_FD_FILESTAT_SET_SIZE

	// RIGHT_FD_FILESTAT_SET_TIMES is the right to invoke
	// fd_filestat_set_times.
	RIGHT_FD_FILESTAT_SET_TIMES

	// RIGHT_PATH_SYMLINK is the right to invoke path_symlink.
	RIGHT_PATH_SYMLINK

	// RIGHT_PATH_REMOVE_DIRECTORY is the right to invoke
	// path_remove_directory.
	RIGHT_PATH_REMOVE_DIRECTORY

	// RIGHT_PATH_UNLINK_FILE is the right to invoke path_unlink_file.
	RIGHT_PATH_UNLINK_FILE

	// RIGHT_POLL_FD_READWRITE when RIGHT_FD_READ is set, includes the right to
	// invoke poll_oneoff to subscribe to eventtype::fd_read. If RIGHT_FD_WRITE
	// is set, includes the right to invoke poll_oneoff to subscribe to
	// eventtype::fd_write.
	RIGHT_POLL_FD_READWRITE

	// RIGHT_SOCK_SHUTDOWN is the right to invoke sock_shutdown.
	RIGHT_SOCK_SHUTDOWN
)

const (
	baseRights = RIGHT_FD_SEEK |
		RIGHT_FD_TELL |
		RIGHT_FD_FILESTAT_GET |
		RIGHT_PATH_OPEN |
		RIGHT_PATH_CREATE_DIRECTORY |
		RIGHT_PATH_FILESTAT_GET |
		RIGHT_PATH_FILESTAT_SET_SIZE |
		RIGHT_PATH_FILESTAT_SET_TIMES
	R  = baseRights | RIGHT_FD_READ | RIGHT_FD_READDIR
	W  = baseRights | RIGHT_FD_WRITE | RIGHT_FD_FILESTAT_SET_SIZE | RIGHT_FD_FILESTAT_SET_TIMES
	RW = R | W
)

type Device uint64

type Inode uint64

type Size uint32

type Linkcount uint64

type Filedelta int64

type Filesize uint64

// Filemode is not part of the WASI spec but it is useful to bridge with unix
// file systems.
//
// The only use of this type is within Filestat, where it is packed within the
// alignment of Filetype.
type Filemode uint16

func makeFileMode(typ Filetype) fs.FileMode {
	switch typ {
	case FILETYPE_BLOCK_DEVICE:
		return fs.ModeDevice
	case FILETYPE_CHARACTER_DEVICE:
		return fs.ModeCharDevice
	case FILETYPE_DIRECTORY:
		return fs.ModeDir
	case FILETYPE_REGULAR_FILE:
		return 0
	case FILETYPE_SOCKET_DGRAM, FILETYPE_SOCKET_STREAM:
		return fs.ModeSocket
	case FILETYPE_SYMBOLIC_LINK:
		return fs.ModeSymlink
	default: // Unknown
		return fs.ModeIrregular
	}
}

type Filetype uint8

const (
	FILETYPE_UNKNOWN Filetype = iota
	FILETYPE_BLOCK_DEVICE
	FILETYPE_CHARACTER_DEVICE
	FILETYPE_DIRECTORY
	FILETYPE_REGULAR_FILE
	FILETYPE_SOCKET_DGRAM
	FILETYPE_SOCKET_STREAM
	FILETYPE_SYMBOLIC_LINK
)

// Name returns string name of the file type.
func (t Filetype) Name() string {
	if int(t) < len(filetypeToString) {
		return filetypeToString[t]
	}
	return fmt.Sprintf("filetype(%d)", t)
}

var filetypeToString = [...]string{
	"UNKNOWN",
	"BLOCK_DEVICE",
	"CHARACTER_DEVICE",
	"DIRECTORY",
	"REGULAR_FILE",
	"SOCKET_DGRAM",
	"SOCKET_STREAM",
	"SYMBOLIC_LINK",
}

func makeFiletype(mode fs.FileMode) Filetype {
	switch mode & fs.ModeType {
	case fs.ModeDir:
		return FILETYPE_DIRECTORY
	case fs.ModeSymlink:
		return FILETYPE_SYMBOLIC_LINK
	case fs.ModeDevice:
		return FILETYPE_BLOCK_DEVICE
	case fs.ModeSocket:
		return FILETYPE_SOCKET_STREAM
	case fs.ModeCharDevice:
		return FILETYPE_CHARACTER_DEVICE
	case fs.ModeNamedPipe, fs.ModeIrregular:
		return FILETYPE_UNKNOWN
	default:
		return FILETYPE_REGULAR_FILE
	}
}

type Filestat struct {
	Dev   Device
	Ino   Inode
	Type  Filetype
	Mode  Filemode
	Nlink Linkcount
	Size  Filesize
	Atim  Timestamp
	Mtim  Timestamp
	Ctim  Timestamp
}

func (s *Filestat) FileMode() fs.FileMode {
	return makeFileMode(s.Type) | fs.FileMode(s.Mode)
}

func (s *Filestat) Marshal() (b [64]byte) {
	binary.LittleEndian.PutUint64(b[0:], uint64(s.Dev))
	binary.LittleEndian.PutUint64(b[8:], uint64(s.Ino))
	binary.LittleEndian.PutUint16(b[16:], uint16(s.Type))
	binary.LittleEndian.PutUint16(b[18:], uint16(s.Mode))
	binary.LittleEndian.PutUint64(b[24:], uint64(s.Nlink))
	binary.LittleEndian.PutUint64(b[32:], uint64(s.Size))
	binary.LittleEndian.PutUint64(b[40:], uint64(s.Atim))
	binary.LittleEndian.PutUint64(b[48:], uint64(s.Mtim))
	binary.LittleEndian.PutUint64(b[56:], uint64(s.Ctim))
	return b
}

func (s *Filestat) Unmarshal(b [64]byte) {
	s.Dev = Device(binary.LittleEndian.Uint64(b[0:]))
	s.Ino = Inode(binary.LittleEndian.Uint64(b[8:]))
	s.Type = Filetype(binary.LittleEndian.Uint16(b[16:]))
	s.Mode = Filemode(binary.LittleEndian.Uint16(b[18:]))
	s.Nlink = Linkcount(binary.LittleEndian.Uint64(b[24:]))
	s.Size = Filesize(binary.LittleEndian.Uint64(b[32:]))
	s.Atim = Timestamp(binary.LittleEndian.Uint64(b[40:]))
	s.Mtim = Timestamp(binary.LittleEndian.Uint64(b[48:]))
	s.Ctim = Timestamp(binary.LittleEndian.Uint64(b[56:]))
}

func makeFilestat(info fs.FileInfo) Filestat {
	mode := info.Mode()
	atim := time.Time{}
	mtim := time.Time{}
	ctim := time.Time{}
	dev := uint64(0)
	ino := uint64(0)
	nlink := uint64(0)

	switch s := info.Sys().(type) {
	case *Filestat:
		dev = uint64(s.Dev)
		ino = uint64(s.Ino)
		nlink = uint64(s.Nlink)
		atim = s.Atim.Time()
		mtim = s.Mtim.Time()
		ctim = s.Ctim.Time()
	case *syscall.Stat_t:
		dev = s.Dev
		ino = s.Ino
		nlink = s.Nlink
		atim = time.Unix(s.Atim.Unix())
		mtim = time.Unix(s.Mtim.Unix())
		ctim = time.Unix(s.Ctim.Unix())
	default:
		mtim = info.ModTime()
	}

	return Filestat{
		Dev:   Device(dev),
		Ino:   Inode(ino),
		Type:  makeFiletype(mode),
		Mode:  Filemode(mode & fs.ModePerm),
		Nlink: Linkcount(nlink),
		Size:  Filesize(info.Size()),
		Atim:  makeTimestamp(atim),
		Mtim:  makeTimestamp(mtim),
		Ctim:  makeTimestamp(ctim),
	}
}

type Dircookie uint64

type Dirnamlen uint32

type Dirent struct {
	Next    Dircookie
	Ino     Inode
	Namelen Dirnamlen
	Type    Filetype
	Mode    Filemode
}

func (d *Dirent) Size() Size { return 24 + Size(d.Namelen) }

func (d *Dirent) Marshal() (b [24]byte) {
	binary.LittleEndian.PutUint64(b[0:], uint64(d.Next))
	binary.LittleEndian.PutUint64(b[8:], uint64(d.Ino))
	binary.LittleEndian.PutUint32(b[16:], uint32(d.Namelen))
	binary.LittleEndian.PutUint16(b[20:], uint16(d.Type))
	binary.LittleEndian.PutUint16(b[22:], uint16(d.Mode))
	return b
}

func (d *Dirent) Unmarshal(b [24]byte) {
	d.Next = Dircookie(binary.LittleEndian.Uint64(b[0:]))
	d.Ino = Inode(binary.LittleEndian.Uint64(b[8:]))
	d.Namelen = Dirnamlen(binary.LittleEndian.Uint32(b[16:]))
	d.Type = Filetype(binary.LittleEndian.Uint16(b[20:]))
	d.Mode = Filemode(binary.LittleEndian.Uint16(b[22:]))
}

type Timestamp uint64

func (t Timestamp) Time() time.Time {
	return time.Unix(0, int64(t))
}

func makeTimestamp(t time.Time) Timestamp {
	if t.IsZero() {
		return 0
	}
	return Timestamp(t.UnixNano())
}

type Whence uint8

const (
	SEEK_SET Whence = iota //nolint
	SEEK_CUR
	SEEK_END
)
