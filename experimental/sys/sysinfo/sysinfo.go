// Package sysinfo exports functions to extract system-dependent information
// from a fs.FileInfo.
package sysinfo

import (
	"io/fs"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
)

// Device returns the device embedded into the given file info.
// If there were no devices, zero is returned.
func Device(info fs.FileInfo) sys.Device { return sys.Device(device(info)) }

// AccessTime returns the file access time.
// If access time is not supported, the zero value of time.Time is returned.
func AccessTime(info fs.FileInfo) time.Time { return atime(info) }

// ChangeTime returns the file access time.
// If change time is not supported, the zero value of time.Time is returned.
func ChangeTime(info fs.FileInfo) time.Time { return ctime(info) }
