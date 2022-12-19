package wasi

import (
	"io"
	"io/fs"

	"github.com/tetratelabs/wazero/wasi/syscall"
)

type File interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Writer
	io.WriterAt
	io.Seeker
	fs.ReadDirFile

	Name() string

	OpenFile(path string, flags int, perm fs.FileMode) (File, error)

	StatFile(path string, flags int) (fs.FileInfo, error)
}

type file struct {
	base               File
	fsRightsBase       syscall.Rights
	fsRightsInheriting syscall.Rights
	dircookie          syscall.Dircookie
	direntries         []fs.DirEntry
}

func (f *file) Name() string { return f.base.Name() }

func (f *file) Close() error { return f.base.Close() }

func (f *file) OpenFile(path string, flags int, perm fs.FileMode) (File, error) {
	if !f.fsRightsBase.Has(syscall.PATH_OPEN) {
		return nil, fs.ErrPermission
	}
	return f.base.OpenFile(path, flags, perm)
}

func (f *file) Read(b []byte) (int, error) {
	if !f.fsRightsBase.Has(syscall.FD_READ) {
		return 0, fs.ErrPermission
	}
	return f.base.Read(b)
}

func (f *file) ReadAt(b []byte, off int64) (int, error) {
	if !f.fsRightsBase.Has(syscall.FD_READ | syscall.FD_SEEK) {
		return 0, fs.ErrPermission
	}
	return f.base.ReadAt(b, off)
}

func (f *file) Write(b []byte) (int, error) {
	if !f.fsRightsBase.Has(syscall.FD_WRITE) {
		return 0, fs.ErrPermission
	}
	return f.base.Write(b)
}

func (f *file) WriteAt(b []byte, off int64) (int, error) {
	if !f.fsRightsBase.Has(syscall.FD_WRITE | syscall.FD_SEEK) {
		return 0, fs.ErrPermission
	}
	return f.base.WriteAt(b, off)
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	rights := syscall.Rights(0)
	if offset == 0 && whence == io.SeekCurrent {
		rights = syscall.FD_TELL
	} else {
		rights = syscall.FD_SEEK
	}
	if !f.fsRightsBase.Has(rights) {
		return -1, fs.ErrPermission
	}
	return f.base.Seek(offset, whence)
}

func (f *file) Stat() (fs.FileInfo, error) {
	if !f.fsRightsBase.Has(syscall.FD_FILESTAT_GET) {
		return nil, fs.ErrPermission
	}
	return f.base.Stat()
}

func (f *file) StatFile(path string, flags int) (fs.FileInfo, error) {
	if !f.fsRightsBase.Has(syscall.FD_FILESTAT_GET) {
		return nil, fs.ErrPermission
	}
	return f.base.StatFile(path, flags)
}

func (f *file) ReadDir(n int) ([]fs.DirEntry, error) {
	if !f.fsRightsBase.Has(syscall.FD_READDIR) {
		return nil, fs.ErrPermission
	}
	return f.base.ReadDir(n)
}

var (
	_ File = (*file)(nil)
)
