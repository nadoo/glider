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

// MixedProxy .
type MixedProxy struct {
	sDialer Dialer

	addr   string
	http   *HTTP
	socks5 *SOCKS5
}

// NewMixedProxy returns a mixed proxy.
func NewMixedProxy(addr, user, pass string, sDialer Dialer) (*MixedProxy, error) {
	p := &MixedProxy{
		sDialer: sDialer,
		addr:    addr,
	}

	p.http, _ = NewHTTP(addr, nil, sDialer)
	p.socks5, _ = NewSOCKS5(addr, user, pass, nil, sDialer)

	return p, nil
}

// ListenAndServe .
func (p *MixedProxy) ListenAndServe() {
	l, err := net.Listen("tcp", p.addr)
	if err != nil {
		logf("failed to listen on %s: %v", p.addr, err)
		return
	}

	logf("listening TCP on %s", p.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			logf("failed to accept: %v", err)
			continue
		}

		go p.Serve(c)
	}
}

func (p *MixedProxy) Serve(conn net.Conn) {
	defer conn.Close()

	if c, ok := conn.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	c := newConn(conn)

	if p.socks5 != nil {
		head, err := c.Peek(1)
		if err != nil {
			logf("peek error: %s", err)
			return
		}

		// check socks5, client send socksversion: 5 as the first byte
		if head[0] == socks5Version {
			p.socks5.Serve(c)
			return
		}
	}

	if p.http != nil {
		head, err := c.Peek(8)
		if err != nil {
			logf("peek error: %s", err)
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
