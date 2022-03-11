package unix

import (
	"net"
	"os"

	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterDialer("unix", NewUnixDialer)
}

// NewUnixDialer returns a unix domain socket dialer.
func NewUnixDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewUnix(s, d, nil)
}

// Addr returns forwarder's address.
func (s *Unix) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
// NOTE: must be the first dialer in a chain
func (s *Unix) Dial(network, addr string) (net.Conn, error) {
	return net.Dial("unix", s.addr)
}

// DialUDP connects to the given address via the proxy.
// NOTE: must be the first dialer in a chain
func (s *Unix) DialUDP(network, addr string) (net.PacketConn, error) {
	laddru := s.addru + "_" + addr
	os.Remove(laddru)

	luaddru, err := net.ResolveUnixAddr("unixgram", laddru)
	if err != nil {
		return nil, err
	}

	pc, err := net.ListenUnixgram("unixgram", luaddru)
	if err != nil {
		return nil, err
	}

	return &PktConn{pc, laddru, luaddru, s.uaddru}, nil
}

// PktConn .
type PktConn struct {
	*net.UnixConn
	addr      string
	uaddr     *net.UnixAddr
	writeAddr *net.UnixAddr
}

// ReadFrom overrides the original function from net.PacketConn.
func (pc *PktConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, _, err := pc.UnixConn.ReadFrom(b)
	return n, pc.uaddr, err
}

// WriteTo overrides the original function from net.PacketConn.
func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return pc.UnixConn.WriteTo(b, pc.writeAddr)
}

// Close overrides the original function from net.PacketConn.
func (pc *PktConn) Close() error {
	pc.UnixConn.Close()
	os.Remove(pc.addr)
	return nil
}
