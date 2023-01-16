package sysinfo

import (
	"io/fs"
	"syscall"
	"time"
)

func makeMode(fileMode fs.FileMode) (mode uint32) {
	mode = uint32(fileMode.Perm())
	switch fileMode.Type() {
	case fs.ModeDevice:
		mode |= syscall.S_IFBLK
	case fs.ModeDevice | fs.ModeCharDevice:
		mode |= syscall.S_IFCHR
	case fs.ModeDir:
		mode |= syscall.S_IFDIR
	case fs.ModeNamedPipe:
		mode |= syscall.S_IFIFO
	case fs.ModeSymlink:
		mode |= syscall.S_IFLNK
	case fs.ModeSocket:
		mode |= syscall.S_IFSOCK
	default:
		mode |= syscall.S_IFREG
	}
	if (fileMode & fs.ModeSetgid) != 0 {
		mode |= syscall.S_ISGID
	}
	if (fileMode & fs.ModeSetuid) != 0 {
		mode |= syscall.S_ISUID
	}
	if (fileMode & fs.ModeSticky) != 0 {
		mode |= syscall.S_ISVTX
	}
	return mode
}

func mode(info fs.FileInfo) uint32 {
	if stat := stat(info); stat != nil {
		return statMode(stat)
	}
	return makeMode(info.Mode())
}

func uid(info fs.FileInfo) uint32 {
	if stat := stat(info); stat != nil {
		return statUid(stat)
	}
	return 0
}

func gid(info fs.FileInfo) uint32 {
	if stat := stat(info); stat != nil {
		return statGid(stat)
	}
	return 0
}

func ino(info fs.FileInfo) uint64 {
	if stat := stat(info); stat != nil {
		return statIno(stat)
	}
	return 0
}

func nlink(info fs.FileInfo) uint64 {
	if stat := stat(info); stat != nil {
		return statNlink(stat)
	}
	return 1
}

func device(info fs.FileInfo) uint64 {
	if stat := stat(info); stat != nil {
		return statDev(stat)
	}
	return 0
}

func mtime(info fs.FileInfo) time.Time {
	if stat := stat(info); stat != nil {
		return time.Unix(statMtime(stat))
	}
	return time.Time{}
}

func atime(info fs.FileInfo) time.Time {
	if stat := stat(info); stat != nil {
		return time.Unix(statAtime(stat))
	}
	return time.Time{}
}

func ctime(info fs.FileInfo) time.Time {
	if stat := stat(info); stat != nil {
		return time.Unix(statCtime(stat))
	}
	return time.Time{}
}

func stat(info fs.FileInfo) *syscall.Stat_t {
	stat, _ := info.Sys().(*syscall.Stat_t)
	return stat
}
