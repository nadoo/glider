package tcptun

import (
	"net"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterServer("tcptun", CreateServer)
}

// Server struct
type Server struct {
	*TCPTun
	*proxy.Forwarder
}

// NewServer returns a local proxy server
func NewServer(s string, f *proxy.Forwarder) (*Server, error) {
	h, err := NewTCPTun(s)
	if err != nil {
		return nil, err
	}
	server := &Server{TCPTun: h, Forwarder: f}
	return server, nil
}

// CreateServer returns a local proxy server
func CreateServer(s string, f *proxy.Forwarder) (proxy.Server, error) {
	return NewServer(s, f)
}

// ListenAndServe serves requests from clients
func (s *Server) ListenAndServe() {
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

			rc, err := s.Dial("tcp", s.raddr)
			if err != nil {

				log.F("failed to connect to target: %v", err)
				return
			}
			defer rc.Close()

			log.F("[tcptun] %s <-> %s", c.RemoteAddr(), s.raddr)

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
