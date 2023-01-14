package sys

import (
	"errors"
	"io"
	"io/fs"
	"strings"
)

// RootFS constructs a root file system by stacking the file system layers given
// as arguments.
//
// The returned file system sandboxes all path lookups so none escape the root,
// even in the presence of symbolic links containing absolute or relative paths.
//
// All layers but the last are treated as read-only. The last layer always
// receives all mutations.
//
// If no layers are given, the function returns an empty, read-only file system.
func RootFS(layers ...FS) FS {
	var root FS

	switch len(layers) {
	case 0:
		root = ErrFS(ErrNotExist)
	case 1:
		root = layers[0]
	default:
		root = overlayFS(layers)
	}

	return sandboxFS(root)
}

// overalFS creates a file system which stacks the layers passed as arguments.
//
// The overlay is intended to behave the same as the Linux Overlay File System,
// see https://docs.kernel.org/filesystems/overlayfs.html
//
// One key implementation difference with he linux overlay is we stack N layers
// instead of stacking pairs recursively. This has the advantage of keeping some
// costs constant (e.g. the directory entry name cache).
func overlayFS(layers []FS) FS {
	// Reverse the layers so we can use `range` to iterate the slice while
	// maintaining the priority order.
	layers = append([]FS{}, layers...)

	for i, j := 0, len(layers)-1; i < j; {
		layers[i], layers[j] = layers[j], layers[i]
		i++
		j--
	}

	return FuncFS(func(_ FS, name string, flags int, perm fs.FileMode) (File, error) {
		root := &fileOverlay{
			files: make([]fileRef, len(layers)),
		}
		defer root.unref() // cleanup in case OpenFile error or panic

		for i := range layers {
			f, err := layers[i].OpenFile(name, flags, perm)
			if err != nil {
				// All roots must be successfuly opened otherwise we might
				// see an inconsistent state of the overlay file system.
				return nil, err
			}
			root.files[i] = makeFileRef(f, 0)
		}

		file := ReadOnlyFile(root, name, nil)
		root.ref()
		return file, nil
	})
}

type fileRef struct {
	*sharedFile
	depth int
}

func makeFileRef(file File, depth int) fileRef {
	return fileRef{shareFile(file), depth}
}

func (r fileRef) ref() {
	if r.sharedFile != nil {
		r.sharedFile.ref()
	}
}

func (r fileRef) unref() {
	if r.sharedFile != nil {
		r.sharedFile.unref()
	}
}

type fileOverlay struct {
	files []fileRef
	depth int
	names map[string]struct{}
}

func overlayFile(files []fileRef, depth int) *fileOverlay {
	f := &fileOverlay{
		files: make([]fileRef, len(files)),
		depth: depth,
	}
	for i, file := range files {
		f.files[i] = file
		f.files[i].ref()
	}
	return f
}

func (f *fileOverlay) ref() {
	for i := range f.files {
		f.files[i].ref()
	}
}

func (f *fileOverlay) unref() {
	for i := range f.files {
		f.files[i].unref()
	}
}

func (f *fileOverlay) Close() error {
	f.unref()
	f.files = nil
	f.names = nil
	return nil
}

func (f *fileOverlay) Read(b []byte) (int, error) {
	return overlayCall(f, func(file File) (int, error) { return file.Read(b) })
}

func (f *fileOverlay) ReadAt(b []byte, offset int64) (int, error) {
	return overlayCall(f, func(file File) (int, error) { return file.ReadAt(b, offset) })
}

func (f *fileOverlay) Write(b []byte) (int, error) {
	return overlayCall(f, func(file File) (int, error) { return file.Write(b) })
}

func (f *fileOverlay) WriteAt(b []byte, offset int64) (int, error) {
	return overlayCall(f, func(file File) (int, error) { return file.WriteAt(b, offset) })
}

func (f *fileOverlay) Seek(offset int64, whence int) (int64, error) {
	// TODO: figure out how to reset going to the beginning of the
	// directory.
	return overlayCall(f, func(file File) (int64, error) { return file.Seek(offset, whence) })
}

func (f *fileOverlay) Readlink() (string, error) {
	return overlayCall(f, File.Readlink)
}

func (f *fileOverlay) Stat() (fs.FileInfo, error) {
	return overlayCall(f, File.Stat)
}

func (f *fileOverlay) ReadDir(n int) (dirents []fs.DirEntry, err error) {
	if n == 0 {
		n = -1
	}
	if n > 0 {
		dirents = make([]fs.DirEntry, 0, n)
	}
	// We have to keep track of all the directory entries that we have seen
	// because some names may appear in multiple layers and we can't produce
	// duplicate entries.
	if f.names == nil {
		f.names = make(map[string]struct{})
	}
	for _, r := range f.files {
		if r.depth != f.depth {
			continue
		}
		ds, err := r.ReadDir(n - len(dirents))
		for _, d := range ds {
			name := d.Name()
			if _, seen := f.names[name]; !seen {
				f.names[name] = struct{}{}
				dirents = append(dirents, d)
			}
		}
		if err != nil && err != io.EOF {
			return dirents, err
		}
		if n == len(dirents) {
			return dirents, nil
		}
	}
	if n > 0 {
		err = io.EOF
	}
	return dirents, err
}

