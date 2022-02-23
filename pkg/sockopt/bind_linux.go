package sockopt

import (
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

func BindControl(iface *net.Interface) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) (err error) {
		return c.Control(func(fd uintptr) {
			err = unix.BindToDevice(int(fd), iface.Name)
		})
	}
}