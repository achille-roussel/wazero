package wasi_snapshot_preview1

import (
	"io"
	"io/fs"
	"math/bits"

	"github.com/tetratelabs/wazero/wasi"
)

type file struct {
	base               wasi.File
	fsRightsBase       Rights
	fsRightsInheriting Rights
	dircookie          Dircookie
	direntries         []fs.DirEntry
}

func (f *file) Name() string { return f.base.Name() }

func (f *file) Close() error { return f.base.Close() }

func (f *file) OpenFile(path string, flags int, perm fs.FileMode) (wasi.File, error) {
	if !f.fsRightsBase.Has(PATH_OPEN) {
		return nil, fs.ErrPermission
	}
	return f.base.OpenFile(path, flags, perm)
}

func (f *file) Read(b []byte) (int, error) {
	if !f.fsRightsBase.Has(FD_READ) {
		return 0, fs.ErrPermission
	}
	return f.base.Read(b)
}

func (f *file) ReadAt(b []byte, off int64) (int, error) {
	if !f.fsRightsBase.Has(FD_READ | FD_SEEK) {
		return 0, fs.ErrPermission
	}
	return f.base.ReadAt(b, off)
}

func (f *file) Write(b []byte) (int, error) {
	if !f.fsRightsBase.Has(FD_WRITE) {
		return 0, fs.ErrPermission
	}
	return f.base.Write(b)
}

func (f *file) WriteAt(b []byte, off int64) (int, error) {
	if !f.fsRightsBase.Has(FD_WRITE | FD_SEEK) {
		return 0, fs.ErrPermission
	}
	return f.base.WriteAt(b, off)
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	rights := Rights(0)
	if offset == 0 && whence == io.SeekCurrent {
		rights = FD_TELL
	} else {
		rights = FD_SEEK
	}
	if !f.fsRightsBase.Has(rights) {
		return -1, fs.ErrPermission
	}
	return f.base.Seek(offset, whence)
}

func (f *file) Stat() (fs.FileInfo, error) {
	if !f.fsRightsBase.Has(FD_FILESTAT_GET) {
		return nil, fs.ErrPermission
	}
	return f.base.Stat()
}

func (f *file) StatFile(path string, flags int) (fs.FileInfo, error) {
	if !f.fsRightsBase.Has(FD_FILESTAT_GET) {
		return nil, fs.ErrPermission
	}
	return f.base.StatFile(path, flags)
}

func (f *file) ReadDir(n int) ([]fs.DirEntry, error) {
	if !f.fsRightsBase.Has(FD_READDIR) {
		return nil, fs.ErrPermission
	}
	return f.base.ReadDir(n)
}

var (
	_ wasi.File = (*file)(nil)
)

// Table is a data structure mapping 32 bit keys to items of arbitrary type.
//
// Key generation is managed by the table, and currently uses a strategy similar
// to the file descriptor allocation on unix systems: the lowest key not mapped
// to any value is used when inserting a new item.
//
// The data structure is used to implement the file descriptor table of the WASI
// implementation of Wazero. The WASI standard documents that programs cannot
// expect that file descriptor numbers will be allocated with a lowest-first
// strategy (like it is done on unix systems), and they shouldi instead assume
// the values will be random. Since "random" is a very imprecise concept in
// computers, we technically satisfying the implementation with the key
// allocation strategy we use here. We could imagine adding more "randomness"
// to the key selection process, however this should never be used as a security
// measure to prevent applications from guessing the next file number so there
// are no strong incentives to complicate the logic.
//
// The data structure optimizes for memory density and lookup performance,
// trading off compute at insertion time. This is a useful compromise for the
// use cases we employ it with: files are usually read or written a lot more
// often than they are opened, each operation requires a table lookup so we are
// better off spending extra compute to insert files in the table in order to
// get cheaper lookups. Memory efficiency is also crucial to support scaling
// with programs that open thousands of files: having a high or non-linear
// memory-to-item ratio could otherwise be used as an attack vector by malicous
// applications attempting to damage performance of the host.
type fileTable struct {
	masks []uint64
	files []*file
}

// len returns the number of files stored in the table.
func (t *fileTable) len() (n int) {
	// We could make this a O(1) operation if we cached the number of files in
	// the table. More state usually means more problems, so until we have a
	// clear need for this, the simple implementation may be a better trade off.
	for _, mask := range t.masks {
		n += bits.OnesCount64(mask)
	}
	return n
}

// grow ensures that t has enough room for n files, potentially reallocating the
// internal buffers if their capacity was too small to hold this many files.
func (t *fileTable) grow(n int) {
	if n = (n*8 + 7) / 8; n > len(t.masks) {
		masks := make([]uint64, n)
		copy(masks, t.masks)

		files := make([]*file, n*64)
		copy(files, t.files)

		t.masks = masks
		t.files = files
	}
}

// insert inserts the given file to the table, returning the fd that it is
// mapped to.
//
// The method does not perform deduplication, it is possible for the same file
// to be inserted multiple times, each insertion will return a different fd.
func (t *fileTable) insert(file *file) (fd Fd) {
	offset := 0
insert:
	// TODO: this loop could be made a lot more efficient using vectorized
	// operations: 256 bits vector registers would yield a theoretical 4x
	// speed up (e.g. using AVX2).
	for index, mask := range t.masks[offset:] {
		if ^mask != 0 { // not full?
			shift := bits.TrailingZeros64(^mask)
			index += offset
			fd = Fd(index)*64 + Fd(shift)
			t.files[fd] = file
			t.masks[index] = mask | uint64(1<<shift)
			return fd
		}
	}

	offset = len(t.masks)
	n := 2 * len(t.masks)
	if n == 0 {
		n = 1
	}

	t.grow(n)
	goto insert
}

// lookup returns the file associated with the given fd (may be nil).
func (t *fileTable) lookup(fd Fd) *file {
	if i := int(fd); i >= 0 && i < len(t.files) {
		return t.files[i]
	}
	return nil
}

// delete deletes the file stored at the given fd from the table, returning the
// deleted file (may be nil).
func (t *fileTable) delete(fd Fd) (file *file) {
	if index, shift := fd/64, fd%64; int(index) < len(t.masks) {
		mask := t.masks[index]
		if (mask & (1 << shift)) != 0 {
			file = t.files[fd]
			t.files[fd] = nil
			t.masks[index] = mask & ^uint64(1<<shift)
		}
	}
	return file
}

// scan calls f for each file and its associated fd in the table. The function
// f might return false to interupt the iteration.
func (t *fileTable) scan(f func(Fd, *file) bool) {
	for i, mask := range t.masks {
		if mask != 0 {
			for j := Fd(0); j < 64; j++ {
				if (mask & (1 << j)) != 0 {
					if fd := Fd(i)*64 + j; !f(fd, t.files[fd]) {
						return
					}
				}
			}
		}
	}
}

// reset clears the content of the table.
func (t *fileTable) reset() {
	for i := range t.masks {
		t.masks[i] = 0
	}
	for i := range t.files {
		t.files[i] = nil
	}
}
