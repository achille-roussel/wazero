package wasi_snapshot_preview1

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"math"
	fspath "path"
	"time"

	"github.com/tetratelabs/wazero/wasi"
)

// Context represents the execution context of a WASI program.
type Context struct {
	// The file system mounted in this context. If nil, the context acts
	// as if it had an empty file system.
	FileSystem wasi.FS
	// The value of umask in this context.
	Umask fs.FileMode
	// ---
	files fileTable
}

// Close closes the context, releasing all resources that were held.
//
// Calling this method is useful to close all open files tracked by the context
// that the application may have have left open.
func (ctx *Context) Close() error {
	var lastErr error
	ctx.files.scan(func(fd Fd, f *file) bool {
		if err := f.base.Close(); err != nil {
			lastErr = err
		}
		return true
	})
	ctx.files.reset()
	return lastErr
}

// Register adds f to the context, returning its file descriptor number.
//
// In the current implementation of the WASI context, files descriptor numbers
// are allocated incrementally, allowing the use of this method to mount
// standard input and outputs by registering those files before any other
// files were open in the context.
func (ctx *Context) Register(f wasi.File, fsRightsBase, fsRightsInheriting Rights) Fd {
	return ctx.files.insert(file{
		base:               f,
		fsRightsBase:       fsRightsBase,
		fsRightsInheriting: fsRightsInheriting,
	})
}

// Lookup returns the file currently associated with the given file descriptor,
// or nil if the Context does not have an entry for it.
func (ctx *Context) Lookup(fd Fd) wasi.File {
	if f := ctx.files.lookup(fd); f != nil {
		return f.base
	}
	return nil
}

// NumFiles returns the number of files currently opened in this context.
//
// After calling Close on a Context, the method will return zero.
func (ctx *Context) NumFiles() int { return ctx.files.len() }

// FdClose is the implementation of the "fd_close"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_close
func (ctx *Context) FdClose(fd Fd) Errno {
	f := ctx.files.lookup(fd)
	if f == nil {
		return EBADF
	}
	if err := f.base.Close(); err != nil {
		return makeErrno(err)
	}
	ctx.files.delete(fd)
	return ESUCCESS
}

// FdSeek is the implementation of the "fd_seek"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_seek
func (ctx *Context) FdSeek(fd Fd, offset Filedelta, whence Whence) (Filesize, Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return 0, EBADF
	}

	var rights Rights
	if offset == 0 && whence == Cur {
		rights = FD_TELL
	} else {
		rights = FD_SEEK
	}
	if !f.fsRightsBase.Has(rights) {
		return 0, EPERM
	}

	i := int64(offset)
	w := int(0)
	switch whence {
	case Set:
		w = io.SeekStart
	case Cur:
		w = io.SeekCurrent
	case End:
		w = io.SeekEnd
	default:
		return 0, EINVAL
	}
	seek, err := f.base.Seek(i, w)
	return Filesize(seek), makeErrno(err)
}

// FdTell is the implementation of the "fd_tell"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_tell
func (ctx *Context) FdTell(fd Fd) (Filesize, Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return 0, EBADF
	}
	if !f.fsRightsBase.Has(FD_TELL) {
		return 0, EPERM
	}
	tell, err := f.base.Seek(0, io.SeekCurrent)
	return Filesize(tell), makeErrno(err)
}

// FdFdstatGet is the implementation of the "fd_fdstat_get"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_filestat_get
func (ctx *Context) FdFdstatGet(fd Fd) (Fdstat, Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return Fdstat{}, EBADF
	}
	// There is no FD_FDSTAT_GET right, this operation is always allowed.
	s, err := f.base.Stat()
	if err != nil {
		return Fdstat{}, makeErrno(err)
	}
	fdstat := Fdstat{
		FsFiletype:         makeFiletype(s.Mode()),
		FsFlags:            0, // TODO
		FsRightsBase:       f.fsRightsBase,
		FsRightsInheriting: f.fsRightsInheriting,
	}
	return fdstat, ESUCCESS
}

