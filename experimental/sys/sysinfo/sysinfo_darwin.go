package sysinfo

import "syscall"

func statMode(stat *syscall.Stat_t) uint32 {
	return uint32(stat.Mode)
}

func statUid(stat *syscall.Stat_t) uint32 {
	return stat.Uid
}

func statGid(stat *syscall.Stat_t) uint32 {
	return stat.Gid
}

func statIno(stat *syscall.Stat_t) uint64 {
	return stat.Ino
}

func statNlink(stat *syscall.Stat_t) uint64 {
	return uint64(stat.Nlink)
}

func statDev(stat *syscall.Stat_t) uint64 {
	return uint64(stat.Dev)
}

func statMtime(stat *syscall.Stat_t) (int64, int64) {
	return stat.Mtimspec.Unix()
}

func statAtime(stat *syscall.Stat_t) (int64, int64) {
	return stat.Atimspec.Unix()
}

func statCtime(stat *syscall.Stat_t) (int64, int64) {
	return stat.Ctimspec.Unix()
}
