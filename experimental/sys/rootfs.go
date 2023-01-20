package sys

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// MountPoint represents the mount point of a file system at a specified path.
type MountPoint struct {
	Path string
	Fsys FS
}

func (mp MountPoint) String() string { return mp.Path }

// RootFS constructs a root file system from base and list of mount points.
//
// The mount points that appears after in the list take precedence, and might
// mask the lower mount points or paths on the base file system if they overlap.
//
// The returned file system sandboxes all path lookups so none escape the root,
// even in the presence of symbolic links containing absolute or relative paths.
//
// The function panics if any of the file systems is nil, or if some of the
// mount paths are invalid. Validation of the mount paths is done using
// fs.ValidPath and checking that none of the mount paths are the root (".").
func RootFS(base FS, mounts ...MountPoint) FS {
	if len(mounts) > 0 {
		base = mountFS(base, mounts)
	}
	return sandboxFS(base)
}

// mountFS creates a file system from stacking the mount points on top of a base
// file system.
//
// The implementation expects to be wrapped by sandboxFS as it relies on the
// relative path resolution.
func mountFS(base FS, mounts []MountPoint) FS {
	mounts = append([]MountPoint{}, mounts...)

	for _, mount := range mounts {
		if mount.Path == "." || !fs.ValidPath(mount.Path) {
			panic("invalid mount path: " + mount.Path)
		}
		if mount.Fsys == nil {
			panic("invalid mount of nil file system")
		}
	}

	// Reverse so we can use a range loop to iterate over the list of mount
	// points in the right priority order.
	for i, j := 0, len(mounts)-1; i < j; {
		mounts[i], mounts[j] = mounts[j], mounts[i]
		i++
		j--
	}

	fsys := &mountPoints{
		base:   MountPoint{Path: ".", Fsys: base},
		mounts: mounts,
	}

	return FuncFS(func(name string, flags int, perm fs.FileMode) (File, error) {
		m := fsys.findMountPoint(name)
		f, err := m.Fsys.OpenFile(name, flags, perm)
		if err != nil {
			return nil, err
		}
		return fsys.newFile(f, nil, m, name), nil
	})
}

type mountPoints struct {
	base   MountPoint
	mounts []MountPoint
}

func (fsys *mountPoints) findMountPoint(path string) *MountPoint {
	for i := range fsys.mounts {
		m := &fsys.mounts[i]

		if PathContains(m.Path, path) {
			return m
		}
	}
	return &fsys.base
}

func (fsys *mountPoints) newFile(file File, dir *mountedFile, mount *MountPoint, path string) *mountedFile {
	f := &mountedFile{
		dir:        dir,
		fsys:       fsys,
		mount:      mount,
		path:       path,
		sharedFile: sharedFile{File: file},
	}
	if dir != nil {
		dir.ref()
	}
	f.ref()
	return f
}

type mountedFile struct {
	dir   *mountedFile
	fsys  *mountPoints
	mount *MountPoint
	path  string
	sharedFile
}

func (f *mountedFile) GoString() string {
	return fmt.Sprintf("&sys.mountedFile{%#v}", f.File)
}

func (f *mountedFile) Close() error {
	if f.dir != nil { // the root has no parent directory
		f.dir.unref()
		f.dir = nil
	}
	f.unref()
	return nil
}

func (f *mountedFile) OpenFile(name string, flags int, perm fs.FileMode) (file File, err error) {
	// When accessing the parent directory, simply return a reference to it.
	//
	// Reusing this reference to the parent directory is important because we
	// might be entering a different mount point and if we do, we cannot resolve
	// path the absolute path from the file system root or we might follow
	// symbolic links which may not place us at the actual parent directory.
	if name == ".." {
		f.dir.ref()
		return f.dir, nil
	}

	// We now know that we are opening an entry in the current directory, which
	// may also cause entering a new mount point.
	path := JoinPath(f.path, name)
	mount := f.fsys.findMountPoint(path)

	if mount == f.mount {
		file, err = f.sharedFile.OpenFile(name, flags, perm)
	} else {
		file, err = mount.Fsys.OpenFile(".", flags, perm)
	}
	if err != nil {
		return nil, err
	}
	return f.fsys.newFile(file, f, mount, path), nil
}

