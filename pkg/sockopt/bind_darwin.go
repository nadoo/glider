package sockopt

import (
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

func BindControl(iface *net.Interface) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) (err error) {
		return c.Control(func(fd uintptr) {
			switch network {
			case "tcp4", "udp4":
				err = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_BOUND_IF, iface.Index)
			case "tcp6", "udp6":
				err = unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_BOUND_IF, iface.Index)
			}
		})
	}
}
