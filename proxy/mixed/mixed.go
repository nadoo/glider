package mixed

import (
	"bytes"
	"net"
	"net/url"

	"github.com/nadoo/glider/log"
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

	cc := proxy.NewConn(c)
	head, err := cc.Peek(1)
	if err != nil {
		// log.F("[mixed] socks5 peek error: %s", err)
		return
	}

	// check socks5, client send socksversion: 5 as the first byte
	if head[0] == socks5.Version {
		m.socks5Server.Serve(cc)
		return
	}

	head, err = cc.Peek(8)
	if err != nil {
		log.F("[mixed] http peek error: %s", err)
		return
	}

	for _, method := range http.Methods {
		if bytes.HasPrefix(head, method) {
			m.httpServer.Serve(cc)
			return
		}
	}

	log.F("[mixed] unknown request from %s, ignored", c.RemoteAddr())
}
