package wasi

import (
	"errors"
	"io"
	"io/fs"
	fspath "path"
)

const (
	O_CREAT     = 1 << 0
	O_DIRECTORY = 1 << 1
	O_EXCL      = 1 << 2
	O_TRUNC     = 1 << 3
)

const (
	MaxPathLen = 1024
)

const (
	pathCreateDirectory = "path_create_directory"
	pathLink            = "path_link"
	pathOpen            = "path_open"
	pathReadlink        = "path_readlink"
	pathRemoveDirectory = "path_remove_directory"
	pathRename          = "path_rename"
	pathFilestatGet     = "path_filestat_get"
	pathSymlink         = "path_symlink"
	pathUnilnkFile      = "path_unlink_file"
)

type FS interface {
	fs.StatFS

	PathCreateDirectory(path string) error

	PathLink(oldPath, newPath string) error

	PathOpen(path string, oflags int) (File, error)

	PathReadlink(path string) (string, error)

	PathRemoveDirectory(path string) error

	PathRename(oldPath, newPath string) error

	PathStat(path string) (fs.FileInfo, error)

	PathSymlink(oldPath, newPath string) error

	PathUnlinkFile(path string) error
}

func NewFS(base fs.FS) FS { return &fsFileSystem{base} }

type fsFileSystem struct{ base fs.FS }

func (fsys *fsFileSystem) Open(path string) (fs.File, error) {
	f, err := fsys.base.Open(path)
	if err != nil {
		return nil, err
	}
	s, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if s.IsDir() {
		return &fsDirectory{baseFile{path}, f, fsys}, nil
	}
	return &fsFile{baseFile{path}, f}, nil
}

func (fsys *fsFileSystem) Stat(path string) (fs.FileInfo, error) {
	if statfs, ok := fsys.base.(fs.StatFS); ok {
		return statfs.Stat(path)
	}
	f, err := fsys.base.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

func (fsys *fsFileSystem) PathCreateDirectory(path string) error {
	return makePathError(pathCreateDirectory, path, fs.ErrPermission)
}

func (fsys *fsFileSystem) PathLink(oldPath, newPath string) error {
	return makePathError(pathLink, oldPath, fs.ErrPermission)
}

func (fsys *fsFileSystem) PathOpen(path string, oflags int) (File, error) {
	if (oflags & (O_CREAT | O_TRUNC)) != 0 {
		return nil, makePathError(pathOpen, path, fs.ErrPermission)
	}
	cleanPath, err := cleanPath(path)
	if err != nil {
		return nil, makePathError(pathOpen, path, err)
	}
	f, err := fsys.Open(cleanPath)
	if err != nil {
		return nil, makePathError(pathOpen, path, err)
	}
	switch file := f.(type) {
	case *fsFile:
		if (oflags & O_DIRECTORY) != 0 {
			file.Close()
			return nil, makePathError(pathOpen, path, fs.ErrInvalid)
		}
		return file, nil
	case *fsDirectory:
		if (oflags & O_DIRECTORY) == 0 {
			file.Close()
			return nil, makePathError(pathOpen, path, fs.ErrInvalid)
		}
		return file, nil
	default:
		panic("BUG: impossible type returned by Open")
	}
}

func (fsys *fsFileSystem) PathReadlink(path string) (string, error) {
	cleanPath, err := cleanPath(path)
	if err != nil {
		return "", makePathError(pathReadlink, path, err)
	}
	f, err := fsys.base.Open(cleanPath)
	if err != nil {
		return "", makePathError(pathReadlink, path, err)
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxPathLen))
	if err != nil {
		return "", makePathError(pathReadlink, path, err)
	}
	return string(b), nil
}

func (fsys *fsFileSystem) PathRemoveDirectory(path string) error {
	return makePathError(pathRemoveDirectory, path, fs.ErrPermission)
}

func (fsys *fsFileSystem) PathRename(oldPath, newPath string) error {
	return makePathError(pathRename, oldPath, fs.ErrPermission)
}

func (fsys *fsFileSystem) PathStat(path string) (fs.FileInfo, error) {
	cleanPath, err := cleanPath(path)
	if err != nil {
		return nil, err
	}
	s, err := fsys.Stat(cleanPath)
	if err != nil {
		return nil, makePathError(pathFilestatGet, path, err)
	}
	return s, nil
}

func (fsys *fsFileSystem) PathSymlink(oldPath, newPath string) error {
	return makePathError(pathSymlink, oldPath, fs.ErrPermission)
}

func (fsys *fsFileSystem) PathUnlinkFile(path string) error {
	return makePathError(pathUnilnkFile, path, fs.ErrPermission)
}

func cleanPath(path string) (string, error) {
	path = fspath.Clean(path)
	if len(path) == 0 || path[0] != '/' {
		return "", fs.ErrInvalid
	}
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	if len(path) == 0 {
		return ".", nil
	}
	return path, nil
}

type File interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Writer
	io.WriterAt
	fs.ReadDirFile

	Stat() (fs.FileInfo, error)

	Sync() error

	PathCreateDirectory(path string) error

	PathLink(oldPath, newPath string) error

	PathOpen(path string, oflags int) (File, error)

	PathReadlink(path string) (string, error)

	PathRemoveDirectory(path string) error

	PathRename(oldPath, newPath string) error

	PathStat(path string) (fs.FileInfo, error)

	PathSymlink(oldPath, newPath string) error

	PathUnlinkFile(path string) error
}

type baseFile struct{ path string }

func (baseFile) Close() error { return nil }

func (baseFile) Read([]byte) (int, error) { return 0, fs.ErrInvalid }

func (baseFile) ReadAt([]byte, int64) (int, error) { return 0, fs.ErrInvalid }

