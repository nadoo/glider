package main

import (
	"errors"
	"log"
	"net"
	"strings"

	"github.com/shadowsocks/go-shadowsocks2/core"
)

// ss
type SSProxy struct {
	*proxy
	core.StreamConnCipher
}

// NewSSProxy returns a shadowsocks proxy.
func NewSSProxy(addr, method, pass string, upProxy Proxy) (*SSProxy, error) {
	ciph, err := core.PickCipher(method, nil, pass)
	if err != nil {
		log.Fatalf("PickCipher for '%s', error: %s", method, err)
	}

	s := &SSProxy{
		proxy:            NewProxy(addr, upProxy),
		StreamConnCipher: ciph,
	}

	return s, nil
}

// ListenAndServe shadowsocks requests as a server.
func (s *SSProxy) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		logf("failed to listen on %s: %v", s.addr, err)
		return
	}

	logf("listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			logf("failed to accept: %v", err)
			continue
		}
		go s.Serve(c)
	}
}

func (s *SSProxy) Serve(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	c = s.StreamConnCipher.StreamConn(c)

	tgt, err := ReadAddr(c)
	if err != nil {
		logf("failed to get target address: %v", err)
		return
	}

	rc, err := s.GetProxy(tgt.String()).Dial("tcp", tgt.String())
	if err != nil {
		logf("failed to connect to target: %v", err)
		return
	}
	defer rc.Close()

	logf("proxy-ss %s <-> %s", c.RemoteAddr(), tgt)

	_, _, err = relay(c, rc)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return // ignore i/o timeout
		}
		logf("relay error: %v", err)
	}

}

// Dial connects to the address addr on the network net via the proxy.
func (s *SSProxy) Dial(network, addr string) (net.Conn, error) {

	target := ParseAddr(addr)
	if target == nil {
		return nil, errors.New("Unable to parse address: " + addr)
	}

	c, err := s.GetProxy(s.addr).Dial("tcp", s.addr)
	if err != nil {
		logf("dial to %s error: %s", s.addr, err)
		return nil, err
	}

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	c = s.StreamConn(c)
	if _, err = c.Write(target); err != nil {
		c.Close()
		return nil, err
	}

	return c, err
}

// ListCipher .
func ListCipher() string {
	return strings.Join(core.ListCipher(), " ")
}
