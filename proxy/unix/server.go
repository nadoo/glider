package unix

import (
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterServer("unix", NewUnixServer)
}

// NewUnixServer returns a unix domain socket server.
func NewUnixServer(s string, p proxy.Proxy) (proxy.Server, error) {
	transport := strings.Split(s, ",")

	unix, err := NewUnix(transport[0], nil, p)
	if err != nil {
		return nil, err
	}

	if len(transport) > 1 {
		unix.server, err = proxy.ServerFromURL(transport[1], p)
		if err != nil {
			return nil, err
		}
	}

	return unix, nil
}

// ListenAndServe serves requests.
func (s *Unix) ListenAndServe() {
	go s.ListenAndServeUDP()
	s.ListenAndServeTCP()
}

// ListenAndServe serves tcp requests.
func (s *Unix) ListenAndServeTCP() {
	os.Remove(s.addr)
	l, err := net.Listen("unix", s.addr)
	if err != nil {
		log.F("[unix] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[unix] listening on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[unix] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve serves requests.
func (s *Unix) Serve(c net.Conn) {
	if s.server != nil {
		s.server.Serve(c)
		return
	}

	defer c.Close()

	rc, dialer, err := s.proxy.Dial("unix", "")
	if err != nil {
		log.F("[unix] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), s.addr, dialer.Addr(), err)
		s.proxy.Record(dialer, false)
		return
	}
	defer rc.Close()

	log.F("[unix] %s <-> %s", c.RemoteAddr(), dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[unix] %s <-> %s, relay error: %v", c.RemoteAddr(), dialer.Addr(), err)
		// record remote conn failure only
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

// ListenAndServe serves udp requests.
func (s *Unix) ListenAndServeUDP() {
	os.Remove(s.addru)
	c, err := net.ListenPacket("unixgram", s.addru)
	if err != nil {
		log.F("[unix] failed to ListenPacket on %s: %v", s.addru, err)
		return
	}
	defer c.Close()

	log.F("[unix] ListenPacket on %s", s.addru)

	var nm sync.Map
	buf := make([]byte, proxy.UDPBufSize)

	for {
		n, lraddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("[unix] read error: %v", err)
			continue
		}

		var raddr net.Addr
		var pc net.PacketConn
		var dialer proxy.UDPDialer

		v, ok := nm.Load(lraddr.String())
		if !ok && v == nil {
			pc, dialer, raddr, err = s.proxy.DialUDP("udp", "")
			if err != nil {
				log.F("[unix] remote dial error: %v", err)
				continue
			}

			nm.Store(lraddr.String(), pc)

			go func(c, pc net.PacketConn, lraddr net.Addr) {
				proxy.RelayUDP(c, lraddr, pc, 2*time.Minute)
				pc.Close()
				nm.Delete(lraddr.String())
			}(c, pc, lraddr)

		} else {
			pc = v.(net.PacketConn)
		}

		_, err = pc.WriteTo(buf[:n], raddr)
		if err != nil {
			log.F("[unix] remote write error: %v", err)
			continue
		}

		log.F("[unix] %s <-> %s", s.addru, dialer.Addr())

	}
}
