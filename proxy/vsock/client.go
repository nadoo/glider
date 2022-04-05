package vsock

import (
	"net"

	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterDialer("vsock", NewVSockDialer)
}

// NewVSockDialer returns a vm socket dialer.
func NewVSockDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewVSock(s, d, nil)
}

// Dial connects to the address addr on the network net via the proxy.
// NOTE: must be the first dialer in a chain
func (s *vsock) Dial(network, addr string) (net.Conn, error) {
	return Dial(s.cid, s.port)
}

// DialUDP connects to the given address via the proxy.
func (s *vsock) DialUDP(network, addr string) (net.PacketConn, error) {
	return nil, proxy.ErrNotSupported
}
