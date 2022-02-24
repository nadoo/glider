package sockopt

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func control(opt *Options) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) (err error) {
		return c.Control(func(fd uintptr) {

			if opt.bindIface != nil {
				unix.BindToDevice(int(fd), opt.bindIface.Name)
			}
			if opt.reuseAddr {
				unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
				unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			}

		})
	}
}
