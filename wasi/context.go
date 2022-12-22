package wasi

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	fspath "path"
	"strings"
	"time"

	"github.com/tetratelabs/wazero/wasi/syscall"
)

// Context represents the execution context of a WASI program.
type Context struct {
	fsys  FS
	files fileTable
}

// ContextOption is an interface used to represent configuration options when
// creating contexts.
type ContextOption interface {
	ConfigureContext(*Context)
}

type contextOption func(*Context)

func (opt contextOption) ConfigureContext(ctx *Context) { opt(ctx) }

// FileSystem is a configuration option to define the file system mounted into
// a context.
func FileSystem(fsys FS) ContextOption {
	return contextOption(func(ctx *Context) { ctx.fsys = fsys })
}

// NewContext constructs a new Context instance using the options passed as
// arguments.
func NewContext(options ...ContextOption) *Context {
	ctx := new(Context)
	for _, opt := range options {
		opt.ConfigureContext(ctx)
	}
	return ctx
}

// Close closes the context, releasing all resources that were held.
func (ctx *Context) Close() error {
	ctx.files.scan(func(fd syscall.Fd, f *file) bool {
		f.Close()
		return true
	})
	ctx.files.reset()
	return nil
}

// FdClose is the implementation of the "fd_close" syscall.
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_close
func (ctx *Context) FdClose(fd syscall.Fd) syscall.Errno {
	f := ctx.files.lookup(fd)
	if f == nil {
		return syscall.EBADF
	}
	if err := f.Close(); err != nil {
		return makeErrno(err)
	}
	ctx.files.delete(fd)
	return syscall.ESUCCESS
}

// FdSeek is the implementation of the "fd_seek" syscall.
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_seek
func (ctx *Context) FdSeek(fd syscall.Fd, offset syscall.Filedelta, whence syscall.Whence) (syscall.Filesize, syscall.Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return 0, syscall.EBADF
	}
	i := int64(offset)
	w := int(0)
	switch whence {
	case syscall.Set:
		w = io.SeekStart
	case syscall.Cur:
		w = io.SeekCurrent
	case syscall.End:
		w = io.SeekEnd
	default:
		return 0, syscall.EINVAL
	}
	seek, err := f.Seek(i, w)
	return syscall.Filesize(seek), makeErrno(err)
}

// FdTell is the implementation of the "fd_tell" syscall.
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_tell
func (ctx *Context) FdTell(fd syscall.Fd) (syscall.Filesize, syscall.Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return 0, syscall.EBADF
	}
	tell, err := f.Seek(0, io.SeekCurrent)
	return syscall.Filesize(tell), makeErrno(err)
}

// FdFilestatGet is the implementation of the "fd_filestat_get" syscall.
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_filestat_get
func (ctx *Context) FdFilestatGet(fd syscall.Fd) (syscall.Filestat, syscall.Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return syscall.Filestat{}, syscall.EBADF
	}
	s, err := f.Stat()
	if err != nil {
		return syscall.Filestat{}, makeErrno(err)
	}
	return makeFilestat(s), syscall.ESUCCESS
}

// FdPread is the implementation of the "fd_pread" syscall.
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_pread
func (ctx *Context) FdPread(fd syscall.Fd, iovs [][]byte, offset syscall.Filesize) (syscall.Size, syscall.Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return 0, syscall.EBADF
	}
	size := syscall.Size(0)
	for _, buf := range iovs {
		n, err := f.ReadAt(buf, int64(offset))
		offset += syscall.Filesize(n)
		size += syscall.Size(n)
		if err != nil {
			return size, makeErrno(err)
		}
	}
	return size, syscall.ESUCCESS
}

// FdRead is the implementation of the "fd_read" syscall.
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_read
func (ctx *Context) FdRead(fd syscall.Fd, iovs [][]byte) (syscall.Size, syscall.Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return 0, syscall.EBADF
	}
	size := syscall.Size(0)
	for _, buf := range iovs {
		n, err := f.Read(buf)
		size += syscall.Size(n)
		if err != nil {
			return size, makeErrno(err)
		}
	}
	return size, syscall.ESUCCESS
}

