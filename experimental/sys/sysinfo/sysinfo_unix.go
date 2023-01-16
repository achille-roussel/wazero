package sysinfo

import (
	"io/fs"
	"syscall"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
)

func device(info fs.FileInfo) sys.Device {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return sys.Device(statDev(stat))
	}
	return 0
}

func atime(info fs.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(statAtime(stat))
	}
	return time.Time{}
}

func ctime(info fs.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(statCtime(stat))
	}
	return time.Time{}
}
