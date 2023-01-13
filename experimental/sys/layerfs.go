package sys

import (
	"errors"
	"io/fs"
)

// LayerFS construcst a read-only file system exposing a stacked view of
// other file systems.
//
// The underlying file systems are expected to be immutable, or the behavior of
// the layered FS may be undefined.
//
// If a file exists in multiple layers, the last layer containing the file takes
// precedence.
func LayerFS(layers ...ReadFS) ReadFS {
	fs := make(layerFS, len(layers))
	for i := range fs {
		fs[i] = layers[len(layers)-(i+1)]
	}
	return fs
}

type layerFS []ReadFS

func (layers layerFS) Open(name string) (fs.File, error) { return Open(layers, name) }

func (layers layerFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	for _, layer := range layers {
		f, err := layer.OpenFile(name, flags, perm)
		if err != nil {
			if !errors.Is(err, ErrNotExist) {
				return nil, err
			}
		} else {
			return f, nil
		}
	}
	return nil, makePathError("open", name, ErrNotExist)
}
