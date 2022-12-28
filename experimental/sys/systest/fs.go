package systest

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

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
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

type fsTestCase struct {
	scenario string
	function func(sys.FS) error
}

func testFSError(t *testing.T, fsys sys.FS, want error, tests ...fsTestCase) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) {
			assertErrorIs(t, test.function(fsys), want)
		})
	}
}

func testFSErrNotExist(t *testing.T, fsys sys.FS) {
	t.Helper()
	testFSError(t, fsys, fs.ErrNotExist,
		fsTestCase{
			scenario: "calling Open for a path which does not exist returns fs.ErrNotExist",
			function: func(fsys sys.FS) error {
				_, err := fsys.Open("nope")
				return err
			},
		},

		fsTestCase{
			scenario: "calling OpenFile for a path which does not exist returns fs.ErrNotExist",
			function: func(fsys sys.FS) error {
				_, err := fsys.OpenFile("nope", 0, 0)
				return err
			},
		},

		fsTestCase{
			scenario: "calling Stat for a path which does not exist returns fs.ErrNotExist",
			function: func(fsys sys.FS) error {
				_, err := fsys.Stat("nope")
				return err
			},
		},

		fsTestCase{
			scenario: "calling StatFile for a path which does not exist returns fs.ErrNotExist",
			function: func(fsys sys.FS) error {
				_, err := fsys.StatFile("nope", 0)
				return err
			},
		},

		fsTestCase{
			scenario: "calling MakeDir for a path which does not exist returns fs.ErrNotExist",
			function: func(fsys sys.FS) error {
				err := fsys.MakeDir("nope/nope/nope", 0755)
				return err
			},
		},

		fsTestCase{
			scenario: "calling ChtimesFile for a path which does not exist returns fs.ErrNotExist",
			function: func(fsys sys.FS) error {
				now := time.Now()
				err := fsys.Chtimes("nope", 0, now, now)
				return err
			},
		},
	)
}

type fileTestCase struct {
	scenario string
	function func(sys.File) error
}

func testFileError(t *testing.T, file sys.File, want error, tests ...fileTestCase) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) {
			assertErrorIs(t, test.function(file), want)
		})
	}
}

func testFileErrNotExist(t *testing.T, file sys.File) {
	t.Helper()
	testFileError(t, file, fs.ErrNotExist,
		fileTestCase{
			scenario: "calling OpenFile for a file which does not exist returns fs.ErrNotExist",
			function: func(file sys.File) error {
				_, err := file.OpenFile("nope", 0, 0)
				return err
			},
		},

		fileTestCase{
			scenario: "calling StatFile for a file which does not exist returns fs.ErrNotExist",
			function: func(file sys.File) error {
				_, err := file.StatFile("nope", 0)
				return err
			},
		},

		fileTestCase{
			scenario: "calling MakeDir for a file which does not exist returns fs.ErrNotExist",
			function: func(file sys.File) error {
				err := file.MakeDir("nope/nope/nope", 0755)
				return err
			},
		},

		fileTestCase{
			scenario: "calling ChtimesFile for a file which does not exist returns fs.ErrNotExist",
			function: func(file sys.File) error {
				now := time.Now()
				err := file.ChtimesFile("nope", 0, now, now)
				return err
			},
		},
	)
}

