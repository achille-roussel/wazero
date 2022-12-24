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

func testReadWriteFSCreateDirectoryHasPermissions(t *testing.T, newFS MakeReadWriteFS) {
	fsys, closeFS := assertNewFS(t, newFS)
	defer assertCloseFS(t, closeFS)

	assertErrorIs(t, fsys.CreateDir("A", 0755), nil)
	assertErrorIs(t, fsys.CreateDir("B", 0700), nil)
	assertErrorIs(t, fsys.CreateDir("C", 0500), nil)

	assertPermission(t, fsys, "A", 0755)
	assertPermission(t, fsys, "B", 0700)
	assertPermission(t, fsys, "C", 0500)
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
		s, err := f.Stat()
		if err != nil {
			t.Error(err)
		} else {
			if mode := s.Mode() & fs.ModePerm; mode != perm {
				t.Errorf("file permissions mismatch: want=%s got=%s", perm, mode)
			}
		}
	}
	return f
}

func assertPathData(t *testing.T, fsys wasi.FS, path, data string) {
	t.Helper()
	f := assertOpenFile(t, fsys, path, 0, 0)
	defer assertClose(t, f)
	b, err := io.ReadAll(f)
	assertErrorIs(t, err, nil)
	if string(b) != data {
		t.Errorf("%s: content mismatch\nwant: %q\ngot:  %q", path, data, b)
	}
}

func assertCreateDir(t *testing.T, fsys wasi.FS, path string) {
	t.Helper()
	assertErrorIs(t, fsys.CreateDir(path, 0755), nil)
	s, err := fsys.Stat(path)
	if err != nil {
		t.Error(err)
	} else if !s.IsDir() {
		t.Errorf("%s: not a directory", path)
	} else if perm := s.Mode() & 0777; perm != 0755 {
		t.Errorf("%s: permissions mismatch: want=%03o got=%03o", path, 0755, perm)
	}
}

func assertSetFileTimes(t *testing.T, fsys wasi.FS, path string, atim, mtim time.Time) {
	t.Helper()
	assertErrorIs(t, fsys.SetFileTimes(path, 0, atim, mtim), nil)
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

func assertPermission(t *testing.T, fsys wasi.FS, path string, want fs.FileMode) {
	t.Helper()
	s, err := fsys.StatFile(path, 0)
	if err != nil {
		t.Error(err)
	} else {
		perm := s.Mode() & 0777
		if perm != want {
			t.Errorf("%s: permissions mismatch: want=%03o got=%03o", path, want, perm)
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
	if _, err := s.Seek(offset, whence); err != nil {
		t.Fatal(err)
	}
}

func assertRead(t *testing.T, r io.Reader) string {
	t.Helper()
	s := new(strings.Builder)
	_, err := io.Copy(s, r)
	if err != nil {
		t.Fatal(err)
	}
	return s.String()
}

func assertWrite(t *testing.T, w io.Writer, data string) {
	t.Helper()
	n, err := io.WriteString(w, data)
	if err != nil {
		t.Fatal(err)
	}
	if n < len(data) {
		t.Fatal(io.ErrShortWrite)
	}
}
