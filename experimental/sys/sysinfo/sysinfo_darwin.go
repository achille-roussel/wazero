package sysinfo

import "syscall"

func statDev(stat *syscall.Stat_t) int32 {
	return stat.Dev
}

func statAtime(stat *syscall.Stat_t) (int64, int64) {
	return stat.Atimspec.Unix()
}

func statCtime(stat *syscall.Stat_t) (int64, int64) {
	return stat.Ctimspec.Unix()
}
