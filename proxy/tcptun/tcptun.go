package tcptun

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// TCPTun struct.
type TCPTun struct {
	proxy proxy.Proxy
	addr  string

	raddr string
}

func init() {
	proxy.RegisterServer("tcptun", NewTCPTunServer)
}

// NewTCPTun returns a tcptun proxy.
func NewTCPTun(s string, p proxy.Proxy) (*TCPTun, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[tcptun] parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	d := strings.Split(addr, "=")
	if len(d) < 2 {
		return nil, errors.New("error in strings.Split")
	}

	t := &TCPTun{
		proxy: p,
		addr:  d[0],
		raddr: d[1],
	}

	return t, nil
}

// NewTCPTunServer returns a udp tunnel server.
func NewTCPTunServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewTCPTun(s, p)
}

// ListenAndServe listens on server's addr and serves connections.
func (s *TCPTun) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.F("failed to listen on %s: %v", s.addr, err)
		return
	}

	log.F("[tcptun] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[tcptun] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve serves a connection.
func (s *TCPTun) Serve(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	rc, p, err := s.proxy.Dial("tcp", s.raddr)
	if err != nil {
		log.F("[tcptun] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), s.addr, p, err)
		return
	}
	defer rc.Close()

	log.F("[tcptun] %s <-> %s via %s", c.RemoteAddr(), s.raddr, p)

	_, _, err = conn.Relay(c, rc)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return // ignore i/o timeout
		}
		log.F("relay error: %v", err)
	}
}
