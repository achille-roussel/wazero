//go:build !linux && !windows

package sys

import "syscall"

func unlink(path string) error { return syscall.Unlink(path) }
