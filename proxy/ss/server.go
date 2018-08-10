package ss

import (
	"net"
	"sync"
	"time"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/common/socks"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterServer("ss", CreateServer)
}

// Server struct
type Server struct {
	*SS
	*proxy.Forwarder
}

// NewServer returns a local proxy server
func NewServer(s string, f *proxy.Forwarder) (*Server, error) {
	h, err := NewSS(s)
	if err != nil {
		return nil, err
	}
	server := &Server{SS: h, Forwarder: f}
	return server, nil
}

// CreateServer returns a local proxy server
func CreateServer(s string, f *proxy.Forwarder) (proxy.Server, error) {
	return NewServer(s, f)
}

// ListenAndServe serves requests from clients
func (s *Server) ListenAndServe() {
	go s.ListenAndServeUDP()
	s.ListenAndServeTCP()
}

// ListenAndServeTCP serves tcp requests
func (s *Server) ListenAndServeTCP() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.F("[ss] failed to listen on %s: %v", s.addr, err)
		return
	}

	log.F("[ss] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[ss] failed to accept: %v", err)
			continue
		}
		go s.ServeTCP(c)
	}
}

// ServeTCP serves tcp requests
func (s *Server) ServeTCP(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	c = s.StreamConn(c)

	tgt, err := socks.ReadAddr(c)
	if err != nil {
		log.F("[ss] failed to get target address: %v", err)
		return
	}

	dialer := s.NextDialer(tgt.String())

	// udp over tcp?
	uot := socks.UoT(tgt[0])
	if uot && dialer.Addr() == "DIRECT" {
		rc, err := net.ListenPacket("udp", "")
		if err != nil {
			log.F("[ss-uottun] UDP remote listen error: %v", err)
		}
		defer rc.Close()

		req := make([]byte, conn.UDPBufSize)
		n, err := c.Read(req)
		if err != nil {
			log.F("[ss-uottun] error in ioutil.ReadAll: %s\n", err)
			return
		}

		tgtAddr, _ := net.ResolveUDPAddr("udp", tgt.String())
		rc.WriteTo(req[:n], tgtAddr)

		buf := make([]byte, conn.UDPBufSize)
		n, _, err = rc.ReadFrom(buf)
		if err != nil {
			log.F("[ss-uottun] read error: %v", err)
		}

		c.Write(buf[:n])

		log.F("[ss] %s <-tcp-> %s - %s <-udp-> %s ", c.RemoteAddr(), c.LocalAddr(), rc.LocalAddr(), tgt)

		return
	}

	network := "tcp"
	if uot {
		network = "udp"
	}

	rc, err := dialer.Dial(network, tgt.String())
	if err != nil {
		log.F("[ss] failed to connect to target: %v", err)
		return
	}
	defer rc.Close()

	log.F("[ss] %s <-> %s", c.RemoteAddr(), tgt)

	_, _, err = conn.Relay(c, rc)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return // ignore i/o timeout
		}
		log.F("[ss] relay error: %v", err)
	}

}

// ListenAndServeUDP serves udp requests
func (s *Server) ListenAndServeUDP() {
	lc, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.F("[ss-udp] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer lc.Close()

	lc = s.PacketConn(lc)

	log.F("[ss-udp] listening UDP on %s", s.addr)

	var nm sync.Map
	buf := make([]byte, conn.UDPBufSize)

	for {
		c := NewPktConn(lc, nil, nil, true)

		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("[ss-udp] remote read error: %v", err)
			continue
		}

		var pc *PktConn
		v, ok := nm.Load(raddr.String())
		if !ok && v == nil {
			lpc, nextHop, err := s.DialUDP("udp", c.tgtAddr.String())
			if err != nil {
				log.F("[ss-udp] remote dial error: %v", err)
				continue
			}

			pc = NewPktConn(lpc, nextHop, nil, false)
			nm.Store(raddr.String(), pc)

			go func() {
				conn.TimedCopy(c, raddr, pc, 2*time.Minute)
				pc.Close()
				nm.Delete(raddr.String())
			}()

		} else {
			pc = v.(*PktConn)
		}

		_, err = pc.WriteTo(buf[:n], pc.writeAddr)
		if err != nil {
			log.F("[ss-udp] remote write error: %v", err)
			continue
		}

		log.F("[ss-udp] %s <-> %s", raddr, c.tgtAddr)
	}
}
