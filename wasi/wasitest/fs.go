package wasitest

import (
	"errors"
	"io"
	"io/fs"
	"sort"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/wasi"
)

// CloseFS is a function returned by MakeReadOnlyFS and MakeReadWriteFS which is
// used to tear down resources associated with a file system instance created
// during a test.
type CloseFS func() error

func testFS(t *testing.T, fsys fs.FS, files fstest.MapFS) {
	t.Helper()
	t.Run("fstest", func(t *testing.T) {
		expected := make([]string, 0, len(files))
		for fileName := range files {
			expected = append(expected, fileName)
		}
		sort.Strings(expected)
		if err := fstest.TestFS(fsys, expected...); err != nil {
			t.Error(err)
		}
	})
}

func assertNewFS(t *testing.T, newFS func() (wasi.FS, CloseFS, error)) (wasi.FS, CloseFS) {
	t.Helper()
	fsys, closeFS, err := newFS()
	if err != nil {
		t.Fatal("creating file system:", err)
	}
	return fsys, closeFS
}

func assertClose(t *testing.T, c io.Closer) {
	t.Helper()
	assertErrorIs(t, c.Close(), nil)
}

func assertCloseFS(t *testing.T, closeFS CloseFS) {
	t.Helper()
	assertErrorIs(t, closeFS(), nil)
}

func assertOpenFile(t *testing.T, fsys wasi.FS, path string, oflags int, perm fs.FileMode) wasi.File {
	t.Helper()
	f, err := fsys.OpenFile(path, oflags, perm)
	assertErrorIs(t, err, nil)
	if (oflags & wasi.O_CREATE) != 0 {
		stat := assertFileStat(t, f)
		mode := stat.Mode() & fs.ModePerm
		if mode != perm {
			t.Errorf("file permissions mismatch: want=%s got=%s", perm, mode)
		}
	}
	return f
}

func assertPathData(t *testing.T, fsys wasi.FS, path, want string) {
	t.Helper()
	f := assertOpenFile(t, fsys, path, 0, 0)
	defer assertClose(t, f)
	assertFileData(t, f, want)
}

func assertMakeDir(t *testing.T, fsys wasi.FS, path string, perm fs.FileMode) {
	t.Helper()
	assertErrorIs(t, fsys.MakeDir(path, perm), nil)
	s, err := fsys.Stat(path)
	if err != nil {
		t.Error(err)
	} else if !s.IsDir() {
		t.Errorf("%s: not a directory", path)
	} else if mode := s.Mode() & fs.ModePerm; mode != perm {
		t.Errorf("%s: permissions mismatch: want=%03o got=%03o", path, perm, mode)
	}
}

func assertChtimes(t *testing.T, fsys wasi.FS, path string, atim, mtim time.Time) {
	t.Helper()
	assertErrorIs(t, fsys.Chtimes(path, 0, atim, mtim), nil)
	s, err := fsys.Stat(path)
	if err != nil {
		t.Error(err)
	} else {
		statAtim := time.Time{}
		statMtim := time.Time{}

		switch stat := s.Sys().(type) {
		case *wasi_snapshot_preview1.Filestat:
			statAtim = stat.Atim.Time()
			statMtim = stat.Mtim.Time()
		case *syscall.Stat_t:
			statAtim = time.Unix(stat.Atim.Unix())
			statMtim = time.Unix(stat.Mtim.Unix())
		default:
			t.Error("unsupported file stat is neither wasi_snapshot_preview1.Filestat nor syscall.Stat_t")
		}

		if !atim.IsZero() {
			if !atim.Equal(statAtim) {
				t.Errorf("access time mismatch: want=%v got=%v", atim, statAtim)
			}
		}

		if !mtim.IsZero() {
			if !mtim.Equal(statMtim) {
				t.Errorf("modification time mismatch: want=%v got=%v", mtim, statMtim)
			}
		}
	}
}

func assertErrorIs(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Errorf("error mismatch\nwant: %v\ngot:  %v", want, got)
	}
}

func assertSeek(t *testing.T, s io.Seeker, offset int64, whence int) {
	t.Helper()
	_, err := s.Seek(offset, whence)
	assertErrorIs(t, err, nil)
}

func assertRead(t *testing.T, r io.Reader) string {
	t.Helper()
	s := new(strings.Builder)
	_, err := io.Copy(s, r)
	assertErrorIs(t, err, nil)
	return s.String()
}

func assertWrite(t *testing.T, w io.Writer, data string) {
	t.Helper()
	n, err := io.WriteString(w, data)
	assertErrorIs(t, err, nil)
	if n < len(data) {
		t.Fatal(io.ErrShortWrite)
	}
	if n > len(data) {
		t.Fatalf("too many bytes written to file: want=%d got=%d", len(data), n)
	}
}

func assertTruncate(t *testing.T, f wasi.File, size int64) {
	t.Helper()
	err := f.Truncate(size)
	assertErrorIs(t, err, nil)
	assertFileSize(t, f, size)
}

func assertFileStat(t *testing.T, f wasi.File) fs.FileInfo {
	t.Helper()
	s, err := f.Stat()
	assertErrorIs(t, err, nil)
	return s
}

func assertFileData(t *testing.T, f wasi.File, want string) {
	t.Helper()
	assertSeek(t, f, 0, io.SeekStart)
	if got := assertRead(t, f); got != want {
		t.Errorf("file content mimatch: want=%q got=%q", want, got)
	}
}

func assertFileSize(t *testing.T, f wasi.File, want int64) {
	t.Helper()
	stat := assertFileStat(t, f)
	size := stat.Size()
	if size != want {
		t.Errorf("file size mismatch: want=%d got=%d", want, size)
	}
}
