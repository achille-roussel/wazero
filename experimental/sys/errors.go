package sys

import (
	"errors"
	"io/fs"
	"syscall"
)

var (
	ErrClosed         error = fs.ErrClosed
	ErrInvalid        error = fs.ErrInvalid
	ErrExist          error = fs.ErrExist
	ErrNotExist       error = fs.ErrNotExist
	ErrNotEmpty       error = syscall.ENOTEMPTY
	ErrNotDirectory   error = syscall.ENOTDIR
	ErrNotImplemented error = syscall.ENOSYS
	ErrNotSupported   error = syscall.ENOTSUP
	ErrPermission     error = fs.ErrPermission
	ErrReadOnly       error = syscall.EROFS
	ErrLoop           error = syscall.ELOOP
	ErrDevice         error = syscall.ENXIO
)

func unwrap(err error) error {
	for {
		if cause := errors.Unwrap(err); cause != nil {
			err = cause
		} else {
			return err
		}
	}
}

func newPathError(op, path string, err error) error {
	return &fs.PathError{Op: op, Path: path, Err: err}
}

func makePathError(op, path string, err error) error {
	pe, _ := err.(*fs.PathError)
	if pe == nil {
		pe = new(fs.PathError)
	} else {
		err = pe.Err
	}
	switch e := err.(type) {
	case syscall.Errno:
		switch e {
		case syscall.EACCES:
			err = ErrPermission
		case syscall.EINVAL:
			err = ErrInvalid
		case syscall.EPERM:
			err = ErrPermission
		case syscall.EEXIST:
			err = ErrExist
		case syscall.ENOENT:
			err = ErrNotExist
		}
	}
	pe.Op, pe.Path, pe.Err = op, path, err
	return pe
}
