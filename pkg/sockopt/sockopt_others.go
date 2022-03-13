//go:build !linux && !darwin

package sockopt

import (
	"syscall"
)

func control(opt *Options) func(string, string, syscall.RawConn) error { return nil }
