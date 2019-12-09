package mixed

import (
	"bytes"
	"net"
	"net/url"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/http"
	"github.com/nadoo/glider/proxy/socks5"
)

// https://www.ietf.org/rfc/rfc2616.txt, http methods must be uppercase
var httpMethods = [...][]byte{
	[]byte("GET"),
	[]byte("POST"),
	[]byte("PUT"),
	[]byte("DELETE"),
	[]byte("CONNECT"),
	[]byte("HEAD"),
	[]byte("OPTIONS"),
	[]byte("TRACE"),
	[]byte("PATCH"),
}

// Mixed struct.
type Mixed struct {
	proxy proxy.Proxy
	addr  string

	http   *http.HTTP
	socks5 *socks5.Socks5
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

	m.http, _ = http.NewHTTP(s, nil, p)
	m.socks5, _ = socks5.NewSocks5(s, nil, p)

	return m, nil
}

// NewMixedServer returns a mixed server.
func NewMixedServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewMixed(s, p)
}

// ListenAndServe listens on server's addr and serves connections.
func (m *Mixed) ListenAndServe() {
	go m.socks5.ListenAndServeUDP()

	l, err := net.Listen("tcp", m.addr)
	if err != nil {
		log.F("[mixed] failed to listen on %s: %v", m.addr, err)
		return
	}

	log.F("[mixed] listening TCP on %s", m.addr)

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
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	cc := conn.NewConn(c)

	if m.socks5 != nil {
		head, err := cc.Peek(1)
		if err != nil {
			// log.F("[mixed] socks5 peek error: %s", err)
			return
		}

		// check socks5, client send socksversion: 5 as the first byte
		if head[0] == socks5.Version {
			m.socks5.Serve(cc)
			return
		}
	}

	if m.http != nil {
		head, err := cc.Peek(8)
		if err != nil {
			log.F("[mixed] http peek error: %s", err)
			return
		}

		for _, method := range httpMethods {
			if bytes.HasPrefix(head, method) {
				m.http.Serve(cc)
				return
			}
		}
	}

}
