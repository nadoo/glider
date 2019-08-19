// +build linux

package utils

import (
	"syscall"

	"github.com/nadoo/glider/common/log"
)

func InitRLimit() {
	var rlim syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlim)
	if err != nil {
		log.F("get rlimit error: " + err.Error())
		return
	}
	rlim.Cur = 1048576
	rlim.Max = 1048576
	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rlim)
	if err != nil {
		log.F("set rlimit error: " + err.Error())
		return
	}
}
