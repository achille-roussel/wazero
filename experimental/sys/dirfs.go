package sys

import (
	"io"
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
	osPath := name
	osPath = filepath.FromSlash(osPath)
	osPath = filepath.Join(string(path), osPath)
	osFile, err := openFile(osPath, flags, perm)
	if err != nil {
		return nil, err
	}
	return NewFile(dirFile{osFile}, name), nil
}

type dirFile struct{ *os.File }

func (f dirFile) Sys() any { return f.File }

func (f dirFile) ReadFrom(r io.Reader) (int64, error) {
	// Do our best to try to retrieve the underlying *os.File if one exists
	// because the copy between files is optimized by os.(*File).ReadFrom to
	// use copy_file_range on linux.
	if f2, ok := r.(interface{ Sys() any }); ok {
		if rr, ok := f2.Sys().(io.Reader); ok {
			return f.File.ReadFrom(rr)
		}
	}
	return io.Copy(f.File, r)
}

func (f dirFile) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	osFile, err := f.openFile(name, flags, perm)
	if err != nil {
		return nil, err
	}
	return NewFile(dirFile{osFile}, name), nil
}
