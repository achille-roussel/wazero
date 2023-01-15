package sys

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"time"
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
	return FuncFS(func(fsys FS, name string, flags int, perm fs.FileMode) (File, error) {
		if !hasReadOnlyFlags(flags) {
			return nil, ErrReadOnly
		}
		link := name
		loop := 0
	openFile:
		if loop++; loop == 40 {
			return nil, ErrLoop
		}
		f, err := base.Open(link)
		if err != nil {
			return nil, err
		}

		if ((flags & O_DIRECTORY) != 0) || ((flags & O_NOFOLLOW) == 0) {
			s, err := f.Stat()
			if err != nil {
				f.Close()
				return nil, err
			}
			m := s.Mode()
			t := m.Type()

			if (flags & O_DIRECTORY) != 0 {
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

		return ReadOnlyFile(f, name, fsys), nil
	})
}

// FuncFS is an implementation of the FS interface using a function to open
// new files.
//
// The function has a signature similar to OpenFile, but the first argument is
// the FuncFS itself as a FS value, allowing to be captured in the returned
// File (e.g. if it is constructed with ReadOnlyFile).
type FuncFS func(FS, string, int, fs.FileMode) (File, error)

func (open FuncFS) Open(name string) (fs.File, error) {
	return Open(open, name)
}

func (open FuncFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	if !ValidPath(name) {
		return nil, makePathError("open", name, ErrNotExist)
	}
	f, err := open(open, name, flags, perm)
	if err != nil {
		if name == "." {
			// The root should always be successfully opened; wrap the
			// error instead of returning it so it does not invalidation
			// this expectation.
			return NewFile(&errRoot{err}, "."), nil
		}
		if _, ok := err.(*fs.PathError); !ok {
			err = &fs.PathError{Op: "open", Path: name, Err: err}
		}
		return nil, err
	}
	return NewFile(f, name), nil
}

// ErrFS returns a FS which errors with err on all its method calls.
func ErrFS(err error) FS {
	return FuncFS(func(_ FS, _ string, _ int, _ fs.FileMode) (File, error) {
		return nil, err
	})
}

// FileFS constructs a FS instance from a base file, using the file's OpenFile
// method to navigate the file system.
func FileFS(base File) FS {
	return FuncFS(func(_ FS, name string, flags int, perm fs.FileMode) (File, error) {
		return base.OpenFile(name, flags, perm)
	})
}

// SubFS constructs a FS from the given base with the root set to path.
func SubFS(base FS, path string) FS {
	return FuncFS(func(_ FS, name string, flags int, perm fs.FileMode) (File, error) {
		return base.OpenFile(JoinPath(path, name), flags, perm)
	})
}

// MaskFS contructs a file system which only exposes files for which a mask
// function returns no errors.
func MaskFS(base FS, mask func(path string, info fs.FileInfo) error) FS {
	return FuncFS(func(_ FS, name string, flags int, perm fs.FileMode) (File, error) {
		return openMaskedFile(base.OpenFile, mask, ".", name, flags, perm)
	})
}

type maskedFile struct {
	mask maskFileFunc
	path string
	File
}

func (f *maskedFile) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	return openMaskedFile(f.File.OpenFile, f.mask, f.path, name, flags, perm)
}

type openFileFunc func(string, int, fs.FileMode) (File, error)

type maskFileFunc func(string, fs.FileInfo) error

func openMaskedFile(open openFileFunc, mask maskFileFunc, path, name string, flags int, perm fs.FileMode) (File, error) {
	path = JoinPath(path, name)
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
		if err := mask(name, info); err != nil {
			file.Close()
			return nil, err
		}
	}
	return &maskedFile{mask: mask, path: name, File: file}, nil
}

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
	for {
		entries, err := src.ReadDir(100)
		for _, entry := range entries {
			stat, err := entry.Info()
			if err != nil {
				return err
			}
			name := entry.Name()
			switch entry.Type() {
			case fs.ModeDir:
				err = copyDir(dst, src, name, stat)
			case fs.ModeSymlink:
				err = copySymlink(dst, src, name, stat)
			case fs.ModeDevice, fs.ModeDevice | fs.ModeCharDevice:
				err = copyDevice(dst, src, name, stat)
			case 0: // regular file
				err = copyFile(dst, src, name, stat)
			}
			if err != nil {
				return err
			}
		}
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return err
		}
	}
}

