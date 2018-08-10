package ss

import (
	"errors"
	"net"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/common/socks"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterDialer("ss", CreateDialer)
}

// Dialer struct
type Dialer struct {
	*SS
	dialer proxy.Dialer
}

// NewDialer returns a proxy dialer
func NewDialer(s string, dialer proxy.Dialer) (*Dialer, error) {
	h, err := NewSS(s)
	if err != nil {
		return nil, err
	}

	d := &Dialer{SS: h, dialer: dialer}
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
	target := socks.ParseAddr(addr)
	if target == nil {
		return nil, errors.New("[ss] unable to parse address: " + addr)
	}

	if network == "uot" {
		target[0] = target[0] | 0x8
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

// DialUDP returns a PacketConn to the addr
func (s *Dialer) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	pc, nextHop, err := s.dialer.DialUDP(network, s.addr)
	if err != nil {
		log.F("[ss] dialudp to %s error: %s", s.addr, err)
		return nil, nil, err
	}

	pkc := NewPktConn(s.PacketConn(pc), nextHop, socks.ParseAddr(addr), true)
	return pkc, nextHop, err
}