// FdFilestatGet is the implementation of the "fd_filestat_get"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_filestat_get
func (ctx *Context) FdFilestatGet(fd Fd) (Filestat, Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return Filestat{}, EBADF
	}
	if !f.fsRightsBase.Has(FD_FILESTAT_GET) {
		return Filestat{}, EPERM
	}
	s, err := f.base.Stat()
	if err != nil {
		return Filestat{}, makeErrno(err)
	}
	return makeFilestat(s), ESUCCESS
}

// FdFilestatSetSize is the implementation of the "fd_filestat_set_size"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_filestat_set_size
func (ctx *Context) FdFilestatSetSize(fd Fd, size Filesize) Errno {
	f := ctx.files.lookup(fd)
	if f == nil {
		return EBADF
	}
	if !f.fsRightsBase.Has(FD_FILESTAT_SET_SIZE) {
		return EPERM
	}
	return makeErrno(f.base.Truncate(int64(size)))
}

// FdFilestatSetTimes is the implementation of the "fd_filestat_set_times"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_filestat_set_times
func (ctx *Context) FdFilestatSetTimes(fd Fd, atim, mtim Timestamp, flags Fstflags) Errno {
	f := ctx.files.lookup(fd)
	if f == nil {
		return EBADF
	}
	if !f.fsRightsBase.Has(FD_FILESTAT_SET_TIMES) {
		return EPERM
	}
	return makeErrno(f.base.Chtimes(makeFileTimes(atim, mtim, flags)))
}

// FdRead is the implementation of the "fd_read"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_read
func (ctx *Context) FdRead(fd Fd, iovs [][]byte) (Size, Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return 0, EBADF
	}
	if !f.fsRightsBase.Has(FD_READ) {
		return 0, EPERM
	}
	size := Size(0)
	for _, buf := range iovs {
		n, err := f.base.Read(buf)
		size += Size(n)
		if err != nil {
			return size, makeErrno(err)
		}
	}
	return size, ESUCCESS
}

// FdPread is the implementation of the "fd_pread"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_pread
func (ctx *Context) FdPread(fd Fd, iovs [][]byte, offset Filesize) (Size, Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return 0, EBADF
	}
	if !f.fsRightsBase.Has(FD_READ | FD_SEEK) {
		return 0, EPERM
	}
	size := Size(0)
	for _, buf := range iovs {
		n, err := f.base.ReadAt(buf, int64(offset))
		offset += Filesize(n)
		size += Size(n)
		if err != nil {
			return size, makeErrno(err)
		}
	}
	return size, ESUCCESS
}

// FdReaddir is the implementation of the "fd_readdir"
//
// https://github.com/WebAssexombly/WASI/blob/main/phases/snapshot/docs.md#fd_readdir
func (ctx *Context) FdReaddir(fd Fd, buf []byte, dircookie Dircookie) (Size, Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return 0, EBADF
	}
	if !f.fsRightsBase.Has(FD_READDIR) {
		return 0, EPERM
	}
	if len(buf) < 24 {
		return 0, EINVAL
	}
	if dircookie < f.dircookie {
		return 0, EINVAL
	}
	// We control the value of the cookie, and it should never be negative.
	// However, we coerce it to signed to ensure the caller doesn't manipulate
	// it in such a way that becomes negative.
	if dircookie > math.MaxInt64 {
		return 0, EINVAL
	}

	const readDirChunkSize = 10
	for f.dircookie < dircookie {
		d := dircookie - f.dircookie
		n := Dircookie(len(f.direntries))
		if d > n {
			d = n
		}
		f.dircookie += d
		f.direntries = f.direntries[d:]
		if len(f.direntries) == 0 {
			var err error
			f.direntries, err = f.base.ReadDir(readDirChunkSize)
			if len(f.direntries) == 0 && err != nil {
				return 0, makeErrno(err)
			}
		}
	}

	size := Size(0)
	for size < Size(len(buf)) {
		if len(f.direntries) == 0 {
			var err error
			f.direntries, err = f.base.ReadDir(readDirChunkSize)
			if len(f.direntries) == 0 && err != nil {
				return size, makeErrno(err)
			}
		}

		dirent := f.direntries[0]
		name := dirent.Name()
		mode := dirent.Type()

		d := Dirent{
			Next:    f.dircookie + 1,
			Ino:     0, // TODO?
			Namelen: Dirnamlen(len(name)),
			Type:    makeFiletype(mode),
		}

		r := Size(len(buf)) - size
		n := d.Size()
		b := d.Marshal()
		size += Size(copy(buf[size:], b[:]))
		size += Size(copy(buf[size:], name))

		if n <= r {
			f.dircookie++
			f.direntries = f.direntries[1:]
		}
	}
	return size, ESUCCESS
}

