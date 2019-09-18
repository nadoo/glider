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

// TLS struct
type TLS struct {
	dialer proxy.Dialer
	proxy  proxy.Proxy
	addr   string

	tlsConfig *stdtls.Config

	serverName string
	skipVerify bool

	certFile string
	keyFile  string
	cert     stdtls.Certificate

	server proxy.Server
}

func init() {
	proxy.RegisterDialer("tls", NewTLSDialer)
	proxy.RegisterServer("tls", NewTLSServer)
}

// NewTLS returns a tls proxy struct
func NewTLS(s string, d proxy.Dialer, p proxy.Proxy) (*TLS, error) {
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

	query := u.Query()
	skipVerify := query.Get("skipVerify")
	certFile := query.Get("cert")
	keyFile := query.Get("key")

	t := &TLS{
		dialer:     d,
		proxy:      p,
		serverName: serverName,
		skipVerify: false,
		certFile:   certFile,
		keyFile:    keyFile,
	}

	if skipVerify == "true" {
		t.skipVerify = true
	}

	return t, nil
}

// NewTLSDialer returns a tls proxy dialer
func NewTLSDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	p, err := NewTLS(s, d, nil)
	if err != nil {
		return nil, err
	}

	p.tlsConfig = &stdtls.Config{
		ServerName:         p.serverName,
		InsecureSkipVerify: p.skipVerify,
		ClientSessionCache: stdtls.NewLRUClientSessionCache(64),
		MinVersion:         stdtls.VersionTLS10,
	}

	return p, err
}

// NewTLSServer returns a tls transport layer before the real server
func NewTLSServer(s string, p proxy.Proxy) (proxy.Server, error) {
	transport := strings.Split(s, ",")

	// prepare transport listener
	// TODO: check here
	if len(transport) < 2 {
		return nil, errors.New("[tls] malformd listener:" + s)
	}

	t, err := NewTLS(transport[0], nil, p)
	if err != nil {
		return nil, err
	}

	cert, err := stdtls.LoadX509KeyPair(t.certFile, t.keyFile)
	if err != nil {
		log.F("[tls] unable to load cert: %s, key %s", t.certFile, t.keyFile)
		return nil, err
	}

	t.tlsConfig = &stdtls.Config{
		Certificates: []stdtls.Certificate{cert},
		MinVersion:   stdtls.VersionTLS10,
	}

	t.server, err = proxy.ServerFromURL(transport[1], p)
	if err != nil {
		return nil, err
	}

	return t, nil
}

// ListenAndServe .
func (s *TLS) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.F("[tls] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[tls] listening TCP on %s with TLS", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[tls] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve serves requests
func (s *TLS) Serve(c net.Conn) {
	// we know the internal server will close the connection after serve
	// defer c.Close()

	if s.server != nil {
		cc := stdtls.Server(c, s.tlsConfig)
		s.server.Serve(cc)
	}
}

// Addr returns forwarder's address
func (s *TLS) Addr() string { return s.addr }

// Dial connects to the address addr on the network net via the proxy
func (s *TLS) Dial(network, addr string) (net.Conn, error) {
	cc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[tls] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	c := stdtls.Client(cc, s.tlsConfig)
	err = c.Handshake()
	return c, err
}

// DialUDP connects to the given address via the proxy
func (s *TLS) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("tls client does not support udp now")
}
