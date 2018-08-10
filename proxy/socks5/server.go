package socks5

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/common/socks"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterServer("socks5", CreateServer)
}

// Server struct
type Server struct {
	*SOCKS5
	*proxy.Forwarder
}

// NewServer returns a local proxy server
func NewServer(s string, f *proxy.Forwarder) (*Server, error) {
	h, err := NewSOCKS5(s)
	if err != nil {
		return nil, err
	}
	server := &Server{SOCKS5: h, Forwarder: f}
	return server, nil
}

// CreateServer returns a local proxy server
func CreateServer(s string, f *proxy.Forwarder) (proxy.Server, error) {
	return NewServer(s, f)
}

// ListenAndServe serves socks5 requests.
func (s *Server) ListenAndServe() {
	go s.ListenAndServeUDP()
	s.ListenAndServeTCP()
}

// ListenAndServeTCP .
func (s *Server) ListenAndServeTCP() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.F("[socks5] failed to listen on %s: %v", s.addr, err)
		return
	}

	log.F("[socks5] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[socks5] failed to accept: %v", err)
			continue
		}

		go s.ServeTCP(c)
	}
}

// ServeTCP .
func (s *Server) ServeTCP(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	tgt, err := s.handshake(c)
	if err != nil {
		// UDP: keep the connection until disconnect then free the UDP socket
		if err == socks.Errors[9] {
			buf := []byte{}
			// block here
			for {
				_, err := c.Read(buf)
				if err, ok := err.(net.Error); ok && err.Timeout() {
					continue
				}
				// log.F("[socks5] servetcp udp associate end")
				return
			}
		}

		log.F("[socks5] failed to get target address: %v", err)
		return
	}

	rc, err := s.Dial("tcp", tgt.String())
	if err != nil {
		log.F("[socks5] failed to connect to target: %v", err)
		return
	}
	defer rc.Close()

	log.F("[socks5] %s <-> %s", c.RemoteAddr(), tgt)

	_, _, err = conn.Relay(c, rc)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return // ignore i/o timeout
		}
		log.F("[socks5] relay error: %v", err)
	}
}

// ListenAndServeUDP serves udp requests.
func (s *Server) ListenAndServeUDP() {
	lc, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.F("[socks5-udp] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer lc.Close()

	log.F("[socks5-udp] listening UDP on %s", s.addr)

	var nm sync.Map
	buf := make([]byte, conn.UDPBufSize)

	for {
		c := NewPktConn(lc, nil, nil, true, nil)

		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("[socks5-udp] remote read error: %v", err)
			continue
		}

		var pc *PktConn
		v, ok := nm.Load(raddr.String())
		if !ok && v == nil {
			if c.tgtAddr == nil {
				log.F("[socks5-udp] can not get target address, not a valid request")
				continue
			}

			lpc, nextHop, err := s.DialUDP("udp", c.tgtAddr.String())
			if err != nil {
				log.F("[socks5-udp] remote dial error: %v", err)
				continue
			}

			pc = NewPktConn(lpc, nextHop, nil, false, nil)
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
			log.F("[socks5-udp] remote write error: %v", err)
			continue
		}

		log.F("[socks5-udp] %s <-> %s", raddr, c.tgtAddr)
	}

}

// Handshake fast-tracks SOCKS initialization to get target address to connect.
func (s *Server) handshake(rw io.ReadWriter) (socks.Addr, error) {
	// Read RFC 1928 for request and reply structure and sizes.
	buf := make([]byte, socks.MaxAddrLen)
	// read VER, NMETHODS, METHODS
	if _, err := io.ReadFull(rw, buf[:2]); err != nil {
		return nil, err
	}
	nmethods := buf[1]
	if _, err := io.ReadFull(rw, buf[:nmethods]); err != nil {
		return nil, err
	}
	// write VER METHOD
	if _, err := rw.Write([]byte{5, 0}); err != nil {
		return nil, err
	}
	// read VER CMD RSV ATYP DST.ADDR DST.PORT
	if _, err := io.ReadFull(rw, buf[:3]); err != nil {
		return nil, err
	}
	cmd := buf[1]
	addr, err := socks.ReadAddrBuf(rw, buf)
	if err != nil {
		return nil, err
	}
	switch cmd {
	case socks.CmdConnect:
		_, err = rw.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}) // SOCKS v5, reply succeeded
	case socks.CmdUDPAssociate:
		listenAddr := socks.ParseAddr(rw.(net.Conn).LocalAddr().String())
		_, err = rw.Write(append([]byte{5, 0, 0}, listenAddr...)) // SOCKS v5, reply succeeded
		if err != nil {
			return nil, socks.Errors[7]
		}
		err = socks.Errors[9]
	default:
		return nil, socks.Errors[7]
	}

	return addr, err // skip VER, CMD, RSV fields
}
