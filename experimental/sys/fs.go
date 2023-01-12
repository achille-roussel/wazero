package sys

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"time"
)

// FS is an interface representing file systems.
//
// FS is an extension of fs.FS which depending on the underlying backend,
// may allow write operations.
type FS interface {
	fs.StatFS
	// Opens a file on the file system.
	//
	// The signature of this method is similar to os.OpenFile, it receives a
	// bitset of flags configuring properties of the opened file. If the file
	// is to be created (e.g. because O_CREATE was passed) the perm argument
	// is used to set the initial permissions on the newly created file.
	OpenFile(name string, flags int, perm fs.FileMode) (File, error)
	// Creates a directory on the file system.
	Mkdir(name string, perm fs.FileMode) error
	// Removes a directory from the file system.
	Rmdir(name string) error
	// Removes a file from the file system.
	Unlink(name string) error
	// Creates a hard link from oldName to newName. oldName is expressed
	// relative to the receiver, while newName is expressed relative to newFS.
	//
	// The newFS value should either be the same as the receiver, or a FS
	// instance obtained by calling FS on a File obtained from the receiver.
	Link(oldName, newName string, newFS FS) error
	// Moves a file from oldName to newName. oldName is expressed relative to
	// the receivers, while newName is expressed relative to newFS.
	//
	// The newFS value should either be the same as the receiver, or a FS
	// instance obtained by calling FS on a File obtained from the receiver.
	Rename(oldName, newName string, newFS FS) error
	// Creates a symolink link from oldName to newName.
	Symlink(oldName, newName string) error
	// Reads the value of the given symbolic link.
	Readlink(name string) (string, error)
	// Changes a file permissions on the file system.
	Chmod(name string, mode fs.FileMode) error
	// Changes a file access and modification times.
	Chtimes(name string, atime, mtime time.Time) error
	// Changes the size of a file on the file system.
	Truncate(name string, size int64) error
}

// File is an interface implemented by files opened by FS instsances.
//
// The interfance is similar to fs.File, it may represent different types of
// files, including regular files and directories.
type File interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Writer
	io.WriterAt
	io.Seeker
	fs.ReadDirFile
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
	// Returns a view of the file system rooted at the file (which must be a
	// directory).
	//
	// All name resolutions are done relative to the file location.
	//
	// The returned FS remains valid until the file is closed, after which all
	// method calls on the FS return ErrClosed.
	FS() FS
}

// NewFS constructs a FS from a fs.FS.
//
// The returned file system is read-only, all attempts to open files in write
// mode, or mutate the state of the file system will error with ErrReadOnly.
func NewFS(base fs.FS) FS { return &fsFS{base, errFS{ErrReadOnly}} }

type fsFS struct {
	base fs.FS
	errFS
}

func (fsys *fsFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	f, err := fsys.openFile(name, flags, perm)
	if err != nil {
		return nil, makePathError("open", name, err)
	}
	return f, nil
}

func (fsys *fsFS) openFile(name string, flags int, perm fs.FileMode) (*fsFile, error) {
	if !ValidPath(name) {
		return nil, ErrNotExist
	}
	if (flags & ^openFileReadOnlyFlags) != 0 {
		return nil, ErrReadOnly
	}

	f, err := fsys.base.Open(name)
	if err != nil {
		return nil, err
	}

	if (flags & O_DIRECTORY) != 0 {
		s, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		if !s.IsDir() {
			f.Close()
			return nil, ErrNotDirectory
		}
	}

	return &fsFile{fsys: fsys, base: f, name: name}, nil
}

func (fsys *fsFS) Open(name string) (fs.File, error) {
	return fsys.OpenFile(name, O_RDONLY, 0)
}

func (fsys *fsFS) Stat(name string) (fs.FileInfo, error) {
	if !ValidPath(name) {
		return nil, makePathError("stat", name, ErrNotExist)
	}
	return fs.Stat(fsys.base, name)
}

func (fsys *fsFS) Link(oldName, newName string, newFS FS) error {
	switch fsys2 := newFS.(type) {
	case *fsFS:
		return fsys.errFS.Link(oldName, newName, &fsys2.errFS)
	case fsFileFS:
		f, err := fsys.openFile(".", O_DIRECTORY, 0)
		if err != nil {
			return makePathError("link", oldName, err)
		}
		defer f.Close()
		return fsFileFS{f}.Link(oldName, newName, fsys2)
	default:
		return makePathError("link", newName, ErrInvalid)
	}
}

