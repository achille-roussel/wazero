package sys

import (
	"io/fs"
	"syscall"
)

func device(info fs.FileInfo) dev_t {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return dev_t(stat.Dev)
	}
	return 0
}