// sandboxFS creates a sandbox of a file system preventing escape from the root.
//
// The sandbox is always the top-most layer of a root file system, sub-layers
// such as the overlay file system rely on its path lookup algorithm and will
// produce unpredictable behavior if they are used without being wrapped in a
// sandbox.
func sandboxFS(fsys FS) FS {
	return FuncFS(func(name string, flags int, perm fs.FileMode) (File, error) {
		d, err := OpenRoot(fsys)
		if err != nil {
			return nil, err
		}
		// A shared reference to the root is maintained by all files opened from
		// the sandbox root. It is used during path lookups to go back to the
		// root when we find symbolic links with absolute targets.
		//
		// Because it is shared, the root is reference counted to ensure that it
		// remains opened until all files that were derived from it have been
		// closed.
		root := shareFile(d)
		defer root.unref()
		if name == "." {
			root.ref() // +1 because we keep 2 references to it
			return sandboxFile(root, sharedFileRef{root}, "."), nil
		}
		f, err := lookup(d, d, ".", name, flags, perm)
		if err != nil {
			return nil, err
		}
		return sandboxFile(root, f, name), nil
	})
}

func sandboxFile(root *sharedFile, file File, name string) *sandboxedFile {
	root.ref()
	return &sandboxedFile{root: root, name: name, File: file}
}

type sandboxedFile struct {
	root *sharedFile
	name string
	File
}

func (f *sandboxedFile) GoString() string {
	return fmt.Sprintf("&sys.sandboxedFile{%#v}", f.File)
}

func (f *sandboxedFile) Close() error {
	f.root.unref()
	f.root = nil
	return f.File.Close()
}

func (f *sandboxedFile) Access(name string, mode fs.FileMode) error {
	return lookupDir(f, "access", name, func(dir Directory, name string) error {
		return dir.Access(name, mode)
	})
}

func (f *sandboxedFile) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	newFile, err := f.openFile(name, flags, perm)
	if err != nil {
		return nil, err
	}
	return sandboxFile(f.root, newFile, JoinPath(f.name, name)), nil
}

func (f *sandboxedFile) openFile(name string, flags int, perm fs.FileMode) (File, error) {
	return lookup(f.root.File, f.File, f.name, name, flags, perm)
}

func (f *sandboxedFile) Mknod(name string, mode fs.FileMode, dev Device) error {
	return lookupDir(f, "mknod", name, func(dir Directory, name string) error {
		return dir.Mknod(name, mode, dev)
	})
}

func (f *sandboxedFile) Mkdir(name string, perm fs.FileMode) error {
	return lookupDir(f, "mkdir", name, func(dir Directory, name string) error {
		return dir.Mkdir(name, perm)
	})
}

func (f *sandboxedFile) Rmdir(name string) error {
	return lookupDir(f, "rmdir", name, Directory.Rmdir)
}

func (f *sandboxedFile) Unlink(name string) error {
	return lookupDir(f, "unlink", name, Directory.Unlink)
}

func (f *sandboxedFile) Symlink(oldName, newName string) error {
	return lookupDir(f, "symlink", newName, func(dir Directory, newName string) error {
		return dir.Symlink(oldName, newName)
	})
}

func (f *sandboxedFile) Link(oldName string, newDir Directory, newName string) error {
	return lookupDir2(f, newDir, "link", oldName, newName, Directory.Link)
}

func (f *sandboxedFile) Rename(oldName string, newDir Directory, newName string) error {
	return lookupDir2(f, newDir, "rename", oldName, newName, Directory.Rename)
}

func (f *sandboxedFile) Lchmod(name string, mode fs.FileMode) error {
	return lookupDir(f, "chmod", name, func(dir Directory, name string) error {
		return dir.Lchmod(name, mode)
	})
}

func (f *sandboxedFile) Lstat(name string) (fs.FileInfo, error) {
	return lookupDir1(f, "stat", name, Directory.Lstat)
}

type nopClose struct{ File }

func (f nopClose) GoString() string { return fmt.Sprintf("sys.nopClose{%#v}", f.File) }
func (f nopClose) Close() error     { return nil }

// sentinel error value used to break out of WalkPath when resolving symbolic links
var errResolveSymlink = errors.New("resolve symlink")

