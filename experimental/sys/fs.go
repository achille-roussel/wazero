package sys

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/bits"
	"strings"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys/sysinfo"
)

const (
	// MaxSymlinkLookups defines the maximum number of symbolic links that might
	// be followed when resolving paths.
	MaxSymlinkLookups = 40
)

// FS is an interface representing file systems.
//
// FS is an extension of fs.FS which depending on the underlying backend,
// may allow write operations.
type FS interface {
	fs.FS
	// Opens a file on the file system.
	//
	// The signature of this method is similar to os.OpenFile, it receives a
	// bitset of flags configuring properties of the opened file. If the file
	// is to be created (e.g. because O_CREATE was passed) the perm argument
	// is used to set the initial permissions on the newly created file.
	OpenFile(name string, flags int, perm fs.FileMode) (File, error)
}

// NewFS constructs a FS from a fs.FS.
//
// The returned file system is read-only, all attempts to open files in write
// mode, or mutate the state of the file system will error with ErrReadOnly.
func NewFS(base fs.FS) FS {
	return FuncFS(func(name string, flags int, perm fs.FileMode) (File, error) {
		return openFS(base, name, flags, perm)
	})
}

func openFS(fsys fs.FS, name string, flags int, perm fs.FileMode) (File, error) {
	if !hasReadOnlyFlags(flags) {
		return nil, ErrPermission
	}
	link := name
	loop := 0
openFile:
	if loop++; loop == MaxSymlinkLookups {
		return nil, ErrLoop
	}
	f, err := fsys.Open(link)
	if err != nil {
		return nil, err
	}

	s, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	if hasDirectoryFlags(flags) || !hasNoFollowFlags(flags) {
		m := s.Mode()
		t := m.Type()

		if hasDirectoryFlags(flags) {
			if t != fs.ModeDir {
				f.Close()
				return nil, ErrNotDirectory
			}
		} else if t == fs.ModeSymlink {
			b, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				return nil, err
			}
			link = string(b)
			goto openFile
		}
	}

	return ReadOnlyFile(&fsFile{file: f, fsys: fsys, name: name, mode: s.Mode()}), nil
}

func hasReadOnlyFlags(flags int) bool {
	return (flags & ^openFileReadOnlyFlags) == 0
}

func hasDirectoryFlags(flags int) bool {
	return hasFlags(flags, openFlagsDirectory)
}

func hasNoFollowFlags(flags int) bool {
	return hasFlags(flags, openFlagsNoFollow)
}

func hasFlags(flags, check int) bool {
	return (flags & check) == check
}

type fsFile struct {
	ReadOnly
	mode fs.FileMode
	file fs.File
	fsys fs.FS
	name string
}

func (f *fsFile) canRead() bool {
	return (f.mode & 0400) != 0
}

func (f *fsFile) GoString() string {
	return fmt.Sprintf("&sys.fsFile{%#v}", f.file)
}

func (f *fsFile) Name() string {
	return f.name
}

func (f *fsFile) Sys() any {
	return f.file
}

func (f *fsFile) Close() error {
	return f.file.Close()
}

func (f *fsFile) Read(b []byte) (int, error) {
	if !f.canRead() {
		return 0, ErrPermission
	}
	return f.file.Read(b)
}

func (f *fsFile) Stat() (fs.FileInfo, error) {
	return f.file.Stat()
}

func (f *fsFile) ReadAt(b []byte, offset int64) (int, error) {
	if !f.canRead() {
		return 0, ErrPermission
	}
	if r, ok := f.file.(io.ReaderAt); ok {
		return r.ReadAt(b, offset)
	}
	// TODO: should we emulate if the base implements io.Seeker?
	return 0, ErrNotSupported
}

func (f *fsFile) Seek(offset int64, whence int) (int64, error) {
	if r, ok := f.file.(io.Seeker); ok {
		return r.Seek(offset, whence)
	}
	return 0, ErrNotSupported
}