func copyDir(dst, src File, name string, stat fs.FileInfo) error {
	if err := dst.Mkdir(name, stat.Mode()); err != nil {
		return err
	}
	r, err := src.OpenFile(name, O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := dst.OpenFile(name, O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer w.Close()
	if err := copyFS(w, r); err != nil {
		return err
	}
	time := stat.ModTime()
	return w.Chtimes(time, time)
}

func copySymlink(dst, src File, name string, stat fs.FileInfo) error {
	r, err := src.OpenFile(name, O_RDONLY|O_NOFOLLOW, 0)
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
	w, err := dst.OpenFile(name, O_RDWR|O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer w.Close()
	time := stat.ModTime()
	return w.Chtimes(time, time)
}

func copyDevice(dst, src File, name string, stat fs.FileInfo) error {
	return dst.Mknod(name, stat.Mode(), Dev(0, 0))
}

func copyFile(dst, src File, name string, stat fs.FileInfo) error {
	r, err := src.OpenFile(name, O_RDONLY|O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := dst.OpenFile(name, O_WRONLY|O_NOFOLLOW|O_CREATE|O_TRUNC, stat.Mode())
	if err != nil {
		return err
	}
	defer w.Close()
	if _, err := io.Copy(w, r); err != nil {
		return err
	}
	time := stat.ModTime()
	return w.Chtimes(time, time)
}

const equalFSBufsize = 8192

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
	for {
		entries, err := source.ReadDir(100)
		for _, entry := range entries {
			name := entry.Name()
			switch entry.Type() {
			case fs.ModeDir:
				err = equalDir(source, target, name, buf)
			case fs.ModeSymlink:
				err = equalSymlink(source, target, name)
			case fs.ModeDevice, fs.ModeDevice | fs.ModeCharDevice:
				err = equalDevice(source, target, name)
			default:
				err = equalFile(source, target, name, buf)
			}
		}
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return err
		}
	}
}

func equalDir(source, target File, name string, buf *[equalFSBufsize]byte) error {
	sourceDir, err := source.OpenFile(name, O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer sourceDir.Close()
	targetDir, err := target.OpenFile(name, O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer targetDir.Close()
	if err := equalStat(sourceDir, targetDir); err != nil {
		return equalErrorf(name, "%w", err)
	}
	return equalFS(sourceDir, targetDir, buf)
}

func equalSymlink(source, target File, name string) error {
	sourceFile, err := source.OpenFile(name, O_RDONLY|O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	targetFile, err := target.OpenFile(name, O_RDONLY|O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer targetFile.Close()
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
}

func equalDevice(source, target File, name string) error {
	sourceDev, err := source.OpenFile(name, O_RDONLY|O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer sourceDev.Close()
	targetDev, err := target.OpenFile(name, O_RDONLY|O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer targetDev.Close()
	// TODO: compare the device numbers in a portable way
	return equalStat(sourceDev, targetDev)
}

func equalFile(source, target File, name string, buf *[equalFSBufsize]byte) error {
	sourceFile, err := source.OpenFile(name, O_RDONLY|O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	targetFile, err := target.OpenFile(name, O_RDONLY|O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer targetFile.Close()
	if err := equalStat(sourceFile, targetFile); err != nil {
		return equalErrorf(name, "%w", err)
	}
	if err := equalData(sourceFile, targetFile, buf); err != nil {
		return equalErrorf(name, "%w", err)
	}
	return nil
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

func equalStat(source, target File) error {
	sourceInfo, err := source.Stat()
	if err != nil {
		return err
	}
	targetInfo, err := target.Stat()
	if err != nil {
		return err
	}
	sourceMode := sourceInfo.Mode()
	targetMode := targetInfo.Mode()
	if sourceMode != targetMode {
		return fmt.Errorf("file modes mismatch: want=%s got=%s", sourceMode, targetMode)
	}
	sourceTime := sourceInfo.ModTime()
	targetTime := targetInfo.ModTime()
	// Only compare the modification times if both file systems support it,
	// assuming a zero time means it's not supported.
	if !sourceTime.IsZero() && !targetTime.IsZero() {
		if !sourceTime.Equal(targetTime) {
			return fmt.Errorf("file times mismatch: want=%v got=%v", sourceTime, targetTime)
		}
	}
	// Directory sizes are platform-dependent, there is no need to compare.
	if !sourceInfo.IsDir() {
		sourceSize := sourceInfo.Size()
		targetSize := targetInfo.Size()
		if sourceSize != targetSize {
			return fmt.Errorf("files sizes mismatch: want=%d got=%d", sourceSize, targetSize)
		}
	}
	return nil
}

func equalErrorf(name, msg string, args ...any) error {
	return &fs.PathError{Op: "equal", Path: name, Err: fmt.Errorf(msg, args...)}
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
	return fsys.OpenFile(path, O_RDONLY, 0)
}

// OpenDir opens a directory at the given path in fsys.
func OpenDir(fsys FS, path string) (File, error) {
	return fsys.OpenFile(path, O_DIRECTORY, 0)
}

// OpenRoot opens the root directory of fsys.
func OpenRoot(fsys FS) (File, error) {
	return OpenDir(fsys, ".")
}

// Readlink returns the value of the symbolic link at the given path in fsys.
func Readlink(fsys FS, path string) (string, error) {
	return callFile1(fsys, "readlink", path, O_RDONLY|O_NOFOLLOW, File.Readlink)
}

// Chmod changes permissions of a file at the given path in fsys.
//
// If the path refers to a symbolic link, Chmod dereferences it and modifies the
// permissions of the link's target.
func Chmod(fsys FS, path string, mode fs.FileMode) error {
	return callFile(fsys, "chmod", path, O_RDONLY, func(file File) error {
		return file.Chmod(mode)
	})
}

// Chtimes changes times of a file at the given path in fsys.
//
// If the path refers to a symbolic link, Chtimes dereferences it and modifies the
// times of the link's target.
func Chtimes(fsys FS, path string, atime, mtime time.Time) error {
	return callFile(fsys, "chtimes", path, O_RDONLY, func(file File) error {
		return file.Chtimes(atime, mtime)
	})
}

// Truncate truncates to a specified size the file at the given path in fsys.
//
// If the path refers to a symbolic link, Truncate dereferences it and modifies
// the size of the link's target.
func Truncate(fsys FS, path string, size int64) error {
	return callFile(fsys, "truncate", path, O_WRONLY, func(file File) error {
		return file.Truncate(size)
	})
}

// Stat returns file information for the file with the given path in fsys.
//
// If the path refers to a symbolic link, Stat dereferences it returns
// information about the link's target.
func Stat(fsys FS, path string) (fs.FileInfo, error) {
	return callFile1(fsys, "stat", path, O_RDONLY, File.Stat)
}

// Lstat returns file information for the file with the given path in fsys.
//
// If the path refers to a symbolic link, Lstat returns information about the
// link, and not its target.
func Lstat(fsys FS, path string) (fs.FileInfo, error) {
	return callFile1(fsys, "lstat", path, O_RDONLY|O_NOFOLLOW, File.Stat)
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

// GetXAttr retrieves the value of the named extended attribute for the file at
// the given path.
func GetXAttr(fsys FS, path, name string) (string, bool, error) {
	return callFile2(fsys, "getxattr", path, O_RDONLY, func(file File) (string, bool, error) {
		return file.GetXAttr(name)
	})
}

// SetXAttr sets the value of the named extended attribute for the file at the
// given path.
func SetXAttr(fsys FS, path, name, value string, flags int) error {
	return callFile(fsys, "setxattr", path, O_RDONLY, func(file File) error {
		return file.SetXAttr(name, value, flags)
	})
}

// ListXAttr returns the names of the named extended attribute for the file at
// the given path.
func ListXAttr(fsys FS, path string) ([]string, error) {
	return callFile1(fsys, "listxattr", path, O_RDONLY, File.ListXAttr)
}

func callFile(fsys FS, op, name string, flags int, do func(File) error) (err error) {
	_, err = callFile1(fsys, op, name, flags, func(file File) (struct{}, error) {
		return struct{}{}, do(file)
	})
	return
}

func callFile1[Func func(File) (R, error), R any](fsys FS, op, name string, flags int, do Func) (ret R, err error) {
	_, ret, err = callFile2(fsys, op, name, flags, func(file File) (struct{}, R, error) {
		r, e := do(file)
		return struct{}{}, r, e
	})
	return
}

func callFile2[Func func(File) (R1, R2, error), R1, R2 any](fsys FS, op, name string, flags int, do Func) (r1 R1, r2 R2, err error) {
	if !ValidPath(name) {
		return r1, r2, makePathError(op, name, ErrNotExist)
	}
	f, err := fsys.OpenFile(name, flags, 0)
	if err != nil {
		return r1, r2, makePathError(op, name, err)
	}
	defer f.Close()
	return do(f)
}

func callDir(fsys FS, op, name string, do func(Directory, string) error) error {
	if !ValidPath(name) {
		return makePathError(op, name, ErrNotExist)
	}
	dir, base := SplitPath(name)
	d, err := OpenDir(fsys, dir)
	if err != nil {
		return makePathError(op, name, err)
	}
	defer d.Close()
	return do(d, base)
}

func callDir2(fsys FS, op, name1, name2 string, do func(Directory, string, Directory, string) error) error {
	if !ValidPath(name1) {
		return makePathError(op, name1, ErrNotExist)
	}
	if !ValidPath(name2) {
		return makePathError(op, name2, ErrInvalid)
	}
	dir1, base1 := SplitPath(name1)
	dir2, base2 := SplitPath(name2)
	d1, err := OpenDir(fsys, dir1)
	if err != nil {
		return makePathError(op, name1, err)
	}
	defer d1.Close()
	d2, err := OpenDir(fsys, dir2)
	if err != nil {
		return makePathError(op, name2, err)
	}
	defer d2.Close()
	return do(d1, base1, d2, base2)
}
