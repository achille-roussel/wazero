package sys

/*
// NewFile constructs a File normalizing the behavior of the
func NewFile(base File, name string) File {
	return &file{base: base, name: name}
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
	var name string
	if f.base == nil {
		name, err = oldName, ErrClosed
	} else if !ValidPath(oldName) {
		name, err = oldName, ErrNotExist
	} else if !ValidPath(newName) {
		name, err = newName, ErrInvalid
	} else {
		name, err = newName, f.base.Rename(oldName, newDir, newName)
	}
	if err != nil {
		err = makePathError("link", name, err)
	}
	return err
}

func (f *file) Rename(oldName string, newDir Directory, newName string) (err error) {
	var name string
	if f.base == nil {
		name, err = oldName, ErrClosed
	} else if !ValidPath(oldName) {
		name, err = oldName, ErrNotExist
	} else if !ValidPath(newName) {
		name, err = newName, ErrInvalid
	} else {
		name, err = newName, f.base.Rename(oldName, newDir, newName)
	}
	if err != nil {
		err = makePathError("rename", name, err)
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
*/