// FdWrite is the implementation of the "fd_write"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_write
func (ctx *Context) FdWrite(fd Fd, iovs [][]byte) (Size, Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return 0, EBADF
	}
	if !f.fsRightsBase.Has(FD_WRITE) {
		return 0, EPERM
	}
	size := Size(0)
	for _, buf := range iovs {
		n, err := f.base.Write(buf)
		size += Size(n)
		if err != nil {
			return size, makeErrno(err)
		}
	}
	return size, ESUCCESS
}

// FdPwrite is the implementation of the "fd_pwrite"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_pwrite
func (ctx *Context) FdPwrite(fd Fd, iovs [][]byte, offset Filesize) (Size, Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return 0, EBADF
	}
	if !f.fsRightsBase.Has(FD_WRITE | FD_SEEK) {
		return 0, EPERM
	}
	size := Size(0)
	for _, buf := range iovs {
		n, err := f.base.WriteAt(buf, int64(offset))
		offset += Filesize(n)
		size += Size(n)
		if err != nil {
			return size, makeErrno(err)
		}
	}
	return size, ESUCCESS
}

// PathOpen is the implementation of the "path_open"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#path_open
func (ctx *Context) PathOpen(fd Fd, dirflags Lookupflags, path string, oflags Oflags, fsRightsBase, fsRightsInheriting Rights, fdflags Fdflags) (Fd, Errno) {
	var base wasi.File
	var err error

	if truncate := (oflags & O_TRUNC) != 0; truncate && !fsRightsBase.Has(PATH_FILESTAT_SET_SIZE) {
		return None, EPERM
	}

	flags, perm := makeOpenFileFlags(dirflags, oflags, fsRightsBase, fsRightsInheriting, fdflags)
	perm &= ^ctx.umask()

	if fd == None || fspath.IsAbs(path) {
		if ctx.FileSystem == nil {
			return None, ENOENT
		}
		base, err = ctx.FileSystem.OpenFile(toRel(path), flags, perm)
	} else {
		f := ctx.files.lookup(fd)
		if f == nil {
			return None, EBADF
		}
		if !f.fsRightsBase.Has(PATH_OPEN) {
			return None, EPERM
		}
		fsRightsBase &= f.fsRightsInheriting
		fsRightsInheriting &= f.fsRightsInheriting
		base, err = f.base.OpenFile(path, flags, perm)
	}
	if err != nil {
		return None, makeErrno(err)
	}

	return ctx.Register(base, fsRightsBase, fsRightsInheriting), ESUCCESS
}

// PathCreateDirectory is the implementation of the "path_create_directory"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#path_create_directory
func (ctx *Context) PathCreateDirectory(fd Fd, path string) Errno {
	var perm = fs.ModePerm & ^ctx.umask()
	var err error

	if fd == None || fspath.IsAbs(path) {
		if ctx.FileSystem == nil {
			return ENOENT
		}
		err = ctx.FileSystem.MakeDir(toRel(path), perm)
	} else {
		f := ctx.files.lookup(fd)
		if f == nil {
			return EBADF
		}
		if !f.fsRightsBase.Has(PATH_CREATE_DIRECTORY) {
			return EPERM
		}
		err = f.base.MakeDir(path, perm)
	}
	return makeErrno(err)
}