func (fsys *fsFS) Rename(oldName, newName string, newFS FS) error {
	switch fsys2 := newFS.(type) {
	case *fsFS:
		return fsys.errFS.Link(oldName, newName, &fsys2.errFS)
	case fsFileFS:
		f, err := fsys.openFile(".", O_DIRECTORY, 0)
		if err != nil {
			return makePathError("rename", oldName, err)
		}
		defer f.Close()
		return fsFileFS{f}.Rename(oldName, newName, fsys2)
	default:
		return makePathError("rename", newName, ErrInvalid)
	}
}

func (fsys *fsFS) Readlink(name string) (link string, err error) {
	if !ValidPath(name) {
		err = ErrNotExist
	} else {
		err = ErrNotImplemented
	}
	return link, makePathError("readlink", name, err)
}

type fsFile struct {
	fsys *fsFS
	base fs.File
	name string
}

func (f *fsFile) Close() (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		defer func() { f.fsys, f.base = nil, nil }()
		err = f.base.Close()
	}
	if err != nil {
		err = f.makePathError("close", err)
	}
	return err
}

func (f *fsFile) Stat() (info fs.FileInfo, err error) {
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

func (f *fsFile) Read(b []byte) (n int, err error) {
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

func (f *fsFile) ReadAt(b []byte, offset int64) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else if r, ok := f.base.(io.ReaderAt); ok {
		n, err = r.ReadAt(b, offset)
	} else {
		err = ErrNotSupported
	}
	if err != nil && err != io.EOF {
		err = f.makePathError("read", err)
	}
	return n, err
}

func (f *fsFile) Write(b []byte) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = ErrNotSupported
	}
	return 0, f.makePathError("write", err)
}

func (f *fsFile) WriteAt(b []byte, offset int64) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = ErrNotSupported
	}
	return 0, f.makePathError("write", err)
}

func (f *fsFile) WriteString(s string) (n int, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = ErrNotSupported
	}
	return 0, f.makePathError("write", err)
}

func (f *fsFile) ReadFrom(r io.Reader) (n int64, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = ErrNotSupported
	}
	return 0, f.makePathError("write", err)
}

func (f *fsFile) Seek(offset int64, whence int) (seek int64, err error) {
	if f.base == nil {
		err = ErrClosed
	} else if r, ok := f.base.(io.Seeker); ok {
		seek, err = r.Seek(offset, whence)
	} else {
		err = ErrNotSupported
	}
	if err != nil {
		err = f.makePathError("seek", err)
	}
	return seek, err
}

func (f *fsFile) ReadDir(n int) (files []fs.DirEntry, err error) {
	if f.base == nil {
		err = ErrClosed
	} else if d, ok := f.base.(fs.ReadDirFile); ok {
		files, err = d.ReadDir(n)
	} else {
		err = ErrNotSupported
	}
	if err != nil && err != io.EOF {
		err = f.makePathError("readdir", err)
	}
	return files, err
}

func (f *fsFile) Readlink() (link string, err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = ErrNotSupported
	}
	return link, f.makePathError("readlink", err)
}

func (f *fsFile) Chmod(mode fs.FileMode) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = ErrReadOnly
	}
	return f.makePathError("chmod", err)
}

func (f *fsFile) Chtimes(atime, mtime time.Time) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = ErrReadOnly
	}
	return f.makePathError("chtimes", err)
}

func (f *fsFile) Truncate(size int64) (err error) {
	if f.base == nil {
		err = ErrClosed
	} else if size >= 0 {
		err = ErrReadOnly
	} else {
		err = ErrInvalid
	}
	return f.makePathError("truncate", err)
}

func (f *fsFile) Sync() (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = ErrReadOnly
	}
	return f.makePathError("sync", err)
}

func (f *fsFile) Datasync() (err error) {
	if f.base == nil {
		err = ErrClosed
	} else {
		err = ErrReadOnly
	}
	return f.makePathError("datasync", err)
}

func (f *fsFile) makePathError(op string, err error) error {
	return makePathError(op, f.name, err)
}

func (f *fsFile) FS() FS { return fsFileFS{f} }

type fsFileFS struct{ *fsFile }

func (f fsFileFS) join(op, name string) (string, error) {
	if f.fsys == nil {
		return "", makePathError(op, name, ErrClosed)
	}
	if !ValidPath(name) {
		return "", makePathError(op, name, ErrNotExist)
	}
	return path.Join(f.name, name), nil
}

func (f fsFileFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	name, err := f.join("open", name)
	if err != nil {
		return nil, err
	}
	return f.fsys.OpenFile(name, flags, perm)
}

func (f fsFileFS) Open(name string) (fs.File, error) {
	return f.OpenFile(name, O_RDONLY, 0)
}

