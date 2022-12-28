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
	// Not following symlinks or opening directories only is not supported by
	// the os package, and it is unclear at this point if the standard path
	// resolution of unix platforms is compatible with wasi; for example, does
	// not specifying lookupflags::symlink_follow mean that none of the links
	// in the path are followed, or does it mean that only the last link will
	// not be followed (like it does on linux)?
	//
	// We may have to implement path resolution in user space entirely, in
	// which case we could also default to not following links and checking
	// if the path target is a directory.
	O_NOFOLLOW  = 0
	O_DIRECTORY = 0
	// Package os does not have O_DSYNC/O_RSYNC, so fallback to O_SYNC.
	O_DSYNC = os.O_SYNC
	O_RSYNC = os.O_SYNC
)
