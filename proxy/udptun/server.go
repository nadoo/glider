package udptun

import (
	"net"
	"sync"
	"time"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterServer("udptun", CreateServer)
}

// Server struct
type Server struct {
	*UDPTun
	*proxy.Forwarder
}

// NewServer returns a local proxy server
func NewServer(s string, f *proxy.Forwarder) (*Server, error) {
	h, err := NewUDPTun(s)
	if err != nil {
		return nil, err
	}
	server := &Server{UDPTun: h, Forwarder: f}
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
		log.F("[udptun] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	log.F("[udptun] listening UDP on %s", s.addr)

	var nm sync.Map
	buf := make([]byte, conn.UDPBufSize)

	for {
		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("[udptun] read error: %v", err)
			continue
		}

		var pc net.PacketConn
		var writeAddr net.Addr

		v, ok := nm.Load(raddr.String())
		if !ok && v == nil {

			pc, writeAddr, err = s.DialUDP("udp", s.raddr)
			if err != nil {
				log.F("[udptun] remote dial error: %v", err)
				continue
			}

			nm.Store(raddr.String(), pc)

			go func() {
				conn.TimedCopy(c, raddr, pc, 2*time.Minute)
				pc.Close()
				nm.Delete(raddr.String())
			}()

		} else {
			pc = v.(net.PacketConn)
		}

		_, err = pc.WriteTo(buf[:n], writeAddr)
		if err != nil {
			log.F("[udptun] remote write error: %v", err)
			continue
		}

		log.F("[udptun] %s <-> %s", raddr, s.raddr)

	}
}
