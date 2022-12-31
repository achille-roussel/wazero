//go:build purego || !linux

package sysfs

import (
	"io"
	"io/fs"
	"os"
	fspath "path"
	"path/filepath"
	"syscall"
	"time"
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
	// Not following symlinks or opening directories only is not supported by
	// the os package, and it is unclear at this point if the standard path
	// resolution of unix platforms is compatible with wasi; for example, does
	// not specifying lookupflags::symlink_follow mean that none of the links
	// in the path are followed, or does it mean that only the last link will
	// not be followed (like it does on linux)?
	O_NOFOLLOW  = 0
	O_DIRECTORY = 1 << 31
	// Package os does not have O_DSYNC/O_RSYNC, so fallback to O_SYNC.
	O_DSYNC = os.O_SYNC
	O_RSYNC = os.O_SYNC
)

type file struct {
	file *os.File
	name string
	path string
}

func opendir(path string) (*file, error) {
	f, err := os.OpenFile(path, 0, 0)
	if err != nil {
		return nil, err
	}
	if err := checkIsDir(f); err != nil {
		f.Close()
		return nil, err
	}
	return &file{file: f, name: ".", path: path}, nil
}

func (f *file) makeFileError(op string, err error) error {
	return makePathError(op, f.name, err)
}

func (f *file) makePathError(op, path string, err error) error {
	return makePathError(op, fspath.Join(f.name, path), err)
}

func (f *file) pathTo(path string) string {
	return filepath.Join(f.path, filepath.FromSlash(path))
}

func (f *file) Name() string {
	return f.name
}

func (f *file) Close() (err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else {
		file := f.file
		f.file = nil
		err = file.Close()
	}
	if err != nil {
		err = f.makeFileError("close", err)
	}
	return err
}

func (f *file) ReadDir(n int) (ent []fs.DirEntry, err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else {
		ent, err = f.file.ReadDir(n)
	}
	if err != nil {
		err = f.makeFileError("readdir", err)
	}
	return ent, err
}

func (f *file) Read(b []byte) (n int, err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else if len(b) == 0 {
		err = checkIsFile(f.file)
	} else {
		n, err = f.file.Read(b)
	}
	if err != nil && err != io.EOF {
		err = f.makeFileError("read", err)
	}
	return n, err
}

func (f *file) ReadAt(b []byte, off int64) (n int, err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else if len(b) == 0 {
		err = checkIsFile(f.file)
	} else {
		n, err = f.file.ReadAt(b, off)
	}
	if err != nil && err != io.EOF {
		err = f.makeFileError("read", err)
	}
	return n, err
}

func (f *file) Write(b []byte) (n int, err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else if len(b) == 0 {
		if checkIsFile(f.file) != nil {
			err = syscall.EBADF
		}
	} else {
		n, err = f.file.Write(b)
	}
	if err != nil && err != io.EOF {
		err = f.makeFileError("write", err)
	}
	return n, err
}

func (f *file) WriteAt(b []byte, off int64) (n int, err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else if len(b) == 0 {
		if checkIsFile(f.file) != nil {
			err = syscall.EBADF
		}
	} else {
		n, err = f.file.WriteAt(b, off)
	}
	if err != nil && err != io.EOF {
		err = f.makeFileError("write", err)
	}
	return n, err
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	if f.file == nil {
		return 0, f.makeFileError("seek", fs.ErrClosed)
	}
	return f.file.Seek(offset, whence)
}

func (f *file) Chmod(perm fs.FileMode) (err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else {
		err = f.file.Chmod(perm)
	}
	if err != nil {
		err = f.makeFileError("chmod", err)
	}
	return err
}

func (f *file) Chtimes(atim, mtim time.Time) (err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else {
		err = os.Chtimes(f.path, atim, mtim)
	}
	if err != nil {
		err = f.makeFileError("chtimes", err)
	}
	return err
}

func (f *file) Open(path string, flags int, perm fs.FileMode) (File, error) {
	if f.file == nil {
		return nil, f.makePathError("open", path, fs.ErrClosed)
	}
	if path = fspath.Clean(path); path == "." {
		if err := checkIsDir(f.file); err != nil {
			return nil, f.makePathError("open", path, err)
		}
	}
	openFilePath := f.pathTo(path)
	openFileFlags := flags & ^O_DIRECTORY
	openFile, err := os.OpenFile(openFilePath, openFileFlags, perm)
	if err != nil {
		return nil, err
	}
	// We emulate the handling of this flag because the standard os package does
	// not support it.
	if (flags & O_DIRECTORY) != 0 {
		if err := checkIsDir(openFile); err != nil {
			openFile.Close()
			return nil, f.makePathError("open", path, err)
		}
	}
	name := fspath.Join(f.name, path)
	return &file{file: openFile, name: name, path: openFilePath}, nil
}

func (f *file) Unlink(path string) (err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else {
		err = os.Remove(f.pathTo(path))
	}
	if err != nil {
		err = f.makePathError("unlink", path, err)
	}
	return err
}

func (f *file) Rename(oldPath, newPath string) (err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else {
		err = os.Rename(f.pathTo(oldPath), f.pathTo(newPath))
	}
	if err != nil {
		err = f.makePathError("rename", oldPath, err)
	}
	return err
}

func (f *file) Link(oldPath, newPath string) (err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else {
		err = os.Link(f.pathTo(oldPath), f.pathTo(newPath))
	}
	if err != nil {
		err = f.makePathError("link", oldPath, err)
	}
	return err
}

func (f *file) Symlink(oldPath, newPath string) (err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else {
		err = os.Symlink(f.pathTo(oldPath), f.pathTo(newPath))
	}
	if err != nil {
		err = f.makePathError("symlink", oldPath, err)
	}
	return err
}

func (f *file) Readlink(path string) (link string, err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else {
		link, err = os.Readlink(f.pathTo(path))
	}
	if err != nil {
		err = f.makePathError("readlink", path, err)
	}
	return link, err
}

func (f *file) Mkdir(path string, perm fs.FileMode) (err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else {
		err = os.Mkdir(f.pathTo(path), perm)
	}
	if err != nil {
		err = f.makePathError("mkdir", path, err)
	}
	return err
}

func (f *file) Rmdir(path string) (err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else {
		err = os.Remove(f.pathTo(path))
	}
	if err != nil {
		err = f.makePathError("rmdir", path, err)
	}
	return err
}

func (f *file) Stat() (fs.FileInfo, error) {
	if f.file == nil {
		return nil, f.makeFileError("stat", fs.ErrClosed)
	}
	return f.file.Stat()
}

func (f *file) Sync() (err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else {
		err = f.file.Sync()
	}
	if err != nil {
		err = f.makeFileError("sync", err)
	}
	return err
}

func (f *file) Datasync() (err error) {
	if f.file == nil {
		err = fs.ErrClosed
	} else {
		err = f.file.Sync()
	}
	if err != nil {
		err = f.makeFileError("datasync", err)
	}
	return err
}
