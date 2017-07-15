package main

import (
	"bytes"
	"net"
)

// https://www.ietf.org/rfc/rfc2616.txt, http methods must be uppercase.
var httpMethods = [][]byte{
	[]byte("GET"),
	[]byte("POST"),
	[]byte("PUT"),
	[]byte("DELETE"),
	[]byte("CONNECT"),
	[]byte("HEAD"),
	[]byte("OPTIONS"),
	[]byte("TRACE"),
}

// mixedproxy
type mixedproxy struct {
	Proxy
	http   Proxy
	socks5 Proxy
	ss     Proxy
}

// MixedProxy returns a mixed proxy.
func MixedProxy(network, addr, user, pass string, upProxy Proxy) (Proxy, error) {
	p := &mixedproxy{
		Proxy: upProxy,
	}

	p.http, _ = HTTPProxy(addr, upProxy)
	p.socks5, _ = SOCKS5Proxy(network, addr, user, pass, upProxy)

	if user != "" && pass != "" {
		p.ss, _ = SSProxy(user, pass, upProxy)
	}

	return p, nil
}

// mixedproxy .
func (p *mixedproxy) ListenAndServe() {
	l, err := net.Listen("tcp", p.Addr())
	if err != nil {
		logf("failed to listen on %s: %v", p.Addr(), err)
		return
	}

	logf("listening TCP on %s", p.Addr())

	for {
		c, err := l.Accept()
		if err != nil {
			logf("failed to accept: %v", err)
			continue
		}

		go func() {
			defer c.Close()

			if c, ok := c.(*net.TCPConn); ok {
				c.SetKeepAlive(true)
			}

			c := newConn(c)

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

			if p.ss != nil {
				p.ss.Serve(c)
			}

		}()
	}
}
