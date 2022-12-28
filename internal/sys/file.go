package sys

import (
	"io"
	"io/fs"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
)

type null struct{}

func (null) Read(b []byte) (int, error)  { return 0, io.EOF }
func (null) Write(b []byte) (int, error) { return len(b), nil }

type reader struct {
	input io.Reader
	file
}

func (r *reader) Read(b []byte) (int, error) { return r.input.Read(b) }

type writer struct {
	output io.Writer
	file
}

func (w *writer) Write(b []byte) (int, error) { return w.output.Write(b) }

type file struct {
	name string
	mode fs.FileMode
}

func (f *file) Name() string               { return f.name }
func (f *file) Stat() (fs.FileInfo, error) { return fileInfo{f}, nil }

func (*file) Close() error                                        { return nil }
func (*file) Read([]byte) (int, error)                            { return 0, fs.ErrInvalid }
func (*file) ReadAt([]byte, int64) (int, error)                   { return 0, fs.ErrInvalid }
func (*file) Write([]byte) (int, error)                           { return 0, fs.ErrInvalid }
func (*file) WriteAt([]byte, int64) (int, error)                  { return 0, fs.ErrInvalid }
func (*file) Seek(int64, int) (int64, error)                      { return 0, fs.ErrInvalid }
func (*file) ReadDir(int) ([]fs.DirEntry, error)                  { return nil, fs.ErrInvalid }
func (*file) OpenFile(string, int, fs.FileMode) (sys.File, error) { return nil, fs.ErrInvalid }
func (*file) StatFile(string, int) (fs.FileInfo, error)           { return nil, fs.ErrInvalid }
func (*file) MakeDir(string, fs.FileMode) error                   { return fs.ErrInvalid }
func (*file) Chtimes(time.Time, time.Time) error                  { return fs.ErrInvalid }
func (*file) ChtimesFile(string, int, time.Time, time.Time) error { return fs.ErrInvalid }
func (*file) Truncate(int64) error                                { return fs.ErrInvalid }

type fileInfo struct{ file *file }

func (info fileInfo) Name() string       { return info.file.name }
func (info fileInfo) Size() int64        { return 0 }
func (info fileInfo) Mode() fs.FileMode  { return info.file.mode }
func (info fileInfo) ModTime() time.Time { return time.Time{} }
func (info fileInfo) IsDir() bool        { return info.Mode().IsDir() }
func (info fileInfo) Sys() interface{}   { return nil }

var (
	_ sys.File = (*reader)(nil)
	_ sys.File = (*writer)(nil)
)
