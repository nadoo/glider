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

// TLS .
type TLS struct {
	dialer proxy.Dialer
	addr   string

	serverName string
	skipVerify bool

	certFile string
	keyFile  string

	server      proxy.Server
	serverProto string
}

func init() {
	proxy.RegisterDialer("tls", NewTLSDialer)
	proxy.RegisterServer("tls", NewTLSTransport)
}

// NewTLS returns a tls proxy.
func NewTLS(s string, dialer proxy.Dialer) (*TLS, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse url err: %s", err)
		return nil, err
	}

	addr := u.Host

	query := u.Query()
	skipVerify := query.Get("skipVerify")

	colonPos := strings.LastIndex(addr, ":")
	if colonPos == -1 {
		colonPos = len(addr)
	}
	serverName := addr[:colonPos]

	p := &TLS{
		dialer:     dialer,
		addr:       addr,
		serverName: serverName,
		skipVerify: false,
		certFile:   "",
		keyFile:    "",
	}

	if skipVerify == "true" {
		p.skipVerify = true
	}

	return p, nil
}

// NewTLSServerTransport returns a tls transport layer before the real server
func NewTLSTransport(s string, dialer proxy.Dialer) (proxy.Server, error) {
	transport := strings.Split(s, ",")

	// prepare transport listener
	if len(transport) != 2 {
		err := fmt.Errorf("malformd listener: %s", s)
		log.F(err.Error())
		return nil, err
	}

	u, err := url.Parse(transport[0])
	if err != nil {
		log.F("parse url err: %s", err)
		return nil, err
	}

	// TODO: cert=&key=
	query := u.Query()

	certFile := query.Get("cert")
	keyFile := query.Get("key")

	addr := u.Host
	colonPos := strings.LastIndex(addr, ":")
	if colonPos == -1 {
		colonPos = len(addr)
	}
	serverName := addr[:colonPos]

	p := &TLS{
		dialer:      dialer,
		addr:        addr,
		serverName:  serverName,
		skipVerify:  false,
		certFile:    certFile,
		keyFile:     keyFile,
		serverProto: transport[1],
	}

	// prepare layer 7 server
	p.server, err = proxy.ServerFromURL(transport[1], dialer)

	return p, nil
}

func (s *TLS) ListenAndServe(c net.Conn) {
	// c for TCP_FAST_OPEN

	var tlsConfig *stdtls.Config

	var ticketKey [32]byte
	copy(ticketKey[:], "f8710951c1f6d0d95a95eed5e99b51f1")

	if s.certFile != "" && s.keyFile != "" {
		cert, err := stdtls.LoadX509KeyPair(s.certFile, s.keyFile)
		if err != nil {
			log.F("unabled load cert: %s, key %s", s.certFile, s.keyFile)
			return
		}

		tlsConfig = &stdtls.Config{
			Certificates:     []stdtls.Certificate{cert},
			MinVersion:       stdtls.VersionTLS12,
			MaxVersion:       stdtls.VersionTLS13,
			SessionTicketKey: ticketKey,
			Accept0RTTData:   true,
		}
	} else {
		tlsConfig = nil
	}

	l, err := stdtls.Listen("tcp", s.addr, tlsConfig)
	if err != nil {
		log.F("failed to listen on tls %s: %v", s.addr, err)
		return
	}

	defer l.Close()

	log.F("listening TCP on %s with TLS", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[https] failed to accept: %v", err)
			continue
		}

		// it's callee's response to decide process request in sync/async mode.
		s.server.ListenAndServe(c)
	}
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
		log.F("[tls] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	conf := &stdtls.Config{
		ServerName:         s.serverName,
		InsecureSkipVerify: s.skipVerify,
		ClientSessionCache: stdtls.NewLRUClientSessionCache(64),
		MinVersion:         stdtls.VersionTLS12,
		MaxVersion:         stdtls.VersionTLS13,
	}

	c := stdtls.Client(cc, conf)
	err = c.Handshake()
	return c, err
}

// DialUDP connects to the given address via the proxy.
func (s *TLS) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, errors.New("tls client does not support udp now")
}
