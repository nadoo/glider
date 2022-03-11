package tls

import (
	stdtls "crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
)

// TLS struct.
type TLS struct {
	dialer proxy.Dialer
	proxy  proxy.Proxy
	addr   string

	config *stdtls.Config

	serverName string
	skipVerify bool

	certFile string
	keyFile  string

	alpn []string

	server proxy.Server
}

func init() {
	proxy.RegisterDialer("tls", NewTLSDialer)
	proxy.RegisterServer("tls", NewTLSServer)
}

// NewTLS returns a tls struct.
func NewTLS(s string, d proxy.Dialer, p proxy.Proxy) (*TLS, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[tls] parse url err: %s", err)
		return nil, err
	}

	query := u.Query()
	t := &TLS{
		dialer:     d,
		proxy:      p,
		addr:       u.Host,
		serverName: query.Get("serverName"),
		skipVerify: query.Get("skipVerify") == "true",
		certFile:   query.Get("cert"),
		keyFile:    query.Get("key"),
		alpn:       query["alpn"],
	}

	if t.addr != "" {
		if _, port, _ := net.SplitHostPort(t.addr); port == "" {
			t.addr = net.JoinHostPort(t.addr, "443")
		}
		if t.serverName == "" {
			t.serverName = t.addr[:strings.LastIndex(t.addr, ":")]
		}
	}

	return t, nil
}

// NewTLSDialer returns a tls dialer.
func NewTLSDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	t, err := NewTLS(s, d, nil)
	if err != nil {
		return nil, err
	}

	t.config = &stdtls.Config{
		ServerName:         t.serverName,
		InsecureSkipVerify: t.skipVerify,
		NextProtos:         t.alpn,
		MinVersion:         stdtls.VersionTLS12,
	}

	if t.certFile != "" {
		certData, err := os.ReadFile(t.certFile)
		if err != nil {
			return nil, fmt.Errorf("[tls] read cert file error: %s", err)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(certData) {
			return nil, fmt.Errorf("[tls] can not append cert file: %s", t.certFile)
		}
		t.config.RootCAs = certPool
	}

	return t, err
}

// NewTLSServer returns a tls transport layer before the real server.
func NewTLSServer(s string, p proxy.Proxy) (proxy.Server, error) {
	schemes := strings.SplitN(s, ",", 2)
	t, err := NewTLS(schemes[0], nil, p)
	if err != nil {
		return nil, err
	}

	if t.certFile == "" || t.keyFile == "" {
		return nil, errors.New("[tls] cert and key file path must be spcified")
	}

	cert, err := stdtls.LoadX509KeyPair(t.certFile, t.keyFile)
	if err != nil {
		log.F("[tls] unable to load cert: %s, key %s", t.certFile, t.keyFile)
		return nil, err
	}

	t.config = &stdtls.Config{
		Certificates: []stdtls.Certificate{cert},
		NextProtos:   t.alpn,
		MinVersion:   stdtls.VersionTLS12,
	}

	if len(schemes) > 1 {
		t.server, err = proxy.ServerFromURL(schemes[1], p)
		if err != nil {
			return nil, err
		}
	}

	return t, nil
}

// ListenAndServe listens on server's addr and serves connections.
func (s *TLS) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.Fatalf("[tls] failed to listen on %s: %v", s.addr, err)
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

// Serve serves a connection.
func (s *TLS) Serve(cc net.Conn) {
	c := stdtls.Server(cc, s.config)

	if s.server != nil {
		s.server.Serve(c)
		return
	}

	defer c.Close()

	rc, dialer, err := s.proxy.Dial("tcp", "")
	if err != nil {
		log.F("[tls] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), s.addr, dialer.Addr(), err)
		s.proxy.Record(dialer, false)
		return
	}
	defer rc.Close()

	log.F("[tls] %s <-> %s", c.RemoteAddr(), dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[tls] %s <-> %s, relay error: %v", c.RemoteAddr(), dialer.Addr(), err)
		// record remote conn failure only
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

// Addr returns forwarder's address.
func (s *TLS) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (s *TLS) Dial(network, addr string) (net.Conn, error) {
	cc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		log.F("[tls] dial to %s error: %s", s.addr, err)
		return nil, err
	}

	c := stdtls.Client(cc, s.config)
	err = c.Handshake()
	return c, err
}

// DialUDP connects to the given address via the proxy.
func (s *TLS) DialUDP(network, addr string) (net.PacketConn, error) {
	return nil, proxy.ErrNotSupported
}

func init() {
	proxy.AddUsage("tls", `
TLS client scheme:
  tls://host:port[?serverName=SERVERNAME][&skipVerify=true][&cert=PATH][&alpn=proto1][&alpn=proto2]
  
Proxy over tls client:
  tls://host:port[?skipVerify=true][&serverName=SERVERNAME],scheme://
  tls://host:port[?skipVerify=true],http://[user:pass@]
  tls://host:port[?skipVerify=true],socks5://[user:pass@]
  tls://host:port[?skipVerify=true],vmess://[security:]uuid@?alterID=num
  
TLS server scheme:
  tls://host:port?cert=PATH&key=PATH[&alpn=proto1][&alpn=proto2]
  
Proxy over tls server:
  tls://host:port?cert=PATH&key=PATH,scheme://
  tls://host:port?cert=PATH&key=PATH,http://
  tls://host:port?cert=PATH&key=PATH,socks5://
  tls://host:port?cert=PATH&key=PATH,ss://method:pass@
`)
}
