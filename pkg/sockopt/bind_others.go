//go:build !linux
// +build !linux

package sockopt

import (
	"net"
	"syscall"
)

func BindControl(iface *net.Interface) func(string, string, syscall.RawConn) error { return nil }
