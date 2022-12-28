package sys

import "syscall"

const (
	O_RDONLY    = syscall.O_RDONLY
	O_WRONLY    = syscall.O_WRONLY
	O_RDWR      = syscall.O_RDWR
	O_APPEND    = syscall.O_APPEND
	O_CREATE    = syscall.O_CREAT
	O_EXCL      = syscall.O_EXCL
	O_SYNC      = syscall.O_SYNC
	O_TRUNC     = syscall.O_TRUNC
	O_NOFOLLOW  = syscall.O_NOFOLLOW
	O_DIRECTORY = syscall.O_DIRECTORY
	O_DSYNC     = syscall.O_DSYNC
	O_RSYNC     = syscall.O_RSYNC
)
