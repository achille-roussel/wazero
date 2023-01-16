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

func makePathError(op, name string, err error) error {
	if e := errors.Unwrap(err); e != nil {
		err = e
	}

	switch e := err.(type) {
	case syscall.Errno:
		switch e {
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

	return &fs.PathError{Op: op, Path: name, Err: err}
}
