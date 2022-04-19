package vsock

import (
	"net"
	"net/url"
	"strconv"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

type vsock struct {
	dialer    proxy.Dialer
	proxy     proxy.Proxy
	server    proxy.Server
	addr      string
	cid, port uint32
}

// NewVSock returns vm socket proxy.
func NewVSock(s string, d proxy.Dialer, p proxy.Proxy) (*vsock, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[vsock] parse url err: %s", err)
		return nil, err
	}

	v := &vsock{dialer: d, proxy: p, addr: u.Host}
	if hostStr, portStr, _ := net.SplitHostPort(v.addr); portStr != "" {
		if hostStr != "" {
			host, err := strconv.ParseUint(hostStr, 10, 32)
			if err != nil {
				log.F("[vsock] parse cid err: %s", err)
				return nil, err
			}
			v.cid = uint32(host)
		}

		port, err := strconv.ParseUint(portStr, 10, 32)
		if err != nil {
			log.F("[vsock] parse port err: %s", err)
			return nil, err
		}
		v.port = uint32(port)
	}

	return v, nil
}

// Addr returns forwarder's address.
func (s *vsock) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

func init() {
	proxy.AddUsage("vsock", `
VM socket scheme(linux only):
  vsock://[CID]:port

  if you want to listen on any address, just set CID to 4294967295.
`)
}
