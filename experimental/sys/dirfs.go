package sys

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DirFS constructs a FS instance using the given path as root on the host's
// file system.
//
// The path is first converted to an obsolute path on the file system in order
// to ensure that the behavior of the returned FS does not change if the working
// directory changes.
func DirFS(path string) FS {
	path, err := filepath.Abs(path)
	if err != nil {
		return ErrFS(err)
	}
	return dirFS(path)
}

type dirFS string

func (path dirFS) Open(name string) (fs.File, error) { return Open(path, name) }

func (path dirFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	f, err := path.openFile(name, flags, perm)
	if err != nil {
		return nil, makePathError("open", name, err)
	}
	return f, nil
}

func (path dirFS) openFile(name string, flags int, perm fs.FileMode) (File, error) {
	if !ValidPath(name) {
		return nil, ErrNotExist
	}
	osPath := filepath.Join(string(path), filepath.FromSlash(name))
	osFile, err := openFile(osPath, flags, perm)
	if err != nil {
		return nil, err
	}
	return newFile(dirFile{osFile}), nil
}

type dirFile struct{ *os.File }

func (f dirFile) GoString() string { return fmt.Sprintf("sys.dirFile{%q}", f.Name()) }

func (f dirFile) Sys() any { return f.File }

func (f dirFile) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	if !ValidPath(name) {
		return nil, makePathError("open", name, ErrNotExist)
	}
	osFile, err := f.openFile(name, flags, perm)
	if err != nil {
		return nil, err
	}
	return newFile(dirFile{osFile}), nil
}

func (f dirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	dirents, err := f.File.ReadDir(n)
	normalizePathError(err, "use of closed file", ErrClosed)
	return dirents, err
}

func (f dirFile) Stat() (fs.FileInfo, error) {
	stat, err := f.File.Stat()
	normalizePathError(err, "use of closed file", ErrClosed)
	return stat, err
}

func (f dirFile) Truncate(size int64) error {
	err := f.File.Truncate(size)
	normalizePathError(err, "invalid argument", ErrInvalid)
	return err
}

func normalizePathError(err error, msg string, repl error) {
	if e, _ := err.(*fs.PathError); e != nil {
		if unwrap(e.Err).Error() == msg {
			e.Err = repl
		}
	}
}
