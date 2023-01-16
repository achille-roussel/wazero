package sysinfo

import "syscall"

func statMode(stat *syscall.Stat_t) uint32 {
	return stat.Mode
}

func statIno(stat *syscall.Stat_t) uint64 {
	return stat.Ino
}

func statNlink(stat *syscall.Stat_t) uint64 {
	return stat.Nlink
}

func statDev(stat *syscall.Stat_t) uint64 {
	return stat.Dev
}

func statMtime(stat *syscall.Stat_t) (int64, int64) {
	return stat.Mtim.Unix()
}

func statAtime(stat *syscall.Stat_t) (int64, int64) {
	return stat.Atim.Unix()
}

func statCtime(stat *syscall.Stat_t) (int64, int64) {
	return stat.Ctim.Unix()
}
