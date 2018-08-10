package uottun

import (
	"io/ioutil"
	"net"
	"time"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterServer("uottun", CreateServer)
}

// Server struct
type Server struct {
	*UoTTun
	*proxy.Forwarder
}

// NewServer returns a local proxy server
func NewServer(s string, f *proxy.Forwarder) (*Server, error) {
	h, err := NewUoTTun(s)
	if err != nil {
		return nil, err
	}
	server := &Server{UoTTun: h, Forwarder: f}
	return server, nil
}

// CreateServer returns a local proxy server
func CreateServer(s string, f *proxy.Forwarder) (proxy.Server, error) {
	return NewServer(s, f)
}

// ListenAndServe serves requests from clients
func (s *Server) ListenAndServe() {
	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.F("[uottun] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	log.F("[uottun] listening UDP on %s", s.addr)

	buf := make([]byte, conn.UDPBufSize)

	for {
		n, clientAddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("[uottun] read error: %v", err)
			continue
		}

		rc, err := s.Dial("uot", s.raddr)
		if err != nil {
			log.F("[uottun] failed to connect to server %v: %v", s.raddr, err)
			continue
		}

		go func() {
			// no remote forwarder, just a local udp forwarder
			if urc, ok := rc.(*net.UDPConn); ok {
				conn.TimedCopy(c, clientAddr, urc, 2*time.Minute)
				urc.Close()
				return
			}

			// remote forwarder, udp over tcp
			resp, err := ioutil.ReadAll(rc)
			if err != nil {
				log.F("error in ioutil.ReadAll: %s\n", err)
				return
			}
			rc.Close()
			c.WriteTo(resp, clientAddr)
		}()

		_, err = rc.Write(buf[:n])
		if err != nil {
			log.F("[uottun] remote write error: %v", err)
			continue
		}

		log.F("[uottun] %s <-> %s", clientAddr, s.raddr)
	}
}
