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

func init() {
	proxy.RegisterServer("mixed", NewMixedProxyServer)
}

// https://www.ietf.org/rfc/rfc2616.txt, http methods must be uppercase.
var httpMethods = [...][]byte{
	[]byte("GET"),
	[]byte("POST"),
	[]byte("PUT"),
	[]byte("DELETE"),
	[]byte("CONNECT"),
	[]byte("HEAD"),
	[]byte("OPTIONS"),
	[]byte("TRACE"),
}

// Server struct
type Server struct {
	*proxy.Forwarder
	addr   string
	http   *http.Server
	socks5 *socks5.Server
}

// NewMixedProxy returns a mixed proxy.
func NewMixedProxy(s string, f *proxy.Forwarder) (*Server, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	p := &Server{
		Forwarder: f,
		addr:      u.Host,
	}

	p.http, _ = http.NewServer(s, f)
	p.socks5, _ = socks5.NewServer(s, f)

	return p, nil
}

// NewMixedProxyServer returns a mixed proxy server.
func NewMixedProxyServer(s string, f *proxy.Forwarder) (proxy.Server, error) {
	return NewMixedProxy(s, f)
}

// ListenAndServe .
func (p *Server) ListenAndServe() {

	go p.socks5.ListenAndServeUDP()

	l, err := net.Listen("tcp", p.addr)
	if err != nil {
		log.F("[mixed] failed to listen on %s: %v", p.addr, err)
		return
	}

	log.F("[mixed] listening TCP on %s", p.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[mixed] failed to accept: %v", err)
			continue
		}

		go p.Serve(c)
	}
}

// Serve .
func (p *Server) Serve(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	cc := conn.NewConn(c)

	if p.socks5 != nil {
		head, err := cc.Peek(1)
		if err != nil {
			log.F("[mixed] peek error: %s", err)
			return
		}

		// check socks5, client send socksversion: 5 as the first byte
		if head[0] == socks5.Version {
			p.socks5.ServeTCP(cc)
			return
		}
	}

	if p.http != nil {
		head, err := cc.Peek(8)
		if err != nil {
			log.F("[mixed] peek error: %s", err)
			return
		}

		for _, method := range httpMethods {
			if bytes.HasPrefix(head, method) {
				p.http.Serve(cc)
				return
			}
		}
	}

}