// PathFilestatGet is the implementation of the "path_filestat_get"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#path_filestat_get
func (ctx *Context) PathFilestatGet(fd Fd, flags Lookupflags, path string) (Filestat, Errno) {
	var info fs.FileInfo
	var err error

	if fd == None || fspath.IsAbs(path) {
		if ctx.FileSystem == nil {
			return Filestat{}, ENOENT
		}
		info, err = ctx.FileSystem.StatFile(toRel(path), makeDefaultFlags(flags))
	} else {
		f := ctx.files.lookup(fd)
		if f == nil {
			return Filestat{}, EBADF
		}
		if !f.fsRightsBase.Has(PATH_FILESTAT_GET) {
			return Filestat{}, EPERM
		}
		info, err = f.base.StatFile(path, makeDefaultFlags(flags))
	}

	if err != nil {
		return Filestat{}, makeErrno(err)
	}
	return makeFilestat(info), ESUCCESS
}

// PathFilestatSetTimes is the implementation of the "path_filestat_set_times"
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#path_filestat_set_times
func (ctx *Context) PathFilestatSetTimes(fd Fd, flags Lookupflags, path string, atim, mtim Timestamp, fstflags Fstflags) Errno {
	var a, m = makeFileTimes(atim, mtim, fstflags)
	var err error

	if fd == None || fspath.IsAbs(path) {
		if ctx.FileSystem == nil {
			return ENOENT
		}
		err = ctx.FileSystem.Chtimes(toRel(path), makeDefaultFlags(flags), a, m)
	} else {
		f := ctx.files.lookup(fd)
		if f == nil {
			return EBADF
		}
		if !f.fsRightsBase.Has(PATH_FILESTAT_SET_TIMES) {
			return EPERM
		}
		err = f.base.ChtimesFile(path, makeDefaultFlags(flags), a, m)
	}

	return makeErrno(err)
}

// FS returns a file system backed by the context that it is called on.
func (ctx *Context) FS() wasi.FS { return contextFS{ctx} }

func (ctx *Context) umask() fs.FileMode { return ctx.Umask & fs.ModePerm }

func toRel(path string) string {
	n := 0
	for n < len(path) && path[n] == '/' {
		n++
	}
	return path[n:]
}

type contextFS struct{ ctx *Context }

func (fsys contextFS) Open(name string) (fs.File, error) {
	return fsys.OpenFile(name, 0, 0)
}

func (fsys contextFS) OpenFile(path string, flags int, perm fs.FileMode) (wasi.File, error) {
	if !fs.ValidPath(path) {
		return nil, fs.ErrInvalid
	}
	dirflags, oflags, fsRightsBase, fsRightsInheriting, fdflags := makePathOpenFlags(flags, perm)
	fd, errno := fsys.ctx.PathOpen(None, dirflags, path, oflags, fsRightsBase, fsRightsInheriting, fdflags)
	if err := makeError(errno); err != nil {
		return nil, err
	}
	return &contextFile{ctx: fsys.ctx, fd: fd, name: fspath.Base(path)}, nil
}

func (fsys contextFS) Stat(path string) (fs.FileInfo, error) {
	return fsys.StatFile(path, 0)
}

func (fsys contextFS) StatFile(path string, flags int) (fs.FileInfo, error) {
	if !fs.ValidPath(path) {
		return nil, fs.ErrInvalid
	}
	stat, errno := fsys.ctx.PathFilestatGet(None, makeLookupflags(flags), path)
	if err := makeError(errno); err != nil {
		return nil, err
	}
	return &contextFileInfo{name: fspath.Base(path), stat: stat}, nil
}

func (fsys contextFS) MakeDir(path string, perm fs.FileMode) error {
	if !fs.ValidPath(path) {
		return fs.ErrInvalid
	}
	subctx := *fsys.ctx
	subctx.Umask |= ^perm
	return makeError(subctx.PathCreateDirectory(None, path))
}

func (fsys contextFS) Chtimes(path string, flags int, atim, mtim time.Time) error {
	if !fs.ValidPath(path) {
		return fs.ErrInvalid
	}
	a, m, f := makeTimestampsAndFstflags(atim, mtim)
	return makeError(fsys.ctx.PathFilestatSetTimes(None, makeLookupflags(flags), path, a, m, f))
}

