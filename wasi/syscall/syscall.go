// Package syscall defines the types of WASI in Go.
//
// Applications should not take a direct dependency on this package an instead
// use the top-level wasi package only.
package syscall

import (
	"encoding/binary"

	"github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
)

type Errno uint32

func (e Errno) Error() string {
	return wasi_snapshot_preview1.ErrnoName(uint32(e))
}

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
