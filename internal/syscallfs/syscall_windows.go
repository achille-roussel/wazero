package syscallfs

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

const (
	// ERROR_ACCESS_DENIED is a Windows error returned by syscall.Unlink
	// instead of syscall.EPERM
	ERROR_ACCESS_DENIED = syscall.Errno(5)

	// ERROR_ALREADY_EXISTS is a Windows error returned by os.Mkdir
	// instead of syscall.EEXIST
	ERROR_ALREADY_EXISTS = syscall.Errno(183)

	// ERROR_DIRECTORY is a Windows error returned by syscall.Rmdir
	// instead of syscall.ENOTDIR
	ERROR_DIRECTORY = syscall.Errno(267)

	// ERROR_DIR_NOT_EMPTY is a Windows error returned by syscall.Rmdir
	// instead of syscall.ENOTEMPTY
	ERROR_DIR_NOT_EMPTY = syscall.Errno(145)
)

func adjustMkdirError(err error) error {
	// os.Mkdir wraps the syscall error in a path error
	if pe, ok := err.(*fs.PathError); ok && pe.Err == ERROR_ALREADY_EXISTS {
		pe.Err = syscall.EEXIST // adjust it
	}
	return err
}

func adjustRmdirError(err error) error {
	switch err {
	case ERROR_DIRECTORY:
		return syscall.ENOTDIR
	case ERROR_DIR_NOT_EMPTY:
		return syscall.ENOTEMPTY
	}
	return err
}

func adjustUnlinkError(err error) error {
	if err == ERROR_ACCESS_DENIED {
		return syscall.EISDIR
	}
	return err
}

// rename uses os.Rename as `windows.Rename` is internal in Go's source tree.
func rename(source, target string) error {
	sourceStat, err := os.Lstat(source)
	if err != nil {
		return err
	}
	sourceIsDir := sourceStat.IsDir()
	targetIsDir := false

	targetStat, err := os.Lstat(target)
	if err == nil {
		// The target exists, windows typically behaves differently than POSIX
		// systems in this case, so we emulate the POSIX behavior to ensure
		// portability of programs executed by Wazero.
		//
		// Note that we lost atomicity in this case, concurrent operations on
		// the file system will race against one another, resulting in different
		// behavior than the one we are attempting to implement here. This might
		// be mitigated using synchronization strategies, in Wazero or using IPC
		// (not sure if windows gives us the necessary primitives tho). In any
		// case, Wazero is not currently a parallel runtime, and it is unclear
		// whether there is a use case for having multiple WASM applications
		// share a file system in read-write mode, so the races may never be an
		// actual concern in practice.
		targetIsDir = targetStat.IsDir()

		switch {
		case sourceIsDir && !targetIsDir:
			err = syscall.ENOTDIR
		case !sourceIsDir && targetIsDir:
			err = syscall.EISDIR
		default:
			err = os.RemoveAll(target)
		}
		if err != nil {
			return err
		}
	}

	if err = os.Rename(source, target); err != nil {
		if errors.Is(err, ERROR_ACCESS_DENIED) {
			if targetIsDir {
				err = syscall.EISDIR
			} else { // use a mappable code
				err = syscall.EPERM
			}
		}
	}

	return err
}