type contextFile struct {
	ctx  *Context
	fd   Fd
	name string
	// iterator state used when calling ReadDir
	dircookie Dircookie
	dirbuffer []byte
}

func (f *contextFile) Name() string {
	return f.name
}

func (f *contextFile) Close() error {
	if f.fd == None {
		return fs.ErrClosed
	}
	fd := f.fd
	f.fd = None
	return makeError(f.ctx.FdClose(fd))
}

func (f *contextFile) OpenFile(path string, flags int, perm fs.FileMode) (wasi.File, error) {
	if f.fd == None {
		return nil, fs.ErrClosed
	}
	dirflags, oflags, fsRightsBase, fsRightsInheriting, fdflags := makePathOpenFlags(flags, perm)
	fd, errno := f.ctx.PathOpen(f.fd, dirflags, path, oflags, fsRightsBase, fsRightsInheriting, fdflags)
	if err := makeError(errno); err != nil {
		return nil, err
	}
	return &contextFile{ctx: f.ctx, fd: fd, name: fspath.Base(path)}, nil
}

func (f *contextFile) Read(b []byte) (int, error) {
	if f.fd == None {
		return 0, fs.ErrClosed
	}
	size, errno := f.ctx.FdRead(f.fd, [][]byte{b})
	if size == 0 && errno == ESUCCESS && len(b) != 0 {
		return 0, io.EOF
	}
	return int(size), makeError(errno)
}

func (f *contextFile) ReadAt(b []byte, off int64) (int, error) {
	if f.fd == None {
		return 0, fs.ErrClosed
	}
	size, errno := f.ctx.FdPread(f.fd, [][]byte{b}, Filesize(off))
	if size < Size(len(b)) && errno == ESUCCESS {
		return int(size), io.EOF
	}
	return int(size), makeError(errno)
}

func (f *contextFile) MakeDir(path string, perm fs.FileMode) error {
	if f.fd == None {
		return fs.ErrClosed
	}
	subctx := *f.ctx
	subctx.Umask |= ^perm
	return makeError(subctx.PathCreateDirectory(f.fd, path))
}

func (f *contextFile) ReadDir(n int) (ret []fs.DirEntry, err error) {
	if f.fd == None {
		return nil, fs.ErrClosed
	}
	if n < 0 {
		n = 0
	}
	ent := make([]*contextDirEntry, 0, n)
	buf := make([]byte, MaxPathLen+24)

	for n == 0 || len(ent) < n {
		b := buf[:]
		size := Size(0)
		errno := Errno(0)

		if len(f.dirbuffer) == 0 {
			size, errno = f.ctx.FdReaddir(f.fd, b, f.dircookie)
			b = b[:size]
		} else {
			b, f.dirbuffer = f.dirbuffer, f.dirbuffer[:0]
			size = Size(len(b))
		}

		for len(b) >= 24 && (n == 0 || len(ent) < n) {
			d := Dirent{}
			d.Unmarshal(*(*[24]byte)(b))
			b = b[24:]

			if d.Namelen > Dirnamlen(len(b)) {
				b = b[len(b):]
			} else {
				ent = append(ent, &contextDirEntry{
					file: f,
					name: string(b[:d.Namelen]),
					mode: makeFileMode(d.Type),
				})
				b = b[d.Namelen:]
			}

			f.dircookie = d.Next
		}

		if len(b) >= 24 {
			f.dirbuffer = append(f.dirbuffer[:0], b...)
		}

		if errno != ESUCCESS {
			err = makeError(errno)
			break
		}

		if size == 0 {
			err = io.EOF
			break
		}
	}

	if n == 0 && err == io.EOF {
		err = nil
	}

	ret = make([]fs.DirEntry, len(ent))
	for i, e := range ent {
		ret[i] = e
	}
	return ret, err
}

func (f *contextFile) Write(b []byte) (int, error) {
	if f.fd == None {
		return 0, fs.ErrClosed
	}
	size, errno := f.ctx.FdWrite(f.fd, [][]byte{b})
	return int(size), makeError(errno)
}

