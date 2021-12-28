package proxy

import (
	"net"
	"syscall"
)

func bind(dialer *net.Dialer, iface *net.Interface) {
	dialer.Control = func(network, address string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) {
			syscall.BindToDevice(int(fd), iface.Name)
		})
	}
}