func lookup(root, dir File, base, path string, flags int, perm fs.FileMode) (File, error) {
	dir = nopClose{dir} // don't close the first directory received as arguments
	defer func() { dir.Close() }()

	setCurrentDirectory := func(d File) {
		dir.Close()
		dir = d
	}

	setSymbolicLink := func(link string) error {
		if link = CleanPath(link); strings.HasPrefix(link, "/") {
			// The symbolic link contained an absolute path starting with a "/".
			// We go back to the root and start resolving paths back from there.
			setCurrentDirectory(nopClose{root})
			base = "."
			path = link[1:]
		} else if path != "" {
			// There are trailing path components to lookup after resolving the
			// symbolic link, which means the link represented a directory; we
			// walk up the the parent because relative paths will be resolved
			// from there, and append the remaining path components to the link
			// target in order to form the full path to lookup.
			base, _ = SplitPath(base)
			path = CleanPath(link + "/" + path)
		} else {
			// The path was empty, which indicates that we had fully resolved
			// the symbolic link and are now pointing at the right location.
			path = link
		}
		return nil
	}

	var link string
	var loop int
	var err error
resolvePath:
	if loop++; loop == MaxSymlinkLookups {
		return nil, ErrLoop
	}

	base, path, err = WalkPath(base, path, func(dirname string) error {
		f, err := dir.OpenFile(dirname, openFlagsPath, 0)
		if err != nil {
			return err
		}
		defer func() { closeIfNotNil(f) }()

		s, err := f.Stat()
		if err != nil {
			return err
		}

		switch s.Mode().Type() {
		case fs.ModeDir:
			setCurrentDirectory(f)
			f = nil
			return nil

		case fs.ModeSymlink:
			s, err := f.Readlink()
			if err != nil {
				return err
			}
			link = s
			return errResolveSymlink

		default:
			return ErrNotDirectory
		}
	})

	switch err {
	case nil:
	case errResolveSymlink:
		if err := setSymbolicLink(link); err != nil {
			return nil, err
		}
		goto resolvePath
	default:
		return nil, err
	}

	// If O_DIRECTORY is passed, it already enforces O_NOFOLLOW since we are
	// explicitly saying that we want to open a directory and not a symbolic
	// link. In every other case, we add O_NOFOLLOW so we can perform the
	// symbolic link resolution (if any).
	openFlags := flags
	if !hasDirectoryFlags(openFlags) {
		openFlags |= openFlagsNoFollow
	}

	f, err := dir.OpenFile(path, openFlags, perm)
	if err != nil {
		return nil, err
	}

	if !hasDirectoryFlags(flags) && !hasNoFollowFlags(flags) {
		stat, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		if stat.Mode().Type() == fs.ModeSymlink {
			link, err := f.Readlink()
			f.Close()
			if err != nil {
				return nil, err
			}
			path = ""
			if err := setSymbolicLink(link); err != nil {
				return nil, err
			}
			goto resolvePath
		}
	}

	return f, nil
}

func lookupDir(f *sandboxedFile, op, name string, fn func(Directory, string) error) error {
	_, err := lookupDir1(f, op, name, func(d Directory, name string) (struct{}, error) {
		return struct{}{}, fn(d, name)
	})
	return err
}

func lookupDir1[F func(Directory, string) (R, error), R any](f *sandboxedFile, op, name string, fn F) (ret R, err error) {
	dir, base := SplitPath(name)
	if dir == "." {
		return fn(f.File, base)
	}
	d, err := f.openFile(dir, openFlagsDirectory, 0)
	if err != nil {
		return ret, err
	}
	defer d.Close()
	return fn(d, base)
}

func lookupDir2(f *sandboxedFile, d Directory, op, name1, name2 string, fn func(Directory, string, Directory, string) error) error {
	arg1 := Directory(f.File)
	arg2 := d
	dir1, base1 := SplitPath(name1)
	dir2, base2 := SplitPath(name2)
	if dir1 != "." {
		d1, err := f.openFile(dir1, openFlagsDirectory, 0)
		if err != nil {
			return err
		}
		defer d1.Close()
		arg1 = d1
	}
	if dir2 != "." {
		d2, err := f.openFile(dir2, openFlagsDirectory, 0)
		if err != nil {
			return err
		}
		defer d2.Close()
		arg2 = d2
	}
	return fn(arg1, base1, arg2, base2)
}
