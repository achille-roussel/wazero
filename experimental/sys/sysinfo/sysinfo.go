// Package sysinfo exports functions to extract system-dependent information
// from a fs.FileInfo.
package sysinfo

import (
	"io/fs"
	"time"
)

// FileMode converts the given fs.FileMode to the system representation.
func FileMode(mode fs.FileMode) uint32 { return makeMode(mode) }

// Mode returns the system dependent bits composing the file mode.
// If there is none, it is computed by FileMode(info.Mode()).
func Mode(info fs.FileInfo) uint32 { return mode(info) }

// Ino returns the file inode number.
// If there is none, zero is returned.
func Ino(info fs.FileInfo) uint64 { return ino(info) }

// Nlink returns the number of hard links.
// If the information is unknown, the function returns 1.
func Nlink(info fs.FileInfo) uint64 { return nlink(info) }

// Device returns the device embedded into the given file info.
// If there were no devices, zero is returned.
func Device(info fs.FileInfo) uint64 { return device(info) }

// ModTime returns the file modification time.
// If it not available on the embedded system information, the function falls
// back to calling info.ModTime().
func ModTime(info fs.FileInfo) time.Time { return mtime(info) }

// AccessTime returns the file access time.
// If access time is not supported, the zero value of time.Time is returned.
func AccessTime(info fs.FileInfo) time.Time { return atime(info) }

// ChangeTime returns the file access time.
// If change time is not supported, the zero value of time.Time is returned.
func ChangeTime(info fs.FileInfo) time.Time { return ctime(info) }
