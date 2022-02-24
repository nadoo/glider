//go:build !linux && !darwin
// +build !linux,!darwin

package sockopt

import (
	"net"
	"syscall"
)

func control(opt *Options) func(string, string, syscall.RawConn) error { return nil }