func (baseFile) Seek(int64, int) (int64, error) { return 0, fs.ErrInvalid }

func (baseFile) Write([]byte) (int, error) { return 0, fs.ErrInvalid }

func (baseFile) WriteAt([]byte, int64) (int, error) { return 0, fs.ErrInvalid }

func (baseFile) ReadDir(int) ([]fs.DirEntry, error) { return nil, fs.ErrInvalid }

func (baseFile) Stat() (fs.FileInfo, error) { return nil, fs.ErrInvalid }

func (baseFile) Sync() error { return fs.ErrInvalid }

func (f baseFile) PathCreateDirectory(path string) error {
	return f.makePathError(pathCreateDirectory, path, fs.ErrInvalid)
}

func (f baseFile) PathLink(oldPath, newPath string) error {
	return f.makePathError(pathLink, oldPath, fs.ErrInvalid)
}

func (f baseFile) PathOpen(path string, oflags int) (File, error) {
	return nil, f.makePathError(pathOpen, path, fs.ErrInvalid)
}

func (f baseFile) PathReadlink(path string) (string, error) {
	return "", f.makePathError(pathReadlink, path, fs.ErrInvalid)
}

func (f baseFile) PathRemoveDirectory(path string) error {
	return f.makePathError(pathRemoveDirectory, path, fs.ErrInvalid)
}

func (f baseFile) PathRename(oldPath, newPath string) error {
	return f.makePathError(pathRename, oldPath, fs.ErrInvalid)
}

func (f baseFile) PathStat(path string) (fs.FileInfo, error) {
	return nil, f.makePathError(pathFilestatGet, path, fs.ErrInvalid)
}

func (f baseFile) PathSymlink(oldPath, newPath string) error {
	return f.makePathError(pathSymlink, oldPath, fs.ErrInvalid)
}

func (f baseFile) PathUnlinkFile(path string) error {
	return f.makePathError(pathUnilnkFile, path, fs.ErrInvalid)
}

func (f baseFile) makePathError(op string, path string, err error) error {
	path, _ = f.makePath(path)
	return makePathError(op, path, err)
}

func (f baseFile) makePath(path string) (string, error) {
	if len(path) > MaxPathLen {
		return path, fs.ErrInvalid
	}
	absPath := fspath.Join(f.path, path)
	if len(absPath) > MaxPathLen {
		return path, fs.ErrInvalid
	}
	return absPath, nil
}

func makePathError(op, path string, err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		err = pathErr.Err
	}
	return &fs.PathError{Op: op, Path: path, Err: err}
}

type fsFile struct {
	baseFile
	base fs.File
}

func (f *fsFile) Close() error {
	return f.base.Close()
}

func (f *fsFile) Read(b []byte) (int, error) {
	return f.base.Read(b)
}

func (f *fsFile) ReadAt(b []byte, off int64) (int, error) {
	if r, ok := f.base.(io.ReaderAt); ok {
		return r.ReadAt(b, off)
	}
	return 0, fs.ErrInvalid
}

func (f *fsFile) Seek(off int64, whence int) (int64, error) {
	if r, ok := f.base.(io.Seeker); ok {
		return r.Seek(off, whence)
	}
	return 0, fs.ErrInvalid
}

func (f *fsFile) Write(b []byte) (int, error) {
	return 0, fs.ErrPermission
}

func (f *fsFile) WriteAt(b []byte, off int64) (int, error) {
	return 0, fs.ErrPermission
}

func (f *fsFile) Stat() (fs.FileInfo, error) {
	return f.base.Stat()
}

func (f *fsFile) Sync() error {
	return fs.ErrPermission
}

type fsDirectory struct {
	baseFile
	base fs.File
	fsys *fsFileSystem
}

func (d *fsDirectory) Close() error {
	return d.base.Close()
}

func (d *fsDirectory) Stat() (fs.FileInfo, error) {
	return d.base.Stat()
}

func (d *fsDirectory) PathCreateDirectory(path string) error {
	path, err := d.makePath(path)
	if err != nil {
		return err
	}
	return d.fsys.PathCreateDirectory(path)
}

func (d *fsDirectory) PathLink(oldPath, newPath string) error {
	oldPath, err := d.makePath(oldPath)
	if err != nil {
		return err
	}
	return d.fsys.PathLink(oldPath, newPath)
}

func (d *fsDirectory) PathOpen(path string, oflags int) (File, error) {
	path, err := d.makePath(path)
	if err != nil {
		return nil, err
	}
	return d.fsys.PathOpen(path, oflags)
}

func (d *fsDirectory) PathReadlink(path string) (string, error) {
	path, err := d.makePath(path)
	if err != nil {
		return "", err
	}
	return d.fsys.PathReadlink(path)
}

func (d *fsDirectory) PathRemoveDirectory(path string) error {
	path, err := d.makePath(path)
	if err != nil {
		return err
	}
	return d.fsys.PathRemoveDirectory(path)
}

func (d *fsDirectory) PathRename(oldPath, newPath string) error {
	oldPath, err := d.makePath(oldPath)
	if err != nil {
		return err
	}
	return d.fsys.PathRename(oldPath, newPath)
}

func (d *fsDirectory) PathStat(path string) (fs.FileInfo, error) {
	path, err := d.makePath(path)
	if err != nil {
		return nil, err
	}
	return d.fsys.PathStat(path)
}

func (d *fsDirectory) PathSymlink(oldPath, newPath string) error {
	oldPath, err := d.makePath(oldPath)
	if err != nil {
		return err
	}
	return d.fsys.PathSymlink(oldPath, newPath)
}

func (d *fsDirectory) PathUnlinkFile(path string) error {
	path, err := d.makePath(path)
	if err != nil {
		return err
	}
	return d.fsys.PathUnlinkFile(path)
}
