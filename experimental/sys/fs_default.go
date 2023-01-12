//go:build !linux && !darwin

package sys

const (
	O_RDONLY = os.O_RDONLY
	O_WRONLY = os.O_WRONLY
	O_RDWR   = os.O_RDWR
	O_APPEND = os.O_APPEND
	O_CREATE = os.O_CREAT
	O_EXCL   = os.O_EXCL
	O_SYNC   = os.O_SYNC
	O_TRUNC  = os.O_TRUNC

	// Package os does not have these flags, pick values unlikely to conflict.
	O_DIRECTORY = 1 << 30
	O_NOFOLLOW  = 1 << 31

	// Package os does not have O_DSYNC/O_RSYNC, so fallback to O_SYNC.
	O_DSYNC = os.O_SYNC
	O_RSYNC = os.O_SYNC

	openFileReadOnlyFlags = O_RDONLY | O_DIRECTORY | O_NOFOLLOW
)
