package tls

import (
	stdtls "crypto/tls"
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// TLS .
type TLS struct {
	dialer proxy.Dialer
	addr   string

	serverName string
}

func init() {
	proxy.RegisterDialer("tls", NewTLSDialer)
}

// NewTLS returns a tls proxy.
func NewTLS(s string, dialer proxy.Dialer) (*TLS, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse url err: %s", err)
		return nil, err
	}

	addr := u.Host

	colonPos := strings.LastIndex(addr, ":")
	if colonPos == -1 {
		colonPos = len(addr)
	}
	serverName := addr[:colonPos]

	p := &TLS{
		dialer:     dialer,
		addr:       addr,
		serverName: serverName,
	}

	return p, nil
}

// NewTLSDialer returns a tls proxy dialer.
func NewTLSDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewTLS(s, dialer)
}

// Addr returns forwarder's address
func (s *TLS) Addr() string { return s.addr }

// NextDialer returns the next dialer
func (s *TLS) NextDialer(dstAddr string) proxy.Dialer { return s.dialer.NextDialer(dstAddr) }

// Dial connects to the address addr on the network net via the proxy.
func (s *TLS) Dial(network, addr string) (net.Conn, error) {
	cc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("proxy-tls dial to %s error: %s", s.addr, err)
		return nil, err
	}

	conf := &stdtls.Config{
		ServerName: s.serverName,
		//InsecureSkipVerify: true,
	}

	c := stdtls.Client(cc, conf)
	err = c.Handshake()
	return c, err
}

// DialUDP connects to the given address via the proxy.
func (s *TLS) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("tls client does not support udp now")
}
