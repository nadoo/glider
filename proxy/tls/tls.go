package tls

import (
	stdtls "crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// TLS struct
type TLS struct {
	dialer proxy.Dialer
	addr   string

	serverName string
	skipVerify bool

	certFile string
	keyFile  string

	server proxy.Server
}

func init() {
	proxy.RegisterDialer("tls", NewTLSDialer)
	proxy.RegisterServer("tls", NewTLSServer)
}

// NewTLS returns a tls proxy
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

	query := u.Query()
	skipVerify := query.Get("skipVerify")
	certFile := query.Get("cert")
	keyFile := query.Get("key")

	p := &TLS{
		dialer:     dialer,
		addr:       addr,
		serverName: serverName,
		skipVerify: false,
		certFile:   certFile,
		keyFile:    keyFile,
	}

	if skipVerify == "true" {
		p.skipVerify = true
	}

	return p, nil
}

// NewTLSDialer returns a tls proxy dialer.
func NewTLSDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewTLS(s, dialer)
}

// NewTLSServer returns a tls transport layer before the real server
func NewTLSServer(s string, dialer proxy.Dialer) (proxy.Server, error) {
	transport := strings.Split(s, ",")

	// prepare transport listener
	// TODO: check here
	if len(transport) < 2 {
		err := fmt.Errorf("[tls] malformd listener: %s", s)
		log.F(err.Error())
		return nil, err
	}

	p, err := NewTLS(transport[0], dialer)
	if err != nil {
		return nil, err
	}

	p.server, err = proxy.ServerFromURL(transport[1], dialer)

	return p, err
}

// ListenAndServe .
func (s *TLS) ListenAndServe() {
	cert, err := stdtls.LoadX509KeyPair(s.certFile, s.keyFile)
	if err != nil {
		log.F("[tls] unabled load cert: %s, key %s", s.certFile, s.keyFile)
		return
	}

	tlsConfig := &stdtls.Config{
		Certificates: []stdtls.Certificate{cert},
		MinVersion:   stdtls.VersionTLS10,
		MaxVersion:   stdtls.VersionTLS12,
	}

	l, err := stdtls.Listen("tcp", s.addr, tlsConfig)
	if err != nil {
		log.F("[tls] failed to listen on tls %s: %v", s.addr, err)
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

// Serve .
func (s *TLS) Serve(c net.Conn) {
	// TODO: check here
	s.server.Serve(c)
}

// Addr returns forwarder's address
func (s *TLS) Addr() string { return s.addr }

// NextDialer returns the next dialer
func (s *TLS) NextDialer(dstAddr string) proxy.Dialer { return s.dialer.NextDialer(dstAddr) }

// Dial connects to the address addr on the network net via the proxy.
func (s *TLS) Dial(network, addr string) (net.Conn, error) {
	cc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[tls] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	conf := &stdtls.Config{
		ServerName:         s.serverName,
		InsecureSkipVerify: s.skipVerify,
		ClientSessionCache: stdtls.NewLRUClientSessionCache(64),
		MinVersion:         stdtls.VersionTLS10,
		MaxVersion:         stdtls.VersionTLS12,
	}

	c := stdtls.Client(cc, conf)
	err = c.Handshake()
	return c, err
}

// DialUDP connects to the given address via the proxy.
func (s *TLS) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("tls client does not support udp now")
}
