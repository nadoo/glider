package trojan

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/protocol/socks"
)

// NewClearTextDialer returns a trojan cleartext proxy dialer.
func NewClearTextDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	t, err := NewTrojan(s, d, nil)
	if err != nil {
		return nil, fmt.Errorf("[trojanc] create instance error: %s", err)
	}
	t.withTLS = false
	return t, err
}

// NewTrojanDialer returns a trojan proxy dialer.
func NewTrojanDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	t, err := NewTrojan(s, d, nil)
	if err != nil {
		return nil, fmt.Errorf("[trojan] create instance error: %s", err)
	}

	t.tlsConfig = &tls.Config{
		ServerName:         t.serverName,
		InsecureSkipVerify: t.skipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	if t.certFile != "" {
		certData, err := os.ReadFile(t.certFile)
		if err != nil {
			return nil, fmt.Errorf("[trojan] read cert file error: %s", err)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(certData) {
			return nil, fmt.Errorf("[trojan] can not append cert file: %s", t.certFile)
		}
		t.tlsConfig.RootCAs = certPool
	}

	return t, err
}

// Addr returns forwarder's address.
func (s *Trojan) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (s *Trojan) Dial(network, addr string) (net.Conn, error) {
	return s.dial(network, addr)
}

func (s *Trojan) dial(network, addr string) (net.Conn, error) {
	rc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[trojan]: dial to %s error: %s", s.addr, err)
		return nil, err
	}

	if s.withTLS {
		tlsConn := tls.Client(rc, s.tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			return nil, err
		}
		rc = tlsConn
	}

	buf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(buf)

	buf.Write(s.pass[:])
	buf.WriteString("\r\n")

	cmd := socks.CmdConnect
	if network == "udp" {
		cmd = socks.CmdUDPAssociate
	}
	buf.WriteByte(cmd)

	buf.Write(socks.ParseAddr(addr))
	buf.WriteString("\r\n")
	_, err = rc.Write(buf.Bytes())

	return rc, err
}

// DialUDP connects to the given address via the proxy.
func (s *Trojan) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	c, err := s.dial("udp", addr)
	// TODO: check the addr in return value
	return NewPktConn(c, socks.ParseAddr(addr)), nil, err
}