func (f fsFileFS) Mkdir(name string, perm fs.FileMode) error {
	name, err := f.join("mkdir", name)
	if err != nil {
		return err
	}
	return f.fsys.Mkdir(name, perm)
}

func (f fsFileFS) Rmdir(name string) error {
	name, err := f.join("rmdir", name)
	if err != nil {
		return err
	}
	return f.fsys.Rmdir(name)
}

func (f fsFileFS) Unlink(name string) error {
	name, err := f.join("unlink", name)
	if err != nil {
		return err
	}
	return f.fsys.Unlink(name)
}

func (f fsFileFS) Link(oldName, newName string, newFS FS) error {
	oldName, err := f.join("link", oldName)
	if err != nil {
		return err
	}
	switch g := newFS.(type) {
	case fsFileFS:
		newName, err = g.join("link", newName)
		if err != nil {
			return ErrInvalid
		}
		newFS = g.fsys
	default:
		return makePathError("link", newName, ErrInvalid)
	}
	return f.fsys.Link(oldName, newName, newFS)
}

func (f fsFileFS) Rename(oldName, newName string, newFS FS) error {
	oldName, err := f.join("rename", oldName)
	if err != nil {
		return err
	}
	switch g := newFS.(type) {
	case fsFileFS:
		newName, err = g.join("rename", newName)
		if err != nil {
			return ErrInvalid
		}
		newFS = g.fsys
	default:
		return ErrInvalid
	}
	return f.fsys.Rename(oldName, newName, newFS)
}

func (f fsFileFS) Symlink(oldName, newName string) error {
	newName, err := f.join("symlink", newName)
	if err != nil {
		return err
	}
	return f.fsys.Symlink(oldName, newName)
}

func (f fsFileFS) Readlink(name string) (string, error) {
	name, err := f.join("readlink", name)
	if err != nil {
		return "", err
	}
	return f.fsys.Readlink(name)
}

func (f fsFileFS) Chmod(name string, mode fs.FileMode) error {
	name, err := f.join("chmod", name)
	if err != nil {
		return err
	}
	return f.fsys.Chmod(name, mode)
}

func (f fsFileFS) Chtimes(name string, atime, mtime time.Time) error {
	name, err := f.join("chtimes", name)
	if err != nil {
		return err
	}
	return f.fsys.Chtimes(name, atime, mtime)
}

func (f fsFileFS) Truncate(name string, size int64) error {
	name, err := f.join("truncate", name)
	if err != nil {
		return err
	}
	return f.fsys.Truncate(name, size)
}

func (f fsFileFS) Stat(name string) (fs.FileInfo, error) {
	name, err := f.join("stat", name)
	if err != nil {
		return nil, err
	}
	return f.fsys.Stat(name)
}

var (
	_ io.ReaderFrom   = (*fsFile)(nil)
	_ io.StringWriter = (*fsFile)(nil)
)

// ErrFS returns a FS which errors with err on all its method calls.
func ErrFS(err error) FS { return &errFS{err: err} }

type errFS struct{ err error }

func (fsys *errFS) Open(name string) (fs.File, error) {
	return fsys.OpenFile(name, O_RDONLY, 0)
}

func (fsys *errFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	return nil, fsys.validPath("open", name)
}

func (fsys *errFS) Mkdir(name string, perm fs.FileMode) error {
	return fsys.validPath("mkdir", name)
}

func (fsys *errFS) Rmdir(name string) error {
	return fsys.validPath("rmdir", name)
}

func (fsys *errFS) Unlink(name string) error {
	return fsys.validPath("unlink", name)
}

func (fsys *errFS) Link(oldName, newName string, newFS FS) error {
	return fsys.validLink("link", oldName, newName, newFS)
}

func (fsys *errFS) Symlink(oldName, newName string) error {
	return fsys.validPath("symlink", newName)
}

func (fsys *errFS) Readlink(name string) (string, error) {
	return "", fsys.validPath("readlink", name)
}

func (fsys *errFS) Rename(oldName, newName string, newFS FS) error {
	return fsys.validLink("rename", oldName, newName, newFS)
}

func (fsys *errFS) Chmod(name string, mode fs.FileMode) error {
	return fsys.validPath("chmod", name)
}

func (fsys *errFS) Chtimes(name string, atime, mtime time.Time) error {
	return fsys.validPath("chtimes", name)
}

func (fsys *errFS) Truncate(name string, size int64) error {
	return fsys.validPath("truncate", name)
}

func (fsys *errFS) Stat(name string) (fs.FileInfo, error) {
	return nil, fsys.validPath("stat", name)
}

func (fsys *errFS) validPath(op, name string) (err error) {
	if !ValidPath(name) {
		err = ErrNotExist
	} else {
		err = fsys.err
	}
	return makePathError(op, name, err)
}

