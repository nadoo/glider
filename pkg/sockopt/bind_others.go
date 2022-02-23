//go:build !linux && !darwin
// +build !linux,!darwin

package sockopt

import (
	"net"
	"syscall"
)

func BindControl(iface *net.Interface) func(string, string, syscall.RawConn) error { return nil }
