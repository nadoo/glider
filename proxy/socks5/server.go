package socks5

import (
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/proxy/socks"
)

// NewSocks5Server returns a socks5 proxy server.
func NewSocks5Server(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewSocks5(s, nil, p)
}

// ListenAndServe serves socks5 requests.
func (s *Socks5) ListenAndServe() {
	go s.ListenAndServeUDP()
	s.ListenAndServeTCP()
}

// ListenAndServeTCP listen and serve on tcp port.
func (s *Socks5) ListenAndServeTCP() {
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

		go s.Serve(c)
	}
}

// Serve serves a connection.
func (s *Socks5) Serve(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	tgt, err := s.handshake(c)
	if err != nil {
		// UDP: keep the connection until disconnect then free the UDP socket
		if err == socks.Errors[9] {
			buf := pool.GetBuffer(1)
			defer pool.PutBuffer(buf)
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

		log.F("[socks5] failed in handshake with %s: %v", c.RemoteAddr(), err)
		return
	}

	rc, dialer, err := s.proxy.Dial("tcp", tgt.String())
	if err != nil {
		log.F("[socks5] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	log.F("[socks5] %s <-> %s via %s", c.RemoteAddr(), tgt, dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[socks5] %s <-> %s via %s, relay error: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		// record remote conn failure only
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

// ListenAndServeUDP serves udp requests.
func (s *Socks5) ListenAndServeUDP() {
	lc, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.F("[socks5] failed to listen on UDP %s: %v", s.addr, err)
		return
	}
	defer lc.Close()

	log.F("[socks5] listening UDP on %s", s.addr)

	var nm sync.Map
	buf := make([]byte, proxy.UDPBufSize)

	for {
		c := NewPktConn(lc, nil, nil, true, nil)

		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("[socks5u] remote read error: %v", err)
			continue
		}

		var pc *PktConn
		v, ok := nm.Load(raddr.String())
		if !ok && v == nil {
			if c.tgtAddr == nil {
				log.F("[socks5u] can not get target address, not a valid request")
				continue
			}

			lpc, nextHop, err := s.proxy.DialUDP("udp", c.tgtAddr.String())
			if err != nil {
				log.F("[socks5u] remote dial error: %v", err)
				continue
			}

			pc = NewPktConn(lpc, nextHop, nil, false, nil)
			nm.Store(raddr.String(), pc)

			go func() {
				proxy.RelayUDP(c, raddr, pc, 2*time.Minute)
				pc.Close()
				nm.Delete(raddr.String())
			}()

			log.F("[socks5u] %s <-> %s", raddr, c.tgtAddr)

		} else {
			pc = v.(*PktConn)
		}

		_, err = pc.WriteTo(buf[:n], pc.writeAddr)
		if err != nil {
			log.F("[socks5u] remote write error: %v", err)
			continue
		}

		// log.F("[socks5u] %s <-> %s", raddr, c.tgtAddr)
	}

}

// Handshake fast-tracks SOCKS initialization to get target address to connect.
func (s *Socks5) handshake(rw io.ReadWriter) (socks.Addr, error) {
	// Read RFC 1928 for request and reply structure and sizes
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
	if s.user != "" && s.password != "" {
		_, err := rw.Write([]byte{Version, socks.AuthPassword})
		if err != nil {
			return nil, err
		}

		_, err = io.ReadFull(rw, buf[:2])
		if err != nil {
			return nil, err
		}

		// Get username
		userLen := int(buf[1])
		if userLen <= 0 {
			rw.Write([]byte{1, 1})
			return nil, errors.New("auth failed: wrong username length")
		}

		if _, err := io.ReadFull(rw, buf[:userLen]); err != nil {
			return nil, errors.New("auth failed: cannot get username")
		}
		user := string(buf[:userLen])

		// Get password
		_, err = rw.Read(buf[:1])
		if err != nil {
			return nil, errors.New("auth failed: cannot get password len")
		}

		passLen := int(buf[0])
		if passLen <= 0 {
			rw.Write([]byte{1, 1})
			return nil, errors.New("auth failed: wrong password length")
		}

		_, err = io.ReadFull(rw, buf[:passLen])
		if err != nil {
			return nil, errors.New("auth failed: cannot get password")
		}
		pass := string(buf[:passLen])

		// Verify
		if user != s.user || pass != s.password {
			_, err = rw.Write([]byte{1, 1})
			if err != nil {
				return nil, err
			}
			return nil, errors.New("auth failed, authinfo: " + user + ":" + pass)
		}

		// Response auth state
		_, err = rw.Write([]byte{1, 0})
		if err != nil {
			return nil, err
		}

	} else if _, err := rw.Write([]byte{Version, socks.AuthNone}); err != nil {
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