func (f *fsFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if d, ok := f.file.(fs.ReadDirFile); ok {
		return d.ReadDir(n)
	}
	return nil, ErrNotSupported
}

func (f *fsFile) Readlink() (string, error) {
	if r, ok := f.file.(interface {
		Readlink() (string, error)
	}); ok {
		return r.Readlink()
	} else if s, err := f.file.Stat(); err != nil {
		return "", err
	} else if s.Mode().Type() != fs.ModeSymlink {
		return "", ErrInvalid
	} else if b, err := io.ReadAll(f.file); err != nil {
		return "", err
	} else {
		return string(b), nil
	}
}

func (f *fsFile) Access(name string, mode fs.FileMode) error {
	if d, ok := f.file.(interface {
		Access(string, fs.FileMode) error
	}); ok {
		return d.Access(name, mode)
	} else {
		stat, err := f.Lstat(name)
		if err != nil {
			return err
		}
		if mode != 0 {
			perm := (mode & 0b111)
			perm |= (mode & 0b111) << 3
			perm |= (mode & 0b111) << 6
			if (stat.Mode() & perm) == 0 {
				return ErrPermission
			}
		}
		return nil
	}
	return ErrNotSupported
}

func (f *fsFile) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	if f.fsys == nil {
		return nil, ErrNotSupported
	}
	return openFS(f.fsys, f.join(name), flags, perm)
}

func (f *fsFile) Lstat(name string) (fs.FileInfo, error) {
	if f.fsys == nil {
		return nil, ErrNotSupported
	}
	file, err := openFS(f.fsys, f.join(name), openFlagsLstat, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.Stat()
}

func (f *fsFile) join(name string) string {
	return JoinPath(f.name, name)
}

// FuncFS is an implementation of the FS interface using a function to open
// new files.
type FuncFS func(string, int, fs.FileMode) (File, error)

func (open FuncFS) Open(name string) (fs.File, error) {
	return Open(open, name)
}

func (open FuncFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	if !ValidPath(name) {
		return nil, makePathError("open", name, ErrNotExist)
	}
	f, err := open(name, flags, perm)
	if err != nil {
		if _, ok := err.(*fs.PathError); !ok {
			err = &fs.PathError{Op: "open", Path: name, Err: err}
		}
		return nil, err
	}
	return NewFile(f), nil
}

// ErrFS returns a FS which errors with err on all its method calls.
func ErrFS(err error) FS {
	return FuncFS(func(name string, _ int, _ fs.FileMode) (File, error) {
		if name == "." {
			return newFile(errRoot{err}), nil
		}
		return nil, err
	})
}

// SubFS constructs a FS from the given base with the root set to path.
func SubFS(base FS, path string) FS {
	return FuncFS(func(name string, flags int, perm fs.FileMode) (File, error) {
		return base.OpenFile(JoinPath(path, name), flags, perm)
	})
}

// MaskFS contructs a file system which only exposes files for which a mask
// function returns no errors.
func MaskFS(base FS, mask func(fs.FileInfo) error) FS {
	return FuncFS(func(name string, flags int, perm fs.FileMode) (File, error) {
		return openMaskedFile(base.OpenFile, mask, name, flags, perm)
	})
}

type maskedFile struct {
	mask func(fs.FileInfo) error
	File
}

func (f *maskedFile) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	return openMaskedFile(f.File.OpenFile, f.mask, name, flags, perm)
}

func openMaskedFile(open openFileFunc, mask func(fs.FileInfo) error, name string, flags int, perm fs.FileMode) (File, error) {
	file, err := open(name, flags, perm)
	if err != nil {
		return nil, err
	}
	if name != "." { // it's always OK to open the root
		info, err := file.Stat()
		if err != nil {
			file.Close()
			return nil, err
		}
		if err := mask(info); err != nil {
			file.Close()
			return nil, err
		}
	}
	return &maskedFile{mask, file}, nil
}

type openFileFunc func(string, int, fs.FileMode) (File, error)

