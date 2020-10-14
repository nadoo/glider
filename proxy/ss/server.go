package ss

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/socks"
)

// NewSSServer returns a ss proxy server.
func NewSSServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewSS(s, nil, p)
}

// ListenAndServe serves ss requests.
func (s *SS) ListenAndServe() {
	go s.ListenAndServeUDP()
	s.ListenAndServeTCP()
}

// ListenAndServeTCP serves tcp ss requests.
func (s *SS) ListenAndServeTCP() {
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
		go s.Serve(c)
	}

}

// Serve serves a connection.
func (s *SS) Serve(c net.Conn) {
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

	network := "tcp"
	dialer := s.proxy.NextDialer(tgt.String())

	rc, err := dialer.Dial(network, tgt.String())
	if err != nil {
		log.F("[ss] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	log.F("[ss] %s <-> %s via %s", c.RemoteAddr(), tgt, dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[ss] %s <-> %s via %s, relay error: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		// record remote conn failure only
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

// ListenAndServeUDP serves udp ss requests.
func (s *SS) ListenAndServeUDP() {
	lc, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.F("[ss] failed to listen on UDP %s: %v", s.addr, err)
		return
	}
	defer lc.Close()

	lc = s.PacketConn(lc)

	log.F("[ss] listening UDP on %s", s.addr)

	var nm sync.Map
	buf := make([]byte, proxy.UDPBufSize)

	for {
		c := NewPktConn(lc, nil, nil, true)

		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("[ssu] remote read error: %v", err)
			continue
		}

		var pc *PktConn
		v, ok := nm.Load(raddr.String())
		if !ok && v == nil {
			lpc, nextHop, err := s.proxy.DialUDP("udp", c.tgtAddr.String())
			if err != nil {
				log.F("[ssu] remote dial error: %v", err)
				continue
			}

			pc = NewPktConn(lpc, nextHop, nil, false)
			nm.Store(raddr.String(), pc)

			go func() {
				proxy.RelayUDP(c, raddr, pc, 2*time.Minute)
				pc.Close()
				nm.Delete(raddr.String())
			}()

			log.F("[ssu] %s <-> %s via %s", raddr, c.tgtAddr, nextHop)

		} else {
			pc = v.(*PktConn)
		}

		_, err = pc.WriteTo(buf[:n], pc.writeAddr)
		if err != nil {
			log.F("[ssu] remote write error: %v", err)
			continue
		}

		// log.F("[ssu] %s <-> %s", raddr, c.tgtAddr)
	}
}
