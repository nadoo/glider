package main

import (
	"bytes"
	"net"
)

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

// MixedProxy struct
type MixedProxy struct {
	dialer Dialer

	addr   string
	http   *HTTP
	socks5 *SOCKS5
}

// NewMixedProxy returns a mixed proxy.
func NewMixedProxy(addr, user, pass, rawQuery string, dialer Dialer) (*MixedProxy, error) {
	p := &MixedProxy{
		dialer: dialer,
		addr:   addr,
	}

	p.http, _ = NewHTTP(addr, user, pass, rawQuery, dialer)
	p.socks5, _ = NewSOCKS5(addr, user, pass, dialer)

	return p, nil
}

// ListenAndServe .
func (p *MixedProxy) ListenAndServe() {

	go p.socks5.ListenAndServeUDP()

	l, err := net.Listen("tcp", p.addr)
	if err != nil {
		logf("proxy-mixed failed to listen on %s: %v", p.addr, err)
		return
	}

	logf("proxy-mixed listening TCP on %s", p.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			logf("proxy-mixed failed to accept: %v", err)
			continue
		}

		go p.Serve(c)
	}
}

// Serve .
func (p *MixedProxy) Serve(conn net.Conn) {
	defer conn.Close()

	if c, ok := conn.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	c := newConn(conn)

	if p.socks5 != nil {
		head, err := c.Peek(1)
		if err != nil {
			logf("proxy-mixed peek error: %s", err)
			return
		}

		// check socks5, client send socksversion: 5 as the first byte
		if head[0] == socks5Version {
			p.socks5.ServeTCP(c)
			return
		}
	}

	if p.http != nil {
		head, err := c.Peek(8)
		if err != nil {
			logf("proxy-mixed peek error: %s", err)
			return
		}

		for _, method := range httpMethods {
			if bytes.HasPrefix(head, method) {
				p.http.Serve(c)
				return
			}
		}
	}

}