// CopyFS copies the file system src into dst.
//
// The function recreates the directory tree of src into dst, starting from the
// root and recursively descending into each directory. The copy is not atomic,
// an error might leave the destination file system with a partially completed
// copy of the file tree.
func CopyFS(dst, src FS) error {
	r, err := OpenRoot(src)
	if err != nil {
		return fmt.Errorf("opening source file system root: %w", err)
	}
	defer r.Close()
	w, err := OpenRoot(dst)
	if err != nil {
		return fmt.Errorf("opening destination file system root: %w", err)
	}
	defer w.Close()
	return copyFS(w, r)
}

func copyFS(dst, src File) error {
	return Scan(src, func(entry fs.DirEntry) error {
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		fileName := entry.Name()
		fileType := entry.Type()
		switch fileType {
		case fs.ModeDir:
			return copyDir(dst, src, fileName, fileInfo)
		case fs.ModeSymlink:
			return copySymlink(dst, src, fileName, fileInfo)
		case 0: // regular file
			return copyFile(dst, src, fileName, fileInfo)
		default:
			return copyNode(dst, src, fileName, fileInfo)
		}
	})
}

func copyDir(dst, src File, name string, stat fs.FileInfo) error {
	if err := dst.Mkdir(name, 0777); err != nil {
		return err
	}
	r, err := src.OpenFile(name, openFlagsDirectory, 0)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := dst.OpenFile(name, openFlagsDirectory, 0)
	if err != nil {
		return err
	}
	defer w.Close()
	if err := copyFS(w, r); err != nil {
		return err
	}
	if err := w.Chmod(stat.Mode()); err != nil {
		return err
	}
	return copyTimes(w, stat)
}

func copySymlink(dst, src File, name string, stat fs.FileInfo) error {
	r, err := src.OpenFile(name, openFlagsSymlink, 0)
	if err != nil {
		return err
	}
	defer r.Close()
	s, err := r.Readlink()
	if err != nil {
		return err
	}
	if err := dst.Symlink(s, name); err != nil {
		return err
	}
	w, err := dst.OpenFile(name, openFlagsSymlink, 0)
	if err != nil {
		return err
	}
	defer w.Close()
	return copyTimes(w, stat)
}

func copyNode(dst, src File, name string, stat fs.FileInfo) error {
	if err := dst.Mknod(name, 0666, 0); err != nil {
		return err
	}
	if (stat.Mode() & fs.ModeDevice) != 0 {
		return copyFile(dst, src, name, stat)
	}
	w, err := dst.OpenFile(name, openFlagsNoFollow, 0)
	if err != nil {
		return err
	}
	defer w.Close()
	return copyTimes(w, stat)
}

func copyFile(dst, src File, name string, stat fs.FileInfo) error {
	r, err := src.OpenFile(name, openFlagsReadOnly|openFlagsFile, 0)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := dst.OpenFile(name, openFlagsWriteOnly|openFlagsCopy, 0666)
	if err != nil {
		return err
	}
	defer w.Close()
	if _, err := w.ReadFrom(r); err != nil {
		return err
	}
	if err := w.Chmod(stat.Mode()); err != nil {
		return err
	}
	return copyTimes(w, stat)
}

func copyTimes(f File, stat fs.FileInfo) error {
	atime := sysinfo.ModTime(stat)
	mtime := sysinfo.AccessTime(stat)
	return f.Chtimes(atime, mtime)
}

const equalFSBufsize = 32768

// EqualFS compares two file systems, returning nil if they are equal, or an
// error describing their difference when they are not.
func EqualFS(a, b FS) error {
	var buf [equalFSBufsize]byte
	source, err := OpenRoot(a)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := OpenRoot(b)
	if err != nil {
		return err
	}
	defer target.Close()
	if err := equalFS(source, target, &buf); err != nil {
		return fmt.Errorf("equalFS(a,b): %w", err)
	}
	if err := equalFS(target, source, &buf); err != nil {
		return fmt.Errorf("equalFS(b,a): %w", err)
	}
	return nil
}