func (fsys *errFS) validLink(op, oldName, newName string, newFS FS) error {
	var name string
	var err error
	switch {
	case !ValidPath(oldName):
		name, err = oldName, ErrNotExist
	case !ValidPath(newName):
		name, err = newName, ErrInvalid
	default:
		name, err = oldName, fsys.err
	}
	return makePathError(op, name, err)
}

// CopyFS copies the file system src into dst.
//
// The function recreates the directory tree of src into dst, starting from the
// root and recursively descending into each directory. The copy is not atomic,
// an error might leave the destination file system with a partially completed
// copy of the file tree.
func CopyFS(dst FS, src fs.FS) error {
	return fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}

		if d.IsDir() {
			s, err := d.Info()
			if err != nil {
				return err
			}
			err = dst.Mkdir(path, s.Mode())
			if err == nil {
				atime := time.Time{}
				mtime := s.ModTime()
				err = dst.Chtimes(path, atime, mtime)
			}
			return err
		}

		r, err := src.Open(path)
		if err != nil {
			return err
		}
		defer r.Close()

		s, err := r.Stat()
		if err != nil {
			return err
		}

		w, err := dst.OpenFile(path, O_CREATE|O_TRUNC|O_WRONLY, s.Mode())
		if err != nil {
			return err
		}
		defer w.Close()

		if _, err := io.Copy(w, r); err != nil {
			return err
		}

		atime := time.Time{}
		mtime := s.ModTime()
		return w.Chtimes(atime, mtime)
	})
}

// EqualFS compares two file systems, returning nil if they are equal, or an
// error describing their difference when they are not.
func EqualFS(a, b fs.FS) error {
	var buf [8192]byte
	if err := equalFS(a, b, &buf); err != nil {
		return fmt.Errorf("equalFS(a,b): %w", err)
	}
	if err := equalFS(b, a, &buf); err != nil {
		return fmt.Errorf("equalFS(b,a): %w", err)
	}
	return nil
}

func equalFS(source, target fs.FS, buf *[8192]byte) error {
	return fs.WalkDir(source, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}

		sourceInfo, err := d.Info()
		if err != nil {
			return err
		}

		if sourceInfo.Mode().Type() == fs.ModeSymlink {
			// fs.Stat follows symbolic links, but they may be broken.
			sourceInfo, err = fs.Stat(source, path)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					_, targetErr := fs.Stat(target, path)
					if errors.Is(targetErr, fs.ErrNotExist) {
						err = nil
					}
				}
				return err
			}
		}

		targetInfo, err := fs.Stat(target, path)
		if err != nil {
			return err
		}

		sourceMode := sourceInfo.Mode()
		targetMode := targetInfo.Mode()
		if sourceMode != targetMode {
			return pathErrorf("stat", path, "file modes mismatch: want=%s got=%s", sourceMode, targetMode)
		}
		if sourceMode.IsDir() {
			return nil
		}

		sourceTime := sourceInfo.ModTime()
		targetTime := targetInfo.ModTime()
		// Only compare the modification times if both file systems support it,
		// assuming a zero time means it's not supported.
		if !sourceTime.IsZero() && !targetTime.IsZero() {
			if !sourceTime.Equal(targetTime) {
				return pathErrorf("stat", path, "file times mismatch: want=%v got=%v", sourceTime, targetTime)
			}
		}

		sourceSize := sourceInfo.Size()
		targetSize := targetInfo.Size()
		if sourceSize != targetSize {
			return pathErrorf("stat", path, "files sizes mismatch: want=%d got=%d", sourceSize, targetSize)
		}

		sourceFile, err := source.Open(path)
		if err != nil {
			return err
		}
		defer sourceFile.Close()

		targetFile, err := target.Open(path)
		if err != nil {
			return err
		}
		defer targetFile.Close()

		buf1 := buf[:4096]
		buf2 := buf[4096:]
		for {
			n1, err1 := sourceFile.Read(buf1)
			n2, err2 := targetFile.Read(buf2)
			if n1 != n2 {
				return pathErrorf("read", path, "file read size mismatch: want=%d got=%d", n1, n2)
			}

			b1 := buf1[:n1]
			b2 := buf2[:n2]
			if !bytes.Equal(b1, b2) {
				return pathErrorf("read", path, "file content mismatch: want=%q got=%q", b1, b2)
			}

			if err1 != err2 {
				return pathErrorf("read", path, "file read error mismatch: want=%v got=%v", err1, err2)
			}
			if err1 != nil {
				break
			}
		}

		return nil
	})
}
