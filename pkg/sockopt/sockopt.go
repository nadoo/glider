package sockopt

import (
	"net"
	"syscall"
)

// Options is the options struct.
type Options struct {
	bindIface *net.Interface
	reuseAddr bool
}

// Option is the function paramater.
type Option func(opts *Options)

// Bind sets the bind interface option.
func Bind(intf *net.Interface) Option { return func(opts *Options) { opts.bindIface = intf } }

// ReuseAddr sets the reuse addr option.
func ReuseAddr() Option { return func(opts *Options) { opts.reuseAddr = true } }

// Control returns a control function for the net.Dialer and net.ListenConfig.
func Control(opts ...Option) func(network, address string, c syscall.RawConn) error {
	option := &Options{}
	for _, opt := range opts {
		opt(option)
	}

	return control(option)
}
