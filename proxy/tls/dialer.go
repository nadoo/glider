package tls

import (
	stdtls "crypto/tls"
	"errors"
	"net"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterDialer("tls", CreateDialer)
}

// Dialer struct
type Dialer struct {
	*TLS
	dialer proxy.Dialer
}

// NewDialer returns a proxy dialer
func NewDialer(s string, dialer proxy.Dialer) (*Dialer, error) {
	h, err := NewTLS(s)
	if err != nil {
		return nil, err
	}

	d := &Dialer{TLS: h, dialer: dialer}
	return d, nil
}

// CreateDialer returns a proxy dialer
func CreateDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewDialer(s, dialer)
}

// Addr returns dialer's address
func (s *Dialer) Addr() string { return s.addr }

// NextDialer returns the next dialer
func (s *Dialer) NextDialer(dstAddr string) proxy.Dialer { return s.dialer.NextDialer(dstAddr) }

// Dial establishes a connection to the addr
func (s *Dialer) Dial(network, addr string) (net.Conn, error) {
	cc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[tls] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	conf := &stdtls.Config{
		ServerName:         s.serverName,
		InsecureSkipVerify: s.skipVerify,
	}

	c := stdtls.Client(cc, conf)
	err = c.Handshake()
	return c, err
}

// DialUDP returns a PacketConn to the addr
func (s *Dialer) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("tls client does not support udp now")
}
