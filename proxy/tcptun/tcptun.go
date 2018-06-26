package tcptun

import (
	"net"
	"net/url"
	"strings"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// TCPTun struct
type TCPTun struct {
	dialer proxy.Dialer
	addr   string

	raddr string
}

func init() {
	proxy.RegisterServer("tcptun", NewTCPTunServer)
}

// NewTCPTun returns a tcptun proxy.
func NewTCPTun(s string, dialer proxy.Dialer) (*TCPTun, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	d := strings.Split(addr, "=")

	p := &TCPTun{
		dialer: dialer,
		addr:   d[0],
		raddr:  d[1],
	}

	return p, nil
}

// NewTCPTunServer returns a udp tunnel server.
func NewTCPTunServer(s string, dialer proxy.Dialer) (proxy.Server, error) {
	return NewTCPTun(s, dialer)
}

// ListenAndServe .
func (s *TCPTun) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.F("failed to listen on %s: %v", s.addr, err)
		return
	}

	log.F("listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("failed to accept: %v", err)
			continue
		}

		go func() {
			defer c.Close()

			if c, ok := c.(*net.TCPConn); ok {
				c.SetKeepAlive(true)
			}

			rc, err := s.dialer.Dial("tcp", s.raddr)
			if err != nil {

				log.F("failed to connect to target: %v", err)
				return
			}
			defer rc.Close()

			log.F("proxy-tcptun %s <-> %s", c.RemoteAddr(), s.raddr)

			_, _, err = conn.Relay(c, rc)
			if err != nil {
				if err, ok := err.(net.Error); ok && err.Timeout() {
					return // ignore i/o timeout
				}
				log.F("relay error: %v", err)
			}

		}()
	}
}
