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
	// Creates a directory on the file system.
	Mkdir(name string, perm fs.FileMode) error
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
}

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
	// Returns a view of the file system rooted at the file (which must be a
	// directory).
	//
	// All name resolutions are done relative to the file location.
	//
	// The returned FS remains valid until the file is closed, after which all
	// method calls on the FS return ErrClosed.
	FS() FS
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
	// Reads the list of directory entries (see fs.ReadDirFile).
	ReadDir(n int) ([]fs.DirEntry, error)
	// Removes a directory from the file system.
	Rmdir(name string) error
	// Removes a file from the file system.
	Unlink(name string) error
}

// NewFS constructs a FS from a fs.FS.
//
// The returned file system is read-only, all attempts to open files in write
// mode, or mutate the state of the file system will error with ErrReadOnly.
func NewFS(base fs.FS) FS { return &readOnlyFS{fsFS{base}} }

type fsFS struct{ base fs.FS }

func (fsys fsFS) Open(name string) (fs.File, error) { return Open(fsys, name) }

func (fsys fsFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	link := name
	loop := 0
openFile:
	if loop++; loop == 40 {
		return nil, ErrLoop
	}
	f, err := fsys.base.Open(link)
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

	return &readOnlyFile{base: f}, nil
}

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

func (fsys *errFS) Link(oldName, newName string, newFS FS) error {
	return fsys.validLink("link", oldName, newName, newFS)
}

func (fsys *errFS) Symlink(oldName, newName string) error {
	return fsys.validPath("symlink", newName)
}