func (f *contextFile) WriteAt(b []byte, off int64) (int, error) {
	if f.fd == None {
		return 0, fs.ErrClosed
	}
	size, errno := f.ctx.FdPwrite(f.fd, [][]byte{b}, Filesize(off))
	return int(size), makeError(errno)
}

func (f *contextFile) Seek(offset int64, whence int) (int64, error) {
	if f.fd == None {
		return 0, fs.ErrClosed
	}
	size, errno := f.ctx.FdSeek(f.fd, Filedelta(offset), Whence(whence))
	return int64(size), makeError(errno)
}

func (f *contextFile) Stat() (fs.FileInfo, error) {
	if f.fd == None {
		return nil, fs.ErrClosed
	}
	stat, errno := f.ctx.FdFilestatGet(f.fd)
	if err := makeError(errno); err != nil {
		return nil, err
	}
	return &contextFileInfo{name: f.name, stat: stat}, nil
}

func (f *contextFile) StatFile(path string, flags int) (fs.FileInfo, error) {
	if f.fd == None {
		return nil, fs.ErrClosed
	}
	stat, errno := f.ctx.PathFilestatGet(f.fd, 0, path)
	if err := makeError(errno); err != nil {
		return nil, err
	}
	return &contextFileInfo{name: fspath.Base(path), stat: stat}, nil
}

func (f *contextFile) Chtimes(atim, mtim time.Time) error {
	if f.fd == None {
		return fs.ErrClosed
	}
	a, m, fst := makeTimestampsAndFstflags(atim, mtim)
	return makeError(f.ctx.FdFilestatSetTimes(f.fd, a, m, fst))
}

func (f *contextFile) ChtimesFile(path string, flags int, atim, mtim time.Time) error {
	if f.fd == None {
		return fs.ErrClosed
	}
	a, m, fst := makeTimestampsAndFstflags(atim, mtim)
	return makeError(f.ctx.PathFilestatSetTimes(f.fd, makeLookupflags(flags), path, a, m, fst))
}

func (f *contextFile) Truncate(size int64) error {
	if f.fd == None {
		return fs.ErrClosed
	}
	return makeError(f.ctx.FdFilestatSetSize(f.fd, Filesize(size)))
}

type contextFileInfo struct {
	name string
	stat Filestat
}

func (f *contextFileInfo) Name() string       { return f.name }
func (f *contextFileInfo) Size() int64        { return int64(f.stat.Size) }
func (f *contextFileInfo) Mode() fs.FileMode  { return f.stat.FileMode() }
func (f *contextFileInfo) ModTime() time.Time { return f.stat.Mtim.Time() }
func (f *contextFileInfo) IsDir() bool        { return f.Mode().IsDir() }
func (f *contextFileInfo) Sys() interface{}   { return &f.stat }
func (f *contextFileInfo) String() string {
	return fmt.Sprintf("%s %s %d", f.Name(), f.Mode(), f.Size())
}

type contextDirEntry struct {
	file *contextFile
	name string
	mode fs.FileMode
}

func (d *contextDirEntry) Name() string               { return d.name }
func (d *contextDirEntry) Type() fs.FileMode          { return d.mode }
func (d *contextDirEntry) IsDir() bool                { return (d.mode & fs.ModeDir) != 0 }
func (d *contextDirEntry) Info() (fs.FileInfo, error) { return d.file.StatFile(d.name, 0) }
func (d *contextDirEntry) String() string             { return fmt.Sprintf("%s %s", d.name, d.mode) }

// ContextOf returns the WASI context embedded into the given Go context.
func ContextOf(ctx context.Context) *Context {
	wasiCtx, _ := ctx.Value(contextKey{}).(*Context)
	return wasiCtx
}

// WithContext returns a Go context wrapping ctx and embedding the given WASI
// context.
//
// This function is useful to pass a WASI context through abstraction layers
// propagating Go contexts.
func WithContext(ctx context.Context, wasiCtx *Context) context.Context {
	return context.WithValue(ctx, contextKey{}, wasiCtx)
}

type contextKey struct{}
