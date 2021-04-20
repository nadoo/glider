package ss

import (
	"errors"
	"net"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/protocol/socks"
)

// NewSSDialer returns a ss proxy dialer.
func NewSSDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewSS(s, d, nil)
}

// Addr returns forwarder's address.
func (s *SS) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (s *SS) Dial(network, addr string) (net.Conn, error) {
	target := socks.ParseAddr(addr)
	if target == nil {
		return nil, errors.New("[ss] unable to parse address: " + addr)
	}

	c, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[ss] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	c = s.StreamConn(c)
	if _, err = c.Write(target); err != nil {
		c.Close()
		return nil, err
	}

	return c, err
}

// DialUDP connects to the given address via the proxy.
func (s *SS) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	pc, nextHop, err := s.dialer.DialUDP(network, s.addr)
	if err != nil {
		log.F("[ss] dialudp to %s error: %s", s.addr, err)
		return nil, nil, err
	}

	pkc := NewPktConn(s.PacketConn(pc), nextHop, socks.ParseAddr(addr), true)
	return pkc, nextHop, err
}