func testFileErrClosed(t *testing.T, file sys.File) {
	t.Helper()
	assertClose(t, file)

	testFileError(t, file, fs.ErrClosed,
		fileTestCase{
			scenario: "calling Close on a closed file returns fs.ErrClosed",
			function: func(file sys.File) error {
				err := file.Close()
				return err
			},
		},

		fileTestCase{
			scenario: "calling Read on a closed file returns fs.ErrClosed",
			function: func(file sys.File) error {
				_, err := file.Read(nil)
				return err
			},
		},

		fileTestCase{
			scenario: "calling ReadAt on a closed file returns fs.ErrClosed",
			function: func(file sys.File) error {
				_, err := file.ReadAt(nil, 0)
				return err
			},
		},

		fileTestCase{
			scenario: "calling Write on a closed file returns fs.ErrClosed",
			function: func(file sys.File) error {
				_, err := file.Write(nil)
				return err
			},
		},

		fileTestCase{
			scenario: "calling WriteAt on a closed file returns fs.ErrClosed",
			function: func(file sys.File) error {
				_, err := file.WriteAt(nil, 0)
				return err
			},
		},

		fileTestCase{
			scenario: "calling Seek on a closed file returns fs.ErrClosed",
			function: func(file sys.File) error {
				_, err := file.Seek(0, io.SeekStart)
				return err
			},
		},

		fileTestCase{
			scenario: "calling Stat on a closed file returns fs.ErrClosed",
			function: func(file sys.File) error {
				_, err := file.Stat()
				return err
			},
		},

		fileTestCase{
			scenario: "calling ReadDir on a closed file returns fs.ErrClosed",
			function: func(file sys.File) error {
				_, err := file.ReadDir(0)
				return err
			},
		},

		fileTestCase{
			scenario: "calling OpenFile on a closed file returns fs.ErrClosed",
			function: func(file sys.File) error {
				_, err := file.OpenFile("foo", 0, 0)
				return err
			},
		},

		fileTestCase{
			scenario: "calling StatFile on a closed file returns fs.ErrClosed",
			function: func(file sys.File) error {
				_, err := file.StatFile("foo", 0)
				return err
			},
		},

		fileTestCase{
			scenario: "calling MakeDir on a closed file returns fs.ErrClosed",
			function: func(file sys.File) error {
				err := file.MakeDir("foo", 0755)
				return err
			},
		},

		fileTestCase{
			scenario: "calling Chtimes on a closed file returns fs.ErrClosed",
			function: func(file sys.File) error {
				now := time.Now()
				err := file.Chtimes(now, now)
				return err
			},
		},

		fileTestCase{
			scenario: "calling ChtimesFile on a closed file returns fs.ErrClosed",
			function: func(file sys.File) error {
				now := time.Now()
				err := file.ChtimesFile("foo", 0, now, now)
				return err
			},
		},

		fileTestCase{
			scenario: "calling Truncate on a closed file returns fs.ErrClosed",
			function: func(file sys.File) error {
				err := file.Truncate(0)
				return err
			},
		},
	)
}

func assertNewFS(t *testing.T, newFS func() (sys.FS, CloseFS, error)) (sys.FS, CloseFS) {
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

func assertOpenFile(t *testing.T, fsys sys.FS, path string, oflags int, perm fs.FileMode) sys.File {
	t.Helper()
	f, err := fsys.OpenFile(path, oflags, perm)
	assertErrorIs(t, err, nil)
	if (oflags & sys.O_CREATE) != 0 {
		stat := assertFileStat(t, f)
		mode := stat.Mode() & fs.ModePerm
		if mode != perm {
			t.Errorf("file permissions mismatch: want=%s got=%s", perm, mode)
		}
	}
	return f
}

func assertPathData(t *testing.T, fsys sys.FS, path, want string) {
	t.Helper()
	f := assertOpenFile(t, fsys, path, 0, 0)
	defer assertClose(t, f)
	assertFileData(t, f, want)
}

func assertMakeDir(t *testing.T, fsys sys.FS, path string, perm fs.FileMode) {
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

func assertChtimes(t *testing.T, fsys sys.FS, path string, atim, mtim time.Time) {
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

func assertTruncate(t *testing.T, f sys.File, size int64) {
	t.Helper()
	err := f.Truncate(size)
	assertErrorIs(t, err, nil)
	assertFileSize(t, f, size)
}

func assertFileStat(t *testing.T, f sys.File) fs.FileInfo {
	t.Helper()
	s, err := f.Stat()
	assertErrorIs(t, err, nil)
	return s
}

func assertFileData(t *testing.T, f sys.File, want string) {
	t.Helper()
	assertSeek(t, f, 0, io.SeekStart)
	if got := assertRead(t, f); got != want {
		t.Errorf("file content mimatch: want=%q got=%q", want, got)
	}
}

func assertFileSize(t *testing.T, f sys.File, want int64) {
	t.Helper()
	stat := assertFileStat(t, f)
	size := stat.Size()
	if size != want {
		t.Errorf("file size mismatch: want=%d got=%d", want, size)
	}
}
