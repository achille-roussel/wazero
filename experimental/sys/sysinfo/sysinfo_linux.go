package sysinfo

import "syscall"

func statDev(stat *syscall.Stat_t) uint64 {
	return stat.Dev
}

func statAtime(stat *syscall.Stat_t) (int64, int64) {
	return stat.Atim.Unix()
}

func statCtime(stat *syscall.Stat_t) (int64, int64) {
	return stat.Ctim.Unix()
}
