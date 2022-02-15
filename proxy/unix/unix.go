package unix

import (
	"net"
	"net/url"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// Unix domain socket struct.
type Unix struct {
	dialer proxy.Dialer
	proxy  proxy.Proxy
	server proxy.Server

	addr  string // addr for tcp
	uaddr *net.UnixAddr

	addru  string // addr for udp (datagram)
	uaddru *net.UnixAddr
}

// NewUnix returns unix domain socket proxy.
func NewUnix(s string, d proxy.Dialer, p proxy.Proxy) (*Unix, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[unix] parse url err: %s", err)
		return nil, err
	}

	unix := &Unix{
		dialer: d,
		proxy:  p,
		addr:   u.Path,
		addru:  u.Path + "u",
	}

	unix.uaddr, err = net.ResolveUnixAddr("unixgram", unix.addr)
	if err != nil {
		return nil, err
	}

	unix.uaddru, err = net.ResolveUnixAddr("unixgram", unix.addru)
	if err != nil {
		return nil, err
	}

	return unix, nil
}

func init() {
	proxy.AddUsage("unix", `
Unix domain socket scheme:
  unix://path
`)
}