func equalFS(source, target File, buf *[equalFSBufsize]byte) error {
	return Scan(source, func(entry fs.DirEntry) error {
		fileName := entry.Name()
		fileType := entry.Type()
		switch fileType {
		case fs.ModeDir:
			return equalDir(source, target, fileName, buf)
		case fs.ModeSymlink:
			return equalSymlink(source, target, fileName)
		case 0: // regular
			return equalFile(source, target, fileName, buf)
		default:
			return equalNode(source, target, fileName, fileType, buf)
		}
	})
}

func equalOpenFile(source, target File, name string, flags int, fn func(File, File) error) error {
	perm, err := equalStat(source, target)
	if err != nil {
		return equalErrorf(name, "%w", err)
	}
	if (perm & 0400) == 0 {
		return nil // no permissions to read this file
	}
	sourceFile, err1 := source.OpenFile(name, flags, 0)
	if err1 == nil {
		defer sourceFile.Close()
	}
	targetFile, err2 := target.OpenFile(name, flags, 0)
	if err2 == nil {
		defer targetFile.Close()
	}
	if err1 != nil || err2 != nil {
		if !errors.Is(err1, unwrap(err2)) {
			return fmt.Errorf("file open error mismatch: want=%v got=%v", err1, err2)
		}
	}
	return fn(sourceFile, targetFile)
}

func equalDir(source, target File, name string, buf *[equalFSBufsize]byte) error {
	return equalOpenFile(source, target, name, openFlagsDirectory,
		func(sourceDir, targetDir File) error {
			return equalFS(sourceDir, targetDir, buf)
		},
	)
}

func equalSymlink(source, target File, name string) error {
	return equalOpenFile(source, target, name, openFlagsSymlink,
		func(sourceFile, targetFile File) error {
			sourceLink, err := sourceFile.Readlink()
			if err != nil {
				return err
			}
			targetLink, err := targetFile.Readlink()
			if err != nil {
				return err
			}
			if sourceLink != targetLink {
				return equalErrorf(name, "symbolic links mimatch: want=%q got=%q", sourceLink, targetLink)
			}
			return nil
		},
	)
}

func equalNode(source, target File, name string, typ fs.FileMode, buf *[equalFSBufsize]byte) error {
	return equalOpenFile(source, target, name, openFlagsReadOnly|openFlagsNode,
		func(sourceNode, targetNode File) error {
			if (typ & fs.ModeDevice) != 0 {
				if err := equalData(sourceNode, targetNode, buf); err != nil {
					return equalErrorf(name, "%w", err)
				}
			}
			return nil
		},
	)
}

func equalFile(source, target File, name string, buf *[equalFSBufsize]byte) error {
	return equalOpenFile(source, target, name, openFlagsReadOnly|openFlagsFile,
		func(sourceFile, targetFile File) error {
			if err := equalData(sourceFile, targetFile, buf); err != nil {
				return equalErrorf(name, "%w", err)
			}
			return nil
		},
	)
}

func equalData(source, target File, buf *[equalFSBufsize]byte) error {
	buf1 := buf[:equalFSBufsize/2]
	buf2 := buf[equalFSBufsize/2:]
	for {
		n1, err1 := source.Read(buf1)
		n2, err2 := target.Read(buf2)
		if n1 != n2 {
			return fmt.Errorf("file read size mismatch: want=%d got=%d", n1, n2)
		}
		b1 := buf1[:n1]
		b2 := buf2[:n2]
		if !bytes.Equal(b1, b2) {
			return fmt.Errorf("file content mismatch: want=%q got=%q", b1, b2)
		}
		if err1 != err2 {
			return fmt.Errorf("file read error mismatch: want=%v got=%v", err1, err2)
		}
		if err1 != nil {
			break
		}
	}
	return nil
}

