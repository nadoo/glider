package sockopt

import (
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

func BindControl(iface *net.Interface) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) {
			unix.BindToDevice(int(fd), iface.Name)
		})
	}
}