// FdReaddir is the implementation of the "fd_readdir" syscall.
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_readdir
func (ctx *Context) FdReaddir(fd syscall.Fd, buf []byte, dircookie syscall.Dircookie) (syscall.Size, syscall.Errno) {
	f := ctx.files.lookup(fd)
	if f == nil {
		return 0, syscall.EBADF
	}
	if dircookie < f.dircookie {
		return 0, syscall.EINVAL
	}

	const readDirChunkSize = 10
	for f.dircookie < dircookie {
		d := dircookie - f.dircookie
		n := syscall.Dircookie(len(f.direntries))
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

	size := syscall.Size(0)
	for size < syscall.Size(len(buf)) {
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

		d := syscall.Dirent{
			Next:    f.dircookie + 1,
			Ino:     0, // TODO?
			Namelen: syscall.Dirnamlen(len(name)),
			Type:    makeFiletype(mode),
		}

		r := syscall.Size(len(buf)) - size
		n := d.Size()
		b := d.Marshal()
		size += syscall.Size(copy(buf[size:], b[:]))
		size += syscall.Size(copy(buf[size:], name))

		if n <= r {
			f.dircookie++
			f.direntries = f.direntries[1:]
		}
	}
	return size, syscall.ESUCCESS
}

// PathOpen is the implementation of the "path_open" syscall.
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#path_open
func (ctx *Context) PathOpen(fd syscall.Fd, dirflags syscall.Lookupflags, path string, oflags syscall.Oflags, fsRightsBase, fsRightsInheriting syscall.Rights, fdflags syscall.Fdflags) (syscall.Fd, syscall.Errno) {
	var base File
	var err error

	flags, perm := makeOpenFileFlags(dirflags, oflags, fsRightsBase, fsRightsInheriting, fdflags)
	if fd == syscall.None || strings.HasPrefix(path, "/") {
		base, err = ctx.fsys.OpenFile(path, flags, perm)
	} else {
		f := ctx.files.lookup(fd)
		if f == nil {
			return syscall.None, syscall.EBADF
		}
		fsRightsBase &= ^f.fsRightsInheriting
		fsRightsInheriting &= ^f.fsRightsInheriting
		base, err = f.OpenFile(path, flags, perm)
	}
	if err != nil {
		return syscall.None, makeErrno(err)
	}

	newFd := ctx.files.insert(&file{
		base:               base,
		fsRightsBase:       fsRightsBase,
		fsRightsInheriting: fsRightsInheriting,
	})
	return newFd, syscall.ESUCCESS
}

// PathFilestatGet is the implementation of the "path_filestat_get" syscall.
//
// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#path_filestat_get
func (ctx *Context) PathFilestatGet(fd syscall.Fd, flags syscall.Lookupflags, path string) (syscall.Filestat, syscall.Errno) {
	var info fs.FileInfo
	var err error

	if fd == syscall.None || strings.HasPrefix(path, "/") {
		info, err = ctx.fsys.StatFile(path, makeDefaultFlags(flags))
	} else {
		f := ctx.files.lookup(fd)
		if f == nil {
			return syscall.Filestat{}, syscall.EBADF
		}
		info, err = f.StatFile(path, makeDefaultFlags(flags))
	}

	if err != nil {
		return syscall.Filestat{}, makeErrno(err)
	}
	return makeFilestat(info), syscall.ESUCCESS
}

// FS returns a file system backed by the context that it is called on.
func (ctx *Context) FS() FS { return contextFS{ctx} }

type contextFS struct{ ctx *Context }

func (fsys contextFS) Open(name string) (fs.File, error) {
	return fsys.OpenFile(name, 0, 0)
}

func (fsys contextFS) OpenFile(path string, flags int, perm fs.FileMode) (File, error) {
	dirflags, oflags, fsRightsBase, fsRightsInheriting, fdflags := makePathOpenFlags(flags, perm)
	fd, errno := fsys.ctx.PathOpen(syscall.None, dirflags, path, oflags, fsRightsBase, fsRightsInheriting, fdflags)
	if err := makeError(errno); err != nil {
		return nil, err
	}
	return &contextFile{ctx: fsys.ctx, fd: fd, name: fspath.Base(path)}, nil
}

func (fsys contextFS) Stat(path string) (fs.FileInfo, error) {
	return fsys.StatFile(path, 0)
}

func (fsys contextFS) StatFile(path string, flags int) (fs.FileInfo, error) {
	stat, errno := fsys.ctx.PathFilestatGet(syscall.None, makeLookupflags(flags), path)
	if err := makeError(errno); err != nil {
		return nil, err
	}
	return &contextFileInfo{name: fspath.Base(path), stat: stat}, nil
}

type contextFile struct {
	ctx  *Context
	fd   syscall.Fd
	name string
	// iterator state used when calling ReadDir
	dircookie syscall.Dircookie
	dirbuffer []byte
}

func (f *contextFile) Name() string {
	return f.name
}

func (f *contextFile) Close() error {
	fd := f.fd
	f.fd = syscall.None
	return makeError(f.ctx.FdClose(fd))
}

func (f *contextFile) OpenFile(path string, flags int, perm fs.FileMode) (File, error) {
	dirflags, oflags, fsRightsBase, fsRightsInheriting, fdflags := makePathOpenFlags(flags, perm)
	fd, errno := f.ctx.PathOpen(f.fd, dirflags, path, oflags, fsRightsBase, fsRightsInheriting, fdflags)
	if err := makeError(errno); err != nil {
		return nil, err
	}
	return &contextFile{ctx: f.ctx, fd: fd, name: fspath.Base(path)}, nil
}

func (f *contextFile) Read(b []byte) (int, error) {
	size, errno := f.ctx.FdRead(f.fd, [][]byte{b})
	if size == 0 && errno == syscall.ESUCCESS && len(b) != 0 {
		return 0, io.EOF
	}
	return int(size), makeError(errno)
}

func (f *contextFile) ReadAt(b []byte, off int64) (int, error) {
	size, errno := f.ctx.FdPread(f.fd, [][]byte{b}, syscall.Filesize(off))
	if size < syscall.Size(len(b)) && errno == syscall.ESUCCESS {
		return int(size), io.EOF
	}
	return int(size), makeError(errno)
}

func (f *contextFile) ReadDir(n int) (ret []fs.DirEntry, err error) {
	if n < 0 {
		n = 0
	}
	ent := make([]*contextDirEntry, 0, n)
	buf := make([]byte, MaxPathLen+24)

	for n == 0 || len(ent) < n {
		b := buf[:]
		size := syscall.Size(0)
		errno := syscall.Errno(0)

		if len(f.dirbuffer) == 0 {
			size, errno = f.ctx.FdReaddir(f.fd, b, f.dircookie)
			b = b[:size]
		} else {
			b, f.dirbuffer = f.dirbuffer, f.dirbuffer[:0]
			size = syscall.Size(len(b))
		}

		for len(b) >= 24 && (n == 0 || len(ent) < n) {
			d := syscall.Dirent{}
			d.Unmarshal(*(*[24]byte)(b))
			b = b[24:]

			if d.Namelen > syscall.Dirnamlen(len(b)) {
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

		if errno != syscall.ESUCCESS {
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
	return 0, ErrNotImplemented
}

func (f *contextFile) WriteAt(b []byte, off int64) (int, error) {
	return 0, ErrNotImplemented
}

func (f *contextFile) Seek(offset int64, whence int) (int64, error) {
	var size syscall.Filesize
	var errno syscall.Errno
	if offset == 0 && whence == io.SeekCurrent {
		size, errno = f.ctx.FdTell(f.fd)
	} else {
		size, errno = f.ctx.FdSeek(f.fd, syscall.Filedelta(offset), syscall.Whence(whence))
	}
	return int64(size), makeError(errno)
}

func (f *contextFile) Stat() (fs.FileInfo, error) {
	stat, errno := f.ctx.FdFilestatGet(f.fd)
	if err := makeError(errno); err != nil {
		return nil, err
	}
	return &contextFileInfo{name: f.name, stat: stat}, nil
}

func (f *contextFile) StatFile(path string, flags int) (fs.FileInfo, error) {
	stat, errno := f.ctx.PathFilestatGet(f.fd, 0, path)
	if err := makeError(errno); err != nil {
		return nil, err
	}
	return &contextFileInfo{name: fspath.Base(path), stat: stat}, nil
}

type contextFileInfo struct {
	name string
	stat syscall.Filestat
}

func (f *contextFileInfo) Name() string       { return f.name }
func (f *contextFileInfo) Size() int64        { return int64(f.stat.Size) }
func (f *contextFileInfo) Mode() fs.FileMode  { return makeFileMode(f.stat.Filetype) }
func (f *contextFileInfo) ModTime() time.Time { return time.Unix(0, int64(f.stat.Mtim)) }
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