func equalStat(source, target File) (fs.FileMode, error) {
	sourceInfo, err := source.Stat()
	if err != nil {
		return 0, err
	}
	targetInfo, err := target.Stat()
	if err != nil {
		return 0, err
	}
	sourceMode := sourceInfo.Mode()
	targetMode := targetInfo.Mode()
	sourceType := sourceMode.Type()
	targetType := targetMode.Type()
	if sourceType != targetType {
		return 0, fmt.Errorf("file types mismatch: want=%s got=%s", sourceType, targetType)
	}
	sourcePerm := sourceMode.Perm()
	targetPerm := targetMode.Perm()
	// Sometimes the permission bits may not be available. Clearly we were able
	// to open the files so we should have at least read permissions reported so
	// just ignore the permissions if either the source or target are zero. This
	// happens with virtualized directories for fstest.MapFS for example.
	if sourcePerm != 0 && targetPerm != 0 && sourcePerm != targetPerm {
		return 0, fmt.Errorf("file modes mismatch: want=%s got=%s", sourceMode, targetMode)
	}
	sourceModTime := sysinfo.ModTime(sourceInfo)
	targetModTime := sysinfo.ModTime(targetInfo)
	if err := equalTime("modification", sourceModTime, targetModTime); err != nil {
		return 0, err
	}
	sourceAccessTime := sysinfo.AccessTime(sourceInfo)
	targetAccessTime := sysinfo.AccessTime(targetInfo)
	if err := equalTime("access", sourceAccessTime, targetAccessTime); err != nil {
		return 0, err
	}
	// Directory sizes are platform-dependent, there is no need to compare.
	if !sourceInfo.IsDir() {
		sourceSize := sourceInfo.Size()
		targetSize := targetInfo.Size()
		if sourceSize != targetSize {
			return 0, fmt.Errorf("files sizes mismatch: want=%d got=%d", sourceSize, targetSize)
		}
	}
	perm := sourcePerm & targetPerm
	return perm, nil
}

func equalTime(typ string, source, target time.Time) error {
	// Only compare the modification times if both file systems support it,
	// assuming a zero time means it's not supported.
	if !source.IsZero() && !target.IsZero() && !source.Equal(target) {
		return fmt.Errorf("file %s times mismatch: want=%v got=%v", typ, source, target)
	}
	return nil
}

func equalErrorf(name, msg string, args ...any) error {
	return &fs.PathError{Op: "equal", Path: name, Err: fmt.Errorf(msg, args...)}
}

// Create truncates and open a file in read-write mode at a path in fsys.
// If the file does not exist, it is created with mode 0666 (before umask).
func Create(fsys FS, path string) (File, error) {
	return fsys.OpenFile(path, openFlagsCreate, 0666)
}