func (fsys *errFS) Rename(oldName, newName string, newFS FS) error {
	return fsys.validLink("rename", oldName, newName, newFS)
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
func CopyFS(dst FS, src ReadFS) error {
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
	dstFS := dst.FS()
	srcFS := src.FS()

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
				err = copyDir(dstFS, srcFS, name, stat)
			case fs.ModeSymlink:
				err = copySymlink(dstFS, srcFS, name, stat)
			case 0: // regular file
				err = copyFile(dstFS, srcFS, name, stat)
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

func copyDir(dst, src FS, name string, stat fs.FileInfo) error {
	if err := dst.Mkdir(name, stat.Mode()); err != nil {
		return err
	}

	r, err := OpenDir(src, name)
	if err != nil {
		return err
	}
	defer r.Close()

	w, err := OpenDir(dst, name)
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

func copySymlink(dst, src FS, name string, stat fs.FileInfo) error {
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

func copyFile(dst, src FS, name string, stat fs.FileInfo) error {
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
func EqualFS(a, b ReadFS) error {
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
	sourceFS := source.FS()
	targetFS := target.FS()

	for {
		entries, err := source.ReadDir(100)

		for _, entry := range entries {
			name := entry.Name()
			switch entry.Type() {
			case fs.ModeDir:
				err = equalDir(sourceFS, targetFS, name, buf)
			case fs.ModeSymlink:
				err = equalSymlink(sourceFS, targetFS, name)
			default:
				err = equalFile(sourceFS, targetFS, name, buf)
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

func equalDir(source, target FS, name string, buf *[equalFSBufsize]byte) error {
	sourceDir, err := OpenDir(source, name)
	if err != nil {
		return err
	}
	defer sourceDir.Close()

	targetDir, err := OpenDir(target, name)
	if err != nil {
		return err
	}
	defer targetDir.Close()

	if err := equalStat(sourceDir, targetDir); err != nil {
		return equalErrorf(name, "%w", err)
	}
	return equalFS(sourceDir, targetDir, buf)
}

func equalSymlink(source, target FS, name string) error {
	sourceLink, err := Readlink(source, name)
	if err != nil {
		return err
	}
	targetLink, err := Readlink(target, name)
	if err != nil {
		return err
	}
	if sourceLink != targetLink {
		return equalErrorf(name, "symbolic links mimatch: want=%q got=%q", sourceLink, targetLink)
	}
	return nil
}

func equalFile(source, target FS, name string, buf *[equalFSBufsize]byte) error {
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
	if sourceMode.IsDir() {
		return nil
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

	sourceSize := sourceInfo.Size()
	targetSize := targetInfo.Size()
	if sourceSize != targetSize {
		return fmt.Errorf("files sizes mismatch: want=%d got=%d", sourceSize, targetSize)
	}

	return nil
}

func equalErrorf(name, msg string, args ...any) error {
	return &fs.PathError{Op: "equal", Path: name, Err: fmt.Errorf(msg, args...)}
}

func call[Func func(FS, string) error, FS any](fsys FS, op, name string, do Func) error {
	_, err := call1(fsys, op, name, func(fsys FS, name string) (struct{}, error) {
		return struct{}{}, do(fsys, name)
	})
	return err
}

func call1[Func func(FS, string) (Ret, error), FS, Ret any](fsys FS, op, name string, do Func) (ret Ret, err error) {
	if !ValidPath(name) {
		err = ErrNotExist
	} else {
		ret, err = do(fsys, name)
	}
	if err != nil {
		err = makePathError(op, name, err)
	}
	return ret, err
}

type openFileFunc = func(string, int, fs.FileMode) (File, error)

type linkOrRename = func(FS, string, string, FS) error

// Open opens a file at the given name in fsys.
//
// The file is open in read-only mode, it might point to a directory.
//
// If the file name points to a symbolic link, the function returns a file
// opened on the link's target.
//
// This function is a valid implementation of the FS.Open methods.
// Implementations of the interface can define their Open method in terms
// of this function as:
//
//	func (fsys customFS) Open(name string) (File, error) {
//		return sys.Open(fsys, name)
//	}
//
func Open(fsys ReadFS, name string) (File, error) {
	return fsys.OpenFile(name, O_RDONLY, 0)
}

// OpenDir opens a directory at the given name in fsys.
func OpenDir(fsys ReadFS, name string) (File, error) {
	return fsys.OpenFile(name, O_DIRECTORY, 0)
}

// OpenRoot opens the root directory of fsys.
func OpenRoot(fsys ReadFS) (File, error) {
	return OpenDir(fsys, ".")
}

// Readlink returns the value of the symbolic link at the given name in fsys.
func Readlink(fsys FS, name string) (string, error) {
	return callFile1(fsys, "readlink", name, O_RDONLY|O_NOFOLLOW, File.Readlink)
}

// Chmod changes permissions of a file at the given name in fsys.
//
// If the name refers to a symbolic link, Chmod dereferences it and modifies the
// permissions of the link's target.
func Chmod(fsys FS, name string, mode fs.FileMode) error {
	return callFile(fsys, "chmod", name, O_RDONLY, func(file File) error {
		return file.Chmod(mode)
	})
}

// Chtimes changes times of a file at the given name in fsys.
//
// If the name refers to a symbolic link, Chtimes dereferences it and modifies the
// times of the link's target.
func Chtimes(fsys FS, name string, atime, mtime time.Time) error {
	return callFile(fsys, "chtimes", name, O_RDONLY, func(file File) error {
		return file.Chtimes(atime, mtime)
	})
}

// Truncate truncates to a specified size the file at the given name in fsys.
//
// If the name refers to a symbolic link, Truncate dereferences it and modifies
// the size of the link's target.
func Truncate(fsys FS, name string, size int64) error {
	return callFile(fsys, "truncate", name, O_WRONLY, func(file File) error {
		return file.Truncate(size)
	})
}

// Stat returns file information for the file with the given name in fsys.
//
// If the name refers to a symbolic link, Stat dereferences it returns
// information about the link's target.
func Stat(fsys FS, name string) (fs.FileInfo, error) {
	return callFile1(fsys, "stat", name, O_RDONLY, File.Stat)
}

// Lstat returns file information for the file with the given name in fsys.
//
// If the name refers to a symbolic link, Lstat returns information about the
// link, and not its target.
func Lstat(fsys FS, name string) (fs.FileInfo, error) {
	return callFile1(fsys, "lstat", name, O_RDONLY|O_NOFOLLOW, File.Stat)
}

/*
func Mkdir(fsys FS, name string) error {
	return callDir(fsys, "mkdir", name, File.Mkdir)
}
*/

func Rmdir(fsys FS, name string) error {
	return callDir(fsys, "rmdir", name, File.Rmdir)
}

func Unlink(fsys FS, name string) error {
	return callDir(fsys, "unlink", name, File.Unlink)
}

/*
func Symlink(fsys FS, oldName, newName string) error {
	return callDir(fsys, "symlink", newName, func(dir File, newName string) error {
		return dir.Symlink(oldName, newName)
	})
}
*/

func callFile(fsys FS, op, name string, flags int, do func(File) error) error {
	_, err := callFile1(fsys, op, name, flags, func(file File) (struct{}, error) {
		return struct{}{}, do(file)
	})
	return err
}

func callFile1[Func func(File) (Ret, error), Ret any](fsys FS, op, name string, flags int, do Func) (ret Ret, err error) {
	f, err := fsys.OpenFile(name, flags, 0)
	if err != nil {
		return ret, makePathError(op, name, err)
	}
	defer f.Close()
	return do(f)
}

func callDir(fsys FS, op, name string, do func(File, string) error) error {
	dir, base := SplitPath(name)
	f, err := OpenDir(fsys, dir)
	if err != nil {
		return makePathError(op, name, err)
	}
	defer f.Close()
	return do(f, base)
}
