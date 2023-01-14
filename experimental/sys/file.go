package sys

import (
	"io"
	"io/fs"
	"time"
)

// File is an interface implemented by files opened by FS instsances.
//
// The interfance is similar to fs.File, it may represent different types of
// files, including regular files and directories.
type File interface {
	fs.File
	io.ReaderAt
	io.Writer
	io.WriterAt
	io.Seeker
	// Returns the target of the symbolic link that file is opened at.
	Readlink() (string, error)
	// Sets the file permissions.
	Chmod(mode fs.FileMode) error
	// Sets the file access and modification times.
	Chtimes(atime, mtime time.Time) error
	// Sets the file size.
	Truncate(size int64) error
	// Flushes all buffered changes to persistent storage.
	Sync() error
	// Flushes buffered data changes to persistent storage.
	Datasync() error
	// A file might be open an a directory of the file system, in which case
	// the methods provided by the Directory interface allow access to the
	// file system directory tree relative to the file location.
	//
	// If the file is not referencing a directory, calling methods of the
	// Directory interface will fail returning ErrNotDirectory or ErrPermission.
	Directory
}

// Directory is an interface representing an open directory.
//
// Methods accepting a file name perform name resolution relative to the
// location of the directory on the file system.
//
// The file names passed to methods of the Directory interface must be valid
// accoring to ValidPath. For all invalid names, the methods return ErrNotExist.
type Directory interface {
	// Returns the file system handle for this directory.
	Fd() uintptr
	// Opens a file at the given name, relative to the directory.
	OpenFile(name string, flags int, perm fs.FileMode) (File, error)
	// Reads the list of directory entries (see fs.ReadDirFile).
	ReadDir(n int) ([]fs.DirEntry, error)
	// Creates a directory on the file system.
	Mkdir(name string, perm fs.FileMode) error
	// Removes a directory from the file system.
	Rmdir(name string) error
	// Removes a file from the file system.
	Unlink(name string) error
	// Creates a symolink link from oldName to newName.
	Symlink(oldName, newName string) error
	// Creates a hard link from oldName to newName. oldName is expressed
	// relative to the receiver, while newName is expressed relative to newDir.
	Link(oldName string, newDir Directory, newName string) error
	// Moves a file from oldName to newName. oldName is expressed relative to
	// the receivers, while newName is expressed relative to newDir.
	Rename(oldName string, newDir Directory, newName string) error
}

// NewFile creates a wrapper around the given file which ensures that the
// resulting file will satisfy a set of base expectations of the File
// interface.
//
// The returned File ensures that none of the file methods will be called after
// Close. It also wraps all errors with fs.PathError (except io.EOF), using the
// given file name where appropriate to inject context into the error. The File
// also performs validation of all the method inputs, guaranteeing that the
// methods of the underlying file will only be called with valid inputs.
func NewFile(base File, name string) File {
	switch base.(type) {
	case *file, *readOnlyFile:
		// These are the two internal wrapper types we use, there is no need
		// to wrap them multiple times so if we detect them here we can simply
		// return the input. The name might differ but it's only used to carry
		// context in errors, it doe not alter the file behavior.
		return base
	}
	return &file{base, name}
}

type file struct {
	base File
	name string
}

func (f *file) Fd() uintptr {
	if f.base != nil {
		return f.base.Fd()
	}
	return ^uintptr(0)
}

func (f *file) Close() (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = f.base.Close()
		f.base = nil
	}
	if err != nil {
		err = f.makePathError("close", err)
	}
	return err
}

func (f *file) Stat() (info fs.FileInfo, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		info, err = f.base.Stat()
	}
	if err != nil {
		err = f.makePathError("stat", err)
	}
	return info, err
}

func (f *file) Read(b []byte) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		n, err = f.base.Read(b)
	}
	if err != nil && err != io.EOF {
		err = f.makePathError("read", err)
	}
	return n, err
}

func (f *file) ReadAt(b []byte, offset int64) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		n, err = f.base.ReadAt(b, offset)
	}
	if err != nil && err != io.EOF {
		err = f.makePathError("read", err)
	}
	return n, err
}

func (f *file) ReadFrom(r io.Reader) (n int64, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		n, err = io.Copy(f.base, r)
	}
	if err != nil {
		err = f.makePathError("write", err)
	}
	return n, err
}