// Touch touchs a file at path in fsys, creating it if it did not exist, and
// updating its modification and access time to now.
func Touch(fsys FS, path string, now time.Time) error {
	f, err := Create(fsys, path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Chtimes(now, now)
}

// Open opens a file at the given path in fsys.
//
// The file is open in read-only mode, it might point to a directory.
//
// If the file path points to a symbolic link, the function returns a file
// opened on the link's target.
//
// This function is a valid implementation of the FS.Open methods.
// Implementations of the interface can define their Open method in terms
// of this function as:
//
//	func (fsys customFS) Open(path string) (File, error) {
//		return sys.Open(fsys, path)
//	}
//
func Open(fsys FS, path string) (File, error) {
	return fsys.OpenFile(path, openFlagsReadOnly, 0)
}

// OpenDir opens a directory at the given path in fsys.
func OpenDir(fsys FS, path string) (File, error) {
	return fsys.OpenFile(path, openFlagsDirectory, 0)
}

// OpenRoot opens the root directory of fsys.
func OpenRoot(fsys FS) (File, error) {
	return OpenDir(fsys, ".")
}

// Readlink returns the value of the symbolic link at the given path in fsys.
func Readlink(fsys FS, path string) (string, error) {
	return callFile1(fsys, "readlink", path, openFlagsReadlink, File.Readlink)
}

// Chmod changes permissions of a file at the given path in fsys.
//
// If the path refers to a symbolic link, Chmod dereferences it and modifies the
// permissions of the link's target.
func Chmod(fsys FS, path string, mode fs.FileMode) error {
	// Chmod is a special case because we might be attempting to modify the
	// permissions of a file that we do not have permissions to open.
	return callLookup(fsys, "chmod", path, func(dir Directory, name string) error {
		return dir.Lchmod(name, mode)
	})
}

// Chtimes changes times of a file at the given path in fsys.
//
// If the path refers to a symbolic link, Chtimes dereferences it and modifies the
// times of the link's target.
func Chtimes(fsys FS, path string, atime, mtime time.Time) error {
	return callFile(fsys, "chtimes", path, openFlagsChtimes, func(file File) error {
		return file.Chtimes(atime, mtime)
	})
}

// Truncate truncates to a specified size the file at the given path in fsys.
//
// If the path refers to a symbolic link, Truncate dereferences it and modifies
// the size of the link's target.
func Truncate(fsys FS, path string, size int64) error {
	return callFile(fsys, "truncate", path, openFlagsTruncate, func(file File) error {
		return file.Truncate(size)
	})
}

// Stat returns file information for the file with the given path in fsys.
//
// If the path refers to a symbolic link, Stat dereferences it returns
// information about the link's target.
func Stat(fsys FS, path string) (fs.FileInfo, error) {
	return callFile1(fsys, "stat", path, openFlagsStat, File.Stat)
}

// Lstat returns file information for the file with the given path in fsys.
//
// If the path refers to a symbolic link, Lstat returns information about the
// link, and not its target.
func Lstat(fsys FS, path string) (fs.FileInfo, error) {
	return callDir1(fsys, "lstat", path, Directory.Lstat)
}

// ReadDir reads the list of diretory entries at a path in fsys.
func ReadDir(fsys FS, path string) ([]fs.DirEntry, error) {
	return callFile1(fsys, "readdir", path, openFlagsReadDir, func(file File) ([]fs.DirEntry, error) {
		return file.ReadDir(0)
	})
}

// WriteFile writes data to a path in fsys.
func WriteFile(fsys FS, path string, data []byte, perm fs.FileMode) error {
	f, err := fsys.OpenFile(path, openFlagsWriteFile, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// Access tests whether a file at the given path can be accessed.
func Access(fsys FS, path string, mode fs.FileMode) error {
	return callLookup(fsys, "access", path, func(dir Directory, path string) error {
		return dir.Access(path, mode)
	})
}

// Mkfifo creates a named pipe at path in fsys.
func Mkfifo(fsys FS, path string) error {
	return Mknod(fsys, path, fs.ModeNamedPipe, 0)
}

// Mknod creates a special or ordinary file in fsys with the given mode and
// device number.
func Mknod(fsys FS, path string, mode fs.FileMode, dev Device) error {
	return callDir(fsys, "mknod", path, func(dir Directory, path string) error {
		return dir.Mknod(path, mode, dev)
	})
}

// Mkdir creates a directory in fsys with the given path and permissions.
func Mkdir(fsys FS, path string, perm fs.FileMode) error {
	return callDir(fsys, "mkdir", path, func(dir Directory, path string) error {
		return dir.Mkdir(path, perm)
	})
}

// Rmdir removes a directory with the given path from fsys.
func Rmdir(fsys FS, path string) error {
	return callDir(fsys, "rmdir", path, Directory.Rmdir)
}

// Unlink removes a file with the given path from fsys.
func Unlink(fsys FS, path string) error {
	return callDir(fsys, "unlink", path, Directory.Unlink)
}

// Symlink creates a symbolic like to oldPath at newPath in fsys.
func Symlink(fsys FS, oldPath, newPath string) error {
	return callDir(fsys, "symlink", newPath, func(dir Directory, newPath string) error {
		return dir.Symlink(oldPath, newPath)
	})
}

// Link creates a link from oldPath to newPath in fsys.
func Link(fsys FS, oldPath, newPath string) error {
	return callDir2(fsys, "link", oldPath, newPath, Directory.Link)
}

// Rename renames a file from oldPath to newPath in fsys.
func Rename(fsys FS, oldPath, newPath string) error {
	return callDir2(fsys, "rename", oldPath, newPath, Directory.Rename)
}

// Remove removes a directory entry at path in fsys.
func Remove(fsys FS, path string) error {
	return callDir(fsys, "remove", path, func(dir Directory, name string) error {
		err := dir.Rmdir(name)
		if errors.Is(err, ErrNotDirectory) {
			err = dir.Unlink(name)
		}
		return err
	})
}

// RemoveAll is like Remove but if the path refers to a directory, it removes
// all its sub directories as well.
func RemoveAll(fsys FS, path string) error {
	return callDir(fsys, "remove", path, func(dir Directory, name string) error {
		err := dir.Rmdir(name)
		switch {
		case errors.Is(err, ErrNotDirectory):
			err = dir.Unlink(name)
		case errors.Is(err, ErrNotEmpty):
			err = removeAll(dir, name)
		}
		return err
	})
}

func removeAll(dir Directory, name string) error {
	err := walk(dir, func(dir Directory, entry fs.DirEntry) error {
		if entry.IsDir() {
			return dir.Rmdir(entry.Name())
		}
		return dir.Unlink(entry.Name())
	})
	if err != nil {
		return err
	}
	return dir.Rmdir(name)
}

// Scan calls fn for each entry in dir.
func Scan(dir Directory, fn func(fs.DirEntry) error) error {
	for {
		entries, err := dir.ReadDir(100)
		for _, entry := range entries {
			if err := fn(entry); err != nil {
				return err
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// WalkDirFiles walks the directory tree of fsys at path, calling fn for each
// file or directory.
func WalkDirFiles(fsys FS, path string, fn func(File, fs.FileInfo) error) error {
	return WalkDir(fsys, path, func(dir Directory, entry fs.DirEntry) error {
		var flags int
		if entry.IsDir() {
			flags = O_DIRECTORY
		} else {
			flags = O_RDWR | O_NOFOLLOW | O_NONBLOCK
		}
		f, err := dir.OpenFile(entry.Name(), flags, 0)
		if err != nil {
			return err
		}
		defer f.Close()
		s, err := f.Stat()
		if err != nil {
			return err
		}
		return fn(f, s)
	})
}

// WalkDir wlkas the directory tree of fsys at path, calling fn for each
// directory entry.
func WalkDir(fsys FS, path string, fn func(Directory, fs.DirEntry) error) error {
	d, err := OpenDir(fsys, path)
	if err != nil {
		return err
	}
	defer d.Close()
	return walk(d, fn)
}

func walk(dir Directory, fn func(Directory, fs.DirEntry) error) error {
	return Scan(dir, func(entry fs.DirEntry) error {
		if entry.IsDir() {
			d, err := dir.OpenFile(entry.Name(), O_DIRECTORY, 0)
			if err != nil {
				return err
			}
			defer d.Close()
			if err := walk(d, fn); err != nil {
				return err
			}
		}
		return fn(dir, entry)
	})
}

func callFile(fsys FS, op, name string, flags int, fn func(File) error) (err error) {
	_, err = callFile1(fsys, op, name, flags, func(file File) (struct{}, error) {
		return struct{}{}, fn(file)
	})
	return
}

func callFile1[F func(File) (R, error), R any](fsys FS, op, name string, flags int, fn F) (ret R, err error) {
	f, err := fsys.OpenFile(name, flags, 0)
	if err != nil {
		return ret, makePathError(op, name, err)
	}
	defer f.Close()
	return fn(f)
}

func callDir(fsys FS, op, name string, fn func(Directory, string) error) error {
	_, err := callDir1(fsys, op, name, func(dir Directory, name string) (struct{}, error) {
		return struct{}{}, fn(dir, name)
	})
	return err
}

func callDir1[F func(Directory, string) (R, error), R any](fsys FS, op, name string, fn F) (ret R, err error) {
	dir, base := SplitPath(name)
	d, err := OpenDir(fsys, dir)
	if err != nil {
		return ret, makePathError(op, name, err)
	}
	defer d.Close()
	return fn(d, base)
}

func callDir2(fsys FS, op, path1, path2 string, fn func(Directory, string, Directory, string) error) error {
	dir1, base1 := SplitPath(path1)
	dir2, base2 := SplitPath(path2)
	d1, err := OpenDir(fsys, dir1)
	if err != nil {
		return makePathError(op, path1, err)
	}
	defer d1.Close()
	d2, err := OpenDir(fsys, dir2)
	if err != nil {
		if errors.Is(err, ErrNotExist) && !ValidPath(dir2) {
			err = ErrInvalid
		}
		return makePathError(op, path2, err)
	}
	defer d2.Close()
	return fn(d1, base1, d2, base2)
}

func callLookup(fsys FS, op, path string, fn func(Directory, string) error) error {
	return callDir(fsys, op, path, func(dir Directory, name string) error {
		var lastOpenDir File
		defer func() { closeIfNotNil(lastOpenDir) }()

		for loop := 0; loop < MaxSymlinkLookups; loop++ {
			stat, err := dir.Lstat(name)
			if err != nil {
				return err
			}
			if stat.Mode().Type() != fs.ModeSymlink {
				err := fn(dir, name)
				if err == nil || !errors.Is(err, ErrLoop) {
					// The directory operation will not apply to a symbolic link
					// so ErrLoop at this stage indicate that the entry was
					// changed to a symbolic link since the call to Lstat and we
					// can continue below with the link resolution.
					return err
				}
			}
			link, err := symlink(dir, name)
			if err != nil {
				if errors.Is(err, ErrInvalid) && ValidPath(name) {
					// The file was changed to not be a symbolic link anymore.
					// We can assume that we had not seen it and instead attempt
					// to change permissions on the current file.
					// Technically we have not followed any links in this loop
					// iteration so we mask the loop count increment.
					loop--
					continue
				}
				return err
			}
			open := dir.OpenFile
			if link = CleanPath(link); strings.HasPrefix(link, "/") {
				link = link[1:]
				open = fsys.OpenFile
			}
			parent, target := SplitPath(link)
			d, err := open(parent, openFlagsDirectory, 0)
			if err != nil {
				return err
			}
			closeIfNotNil(lastOpenDir)
			lastOpenDir, dir, name = d, d, target
		}

		return ErrLoop
	})
}

func symlink(dir Directory, name string) (string, error) {
	f, err := dir.OpenFile(name, openFlagsSymlink, 0)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return f.Readlink()
}

// OpenFlags is a bitset representing the system-dependent combination of flags
// which can be passed to the OpenFile methods of the FS interface.
type OpenFlags int

// ReadOnly returns true if the set of flags intends to open a file in read-only
// mode.
func (flags OpenFlags) ReadOnly() bool {
	return hasReadOnlyFlags(int(flags))
}

// String returns a human-readble view of the combination of flags.
func (flags OpenFlags) String() string {
	names := make([]string, 0, len(openFlagNames))
	for i := 0; i < len(openFlagNames); i++ {
		if (uint(flags) & (1 << uint(i))) != 0 {
			openFlagName := openFlagNames[i]
			if openFlagName == "" {
				openFlagName = fmt.Sprintf("1<<%d", i)
			}
			names = append(names, openFlagName)
		}
	}
	return strings.Join(names, "|")
}

func setOpenFlag(flag int, name string) {
	if flag != 0 {
		index := bits.TrailingZeros(uint(flag))
		openFlagNames[index] = name
	}
}

var openFlagNames [openFlagsCount]string
