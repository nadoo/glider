package vmess

import (
	"errors"
	"net"

	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterDialer("vmess", CreateDialer)
}

// Dialer struct
type Dialer struct {
	*VMess
	dialer proxy.Dialer
}

// NewDialer returns a proxy dialer
func NewDialer(s string, dialer proxy.Dialer) (*Dialer, error) {
	h, err := NewVMess(s)
	if err != nil {
		return nil, err
	}

	d := &Dialer{VMess: h, dialer: dialer}
	return d, nil
}

// CreateDialer returns a proxy dialer
func CreateDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewDialer(s, dialer)
}

// Addr returns dialer's address
func (s *Dialer) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// NextDialer returns the next dialer
func (s *Dialer) NextDialer(dstAddr string) proxy.Dialer { return s.dialer.NextDialer(dstAddr) }

// Dial establishes a connection to the addr
func (s *Dialer) Dial(network, addr string) (net.Conn, error) {
	rc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		return nil, err
	}

	return s.client.NewConn(rc, addr)
}

// DialUDP returns a PacketConn to the addr
func (s *Dialer) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("vmess client does not support udp now")
}