func (f *fileOverlay) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	depth := f.depth
	if name == ".." {
		depth--
	} else {
		depth++
	}
	open := overlayFile(f.files, depth)
	openFiles := 0
	defer open.unref()

	for i, r := range f.files {
		if r.depth != f.depth {
			continue
		}

		x, err := r.OpenFile(name, flags, perm)
		if err != nil {
			// Any error other than a non-existent entry is treated as fatal
			// because it prevents us from having a deterministic behavior and
			// could result in exposing masked files from the underlying layers.
			if !errors.Is(err, ErrNotExist) {
				return nil, err
			}
			continue
		}

		s, err := x.Stat()
		if err != nil {
			return nil, err
		}

		// If there is already one open file, it means that we are opening a
		// level of directories: if the layer contains a directory, it becomes
		// part of the layered open file. Otherwise, we have a file which
		// masks all the layers below it and therefore we have to stop here.
		if openFiles > 0 && !s.IsDir() {
			x.Close()
			break
		}

		newRef := makeFileRef(x, depth)
		open.files[i].unref()
		open.files[i] = newRef
		openFiles++

		// If this is the first open file, we only carry on if it is a directory
		// because any other file type would mask the underlying layers.
		if openFiles == 1 && !s.IsDir() {
			break
		}
	}

	if openFiles == 0 {
		return nil, ErrNotExist
	}

	file := ReadOnlyFile(open, name, nil)
	open.ref()
	return file, nil
}

func overlayCall[Func func(File) (Ret, error), Ret any](f *fileOverlay, do func(File) (Ret, error)) (Ret, error) {
	for _, r := range f.files {
		if r.depth == f.depth {
			return do(r.File)
		}
	}
	var zero Ret
	return zero, ErrNotSupported
}

// sandboxFS creates a sandbox of a file system preventing escape from the root.
//
// The sandbox is always the top-most layer of a root file system, sub-layers
// such as the overlay file system rely on its path lookup algorithm and will
// produce unpredictable behavior if they are used without being wrapped in a
// sandbox.
func sandboxFS(fsys FS) FS {
	return FuncFS(func(_ FS, name string, flags int, perm fs.FileMode) (File, error) {
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

func (f *sandboxedFile) Close() error {
	f.root.unref()
	f.root = nil
	return f.File.Close()
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
	return lookupDir2(f, "link", oldName, newName, Directory.Link)
}

func (f *sandboxedFile) Rename(oldName string, newDir Directory, newName string) error {
	return lookupDir2(f, "rename", oldName, newName, Directory.Rename)
}

type nopClose struct{ File }

func (nopClose) Close() error { return nil }

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
	if loop++; loop == 40 {
		return nil, ErrLoop
	}

	base, path, err = WalkPath(base, path, func(dirname string) error {
		f, err := dir.OpenFile(dirname, rootfsOpenFileFlags, 0)
		if err != nil {
			return err
		}
		defer func() {
			if f != nil {
				f.Close()
			}
		}()

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
	if (openFlags & O_DIRECTORY) == 0 {
		openFlags |= O_NOFOLLOW
	}

	f, err := dir.OpenFile(path, openFlags, perm)
	if err != nil {
		return nil, err
	}

	s, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	if ((flags & O_NOFOLLOW) == 0) && s.Mode().Type() == fs.ModeSymlink {
		s, err := f.Readlink()
		f.Close()
		if err != nil {
			return nil, err
		}
		path = ""
		if err := setSymbolicLink(s); err != nil {
			return nil, err
		}
		goto resolvePath
	}

	return f, nil
}

func lookupDir(f *sandboxedFile, op, name string, do func(Directory, string) error) error {
	dir, base := SplitPath(name)
	if dir == "." {
		return do(f.File, base)
	}
	d, err := f.openFile(dir, O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer d.Close()
	return do(d, base)
}

func lookupDir2(f *sandboxedFile, op, name1, name2 string, do func(Directory, string, Directory, string) error) error {
	arg1 := Directory(f.File)
	arg2 := Directory(f.File)
	dir1, base1 := SplitPath(name1)
	dir2, base2 := SplitPath(name2)
	if dir1 != "." {
		d1, err := f.openFile(dir1, O_DIRECTORY, 0)
		if err != nil {
			return err
		}
		defer d1.Close()
		arg1 = d1
	}
	if dir2 != "." {
		d2, err := f.openFile(dir2, O_DIRECTORY, 0)
		if err != nil {
			return err
		}
		defer d2.Close()
		arg2 = d2
	}
	return do(arg1, base1, arg2, base2)
}