func (f *file) Write(b []byte) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		n, err = f.base.Write(b)
	}
	if err != nil {
		err = f.makePathError("write", err)
	}
	return n, err
}

func (f *file) WriteAt(b []byte, offset int64) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		n, err = f.base.WriteAt(b, offset)
	}
	if err != nil {
		err = f.makePathError("write", err)
	}
	return n, err
}

func (f *file) WriteString(s string) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		n, err = io.WriteString(f.base, s)
	}
	if err != nil {
		err = f.makePathError("write", err)
	}
	return n, err
}

func (f *file) Seek(offset int64, whence int) (seek int64, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		seek, err = f.base.Seek(offset, whence)
	}
	if err != nil {
		err = f.makePathError("seek", err)
	}
	return seek, err
}

func (f *file) ReadDir(n int) (files []fs.DirEntry, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		files, err = f.base.ReadDir(n)
	}
	if err != nil && err != io.EOF {
		err = f.makePathError("readdir", err)
	}
	return files, err
}

func (f *file) Readlink() (link string, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		link, err = f.base.Readlink()
	}
	if err != nil {
		err = f.makePathError("readlink", err)
	}
	return link, err
}

func (f *file) Chmod(perm fs.FileMode) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = f.base.Chmod(perm)
	}
	if err != nil {
		err = f.makePathError("chmod", err)
	}
	return err
}

func (f *file) Chtimes(atime, mtime time.Time) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = f.base.Chtimes(atime, mtime)
	}
	if err != nil {
		err = f.makePathError("chtimes", err)
	}
	return err
}

func (f *file) Truncate(size int64) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else if size < 0 {
		err = ErrInvalid
	} else {
		err = f.base.Truncate(size)
	}
	if err != nil {
		err = f.makePathError("truncate", err)
	}
	return err
}

func (f *file) Sync() (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = f.base.Sync()
	}
	if err != nil {
		err = f.makePathError("sync", err)
	}
	return err
}

func (f *file) Datasync() (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = f.base.Datasync()
	}
	if err != nil {
		err = f.makePathError("datasync", err)
	}
	return err
}

func (f *file) OpenFile(name string, flags int, perm fs.FileMode) (file File, err error) {
	if f.base == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		file, err = f.base.OpenFile(name, flags, perm)
	}
	if err != nil {
		err = makePathError("open", name, err)
	}
	return file, err
}

func (f *file) Mkdir(name string, perm fs.FileMode) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		err = f.base.Mkdir(name, perm)
	}
	if err != nil {
		err = makePathError("mkdir", name, err)
	}
	return err
}

func (f *file) Rmdir(name string) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		err = f.base.Rmdir(name)
	}
	if err != nil {
		err = makePathError("rmdir", name, err)
	}
	return err
}

func (f *file) Unlink(name string) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else if !ValidPath(name) {
		err = ErrNotExist
	} else {
		err = f.base.Unlink(name)
	}
	if err != nil {
		err = f.makePathError("unlink", err)
	}
	return err
}

func (f *file) Symlink(oldName, newName string) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else if !ValidPath(newName) {
		err = ErrNotExist
	} else {
		err = f.base.Symlink(oldName, newName)
	}
	if err != nil {
		err = makePathError("symlink", newName, err)
	}
	return err
}

func (f *file) Link(oldName string, newDir Directory, newName string) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else if !ValidPath(oldName) {
		err = ErrNotExist
	} else if !ValidPath(newName) {
		err = ErrInvalid
	} else {
		err = f.base.Link(oldName, newDir, newName)
	}
	if err != nil {
		err = makePathError("link", newName, err)
	}
	return err
}

func (f *file) Rename(oldName string, newDir Directory, newName string) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else if !ValidPath(oldName) {
		err = ErrNotExist
	} else if !ValidPath(newName) {
		err = ErrInvalid
	} else {
		err = f.base.Rename(oldName, newDir, newName)
	}
	if err != nil {
		err = makePathError("rename", newName, err)
	}
	return err
}

func (f *file) makePathError(op string, err error) error {
	return makePathError(op, f.name, err)
}

var (
	_ io.ReaderFrom   = (*file)(nil)
	_ io.StringWriter = (*file)(nil)
)