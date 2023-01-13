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

func (layers layerFS) OpenFile(name string, flags int, perm fs.FileMode) (File, error) {
	return layerFSLookup(layers, "open", name, func(layer ReadFS, name string) (File, error) {
		return layer.OpenFile(name, flags, perm)
	})
}

func (layers layerFS) Open(name string) (fs.File, error) {
	return layerFSLookup(layers, "open", name, ReadFS.Open)
}

func (layers layerFS) Stat(name string) (fs.FileInfo, error) {
	return layerFSLookup(layers, "stat", name, ReadFS.Stat)
}

func (layers layerFS) Readlink(name string) (string, error) {
	return layerFSLookup(layers, "readlink", name, ReadFS.Readlink)
}

func layerFSLookup[F func(ReadFS, string) (R, error), R any](layers layerFS, op, name string, do F) (ret R, err error) {
	for _, layer := range layers {
		v, err := do(layer, name)
		if err != nil {
			if !errors.Is(err, ErrNotExist) {
				return ret, err
			}
		} else {
			return v, nil
		}
	}
	return ret, makePathError(op, name, ErrNotExist)
}
