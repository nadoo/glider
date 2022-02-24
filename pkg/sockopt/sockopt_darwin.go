package sockopt

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func control(opt *Options) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) (err error) {
		return c.Control(func(fd uintptr) {

			if opt.bindIface != nil {
				switch network {
				case "tcp4", "udp4":
					unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_BOUND_IF, opt.bindIface.Index)
				case "tcp6", "udp6":
					unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_BOUND_IF, opt.bindIface.Index)
				}
			}
			if opt.reuseAddr {
				unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
				unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			}

		})
	}
}
