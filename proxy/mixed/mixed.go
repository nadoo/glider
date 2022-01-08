package mixed

import (
	"net"
	"net/url"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/http"
	"github.com/nadoo/glider/proxy/socks5"
)

// Mixed struct.
type Mixed struct {
	proxy proxy.Proxy
	addr  string

	httpServer   *http.HTTP
	socks5Server *socks5.Socks5
}

func init() {
	proxy.RegisterServer("mixed", NewMixedServer)
}

// NewMixed returns a mixed proxy.
func NewMixed(s string, p proxy.Proxy) (*Mixed, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	m := &Mixed{
		proxy: p,
		addr:  u.Host,
	}

	m.httpServer, err = http.NewHTTP(s, nil, p)
	if err != nil {
		return nil, err
	}

	m.socks5Server, err = socks5.NewSocks5(s, nil, p)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// NewMixedServer returns a mixed server.
func NewMixedServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewMixed(s, p)
}

// ListenAndServe listens on server's addr and serves connections.
func (m *Mixed) ListenAndServe() {
	go m.socks5Server.ListenAndServeUDP()

	l, err := net.Listen("tcp", m.addr)
	if err != nil {
		log.Fatalf("[mixed] failed to listen on %s: %v", m.addr, err)
		return
	}

	log.F("[mixed] http & socks5 server listening TCP on %s", m.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[mixed] failed to accept: %v", err)
			continue
		}

		go m.Serve(c)
	}
}

// Serve serves connections.
func (m *Mixed) Serve(c net.Conn) {
	conn := proxy.NewConn(c)
	if head, err := conn.Peek(1); err == nil {
		if head[0] == socks5.Version {
			m.socks5Server.Serve(conn)
			return
		}
	}
	m.httpServer.Serve(conn)
}
