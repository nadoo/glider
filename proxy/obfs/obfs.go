// Package obfs implements simple-obfs of ss
package obfs

import (
	"errors"
	"net"
	"net/url"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// Obfs struct
type Obfs struct {
	dialer proxy.Dialer
	addr   string

	obfsType string
	obfsHost string
	obfsURI  string
	obfsUA   string

	obfsConn func(c net.Conn) (net.Conn, error)
}

func init() {
	proxy.RegisterDialer("simple-obfs", NewObfsDialer)
}

// NewObfs returns a proxy struct
func NewObfs(s string, dialer proxy.Dialer) (*Obfs, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse url err: %s", err)
		return nil, err
	}

	addr := u.Host

	query := u.Query()
	obfsType := query.Get("type")
	if obfsType == "" {
		obfsType = "http"
	}

	obfsHost := query.Get("host")
	if obfsHost == "" {
		return nil, errors.New("[obfs] host cannot be null")
	}

	obfsURI := query.Get("uri")
	if obfsURI == "" {
		obfsURI = "/"
	}

	obfsUA := query.Get("ua")
	if obfsUA == "" {
		obfsUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/71.0.3578.80 Safari/537.36"
	}

	p := &Obfs{
		dialer:   dialer,
		addr:     addr,
		obfsType: obfsType,
		obfsHost: obfsHost,
		obfsURI:  obfsURI,
		obfsUA:   obfsUA,
	}

	switch obfsType {
	case "http":
		httpObfs := NewHTTPObfs(obfsHost, obfsURI, obfsUA)
		p.obfsConn = httpObfs.NewConn
	case "tls":
		tlsObfs := NewTLSObfs(obfsHost)
		p.obfsConn = tlsObfs.NewConn
	default:
		return nil, errors.New("[obfs] unknown obfs type: " + obfsType)
	}

	return p, nil
}

// NewObfsDialer returns a proxy dialer
func NewObfsDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewObfs(s, dialer)
}

// Addr returns forwarder's address
func (s *Obfs) Addr() string { return s.addr }

// NextDialer returns the next dialer
func (s *Obfs) NextDialer(dstAddr string) proxy.Dialer { return s.dialer.NextDialer(dstAddr) }

// Dial connects to the address addr on the network net via the proxy
func (s *Obfs) Dial(network, addr string) (net.Conn, error) {
	c, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[obfs] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	return s.obfsConn(c)
}

// DialUDP connects to the given address via the proxy
func (s *Obfs) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("obfs client does not support udp now")
}
