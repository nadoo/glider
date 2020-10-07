// protocol spec:
// https://trojan-gfw.github.io/trojan/protocol

package trojan

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"net"
	"net/url"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/socks"
)

// Trojan is a base trojan struct.
type Trojan struct {
	dialer     proxy.Dialer
	proxy      proxy.Proxy
	addr       string
	pass       [56]byte
	serverName string
	skipVerify bool
	tlsConfig  *tls.Config
}

func init() {
	proxy.RegisterDialer("trojan", NewTrojanDialer)
	// proxy.RegisterServer("trojan", NewTrojanServer)
}

// NewTrojan returns a trojan proxy.
func NewTrojan(s string, d proxy.Dialer, p proxy.Proxy) (*Trojan, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[trojan] parse url err: %s", err)
		return nil, err
	}

	query := u.Query()
	t := &Trojan{
		dialer:     d,
		proxy:      p,
		addr:       u.Host,
		skipVerify: query.Get("skipVerify") == "true",
		serverName: query.Get("serverName"),
	}

	if t.serverName == "" {
		host, port, _ := net.SplitHostPort(t.addr)
		if port == "" {
			t.addr = net.JoinHostPort(t.addr, "443")
		}
		t.serverName = host
	}

	// pass
	hash := sha256.New224()
	hash.Write([]byte(u.User.Username()))
	hex.Encode(t.pass[:], hash.Sum(nil))

	t.tlsConfig = &tls.Config{
		ServerName:         t.serverName,
		InsecureSkipVerify: t.skipVerify,
		NextProtos:         []string{"http/1.1"},
		ClientSessionCache: tls.NewLRUClientSessionCache(64),
		MinVersion:         tls.VersionTLS12,
	}

	return t, nil
}

// NewTrojanDialer returns a trojan proxy dialer.
func NewTrojanDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewTrojan(s, d, nil)
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

	tlsConn := tls.Client(rc, s.tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		return nil, err
	}

	buf := pool.GetWriteBuffer()
	defer pool.PutWriteBuffer(buf)

	buf.Write(s.pass[:])
	buf.WriteString("\r\n")

	cmd := socks.CmdConnect
	if network == "udp" {
		cmd = socks.CmdUDPAssociate
	}
	buf.WriteByte(cmd)

	buf.Write(socks.ParseAddr(addr))
	buf.WriteString("\r\n")
	_, err = tlsConn.Write(buf.Bytes())

	return tlsConn, err
}

// DialUDP connects to the given address via the proxy.
func (s *Trojan) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	c, err := s.dial("udp", addr)
	if err != nil {
		return nil, nil, err
	}

	pkc := NewPktConn(c, socks.ParseAddr(addr))
	// TODO: check the addr in return value
	return pkc, nil, nil
}
