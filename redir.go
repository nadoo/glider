// +build !windows

package main

import (
	"errors"
	"fmt"
	"net"
	"syscall"
)

const SO_ORIGINAL_DST = 80

type redir struct {
	Proxy
	addr string
}

// RedirProxy returns a redirect proxy.
func RedirProxy(addr string, upProxy Proxy) (Proxy, error) {
	s := &redir{
		Proxy: upProxy,
		addr:  addr,
	}

	return s, nil
}

// ListenAndServe redirected requests as a server.
func (s *redir) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		logf("failed to listen on %s: %v", s.addr, err)
		return
	}

	logf("listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			logf("failed to accept: %v", err)
			continue
		}

		go func() {
			defer c.Close()

			if c, ok := c.(*net.TCPConn); ok {
				c.SetKeepAlive(true)
			}

			tgt, c, err := getOriginalDstAddr(c)
			if err != nil {
				logf("failed to get target address: %v", err)
				return
			}

			rc, err := s.GetProxy().Dial("tcp", tgt.String())
			if err != nil {
				logf("failed to connect to target: %v", err)
				return
			}
			defer rc.Close()

			logf("proxy-redir %s <-> %s", c.RemoteAddr(), tgt)

			_, _, err = relay(c, rc)
			if err != nil {
				if err, ok := err.(net.Error); ok && err.Timeout() {
					return // ignore i/o timeout
				}
				logf("relay error: %v", err)
			}

		}()
	}
}

func getOriginalDstAddr(conn net.Conn) (addr net.Addr, c *net.TCPConn, err error) {
	defer conn.Close()

	fc, err := conn.(*net.TCPConn).File()
	if err != nil {
		return
	}
	defer fc.Close()

	mreq, err := syscall.GetsockoptIPv6Mreq(int(fc.Fd()), syscall.IPPROTO_IP, SO_ORIGINAL_DST)
	if err != nil {
		return
	}

	// only ipv4 support
	ip := net.IPv4(mreq.Multiaddr[4], mreq.Multiaddr[5], mreq.Multiaddr[6], mreq.Multiaddr[7])
	port := uint16(mreq.Multiaddr[2])<<8 + uint16(mreq.Multiaddr[3])
	addr, err = net.ResolveTCPAddr("tcp4", fmt.Sprintf("%s:%d", ip.String(), port))
	if err != nil {
		return
	}

	cc, err := net.FileConn(fc)
	if err != nil {
		return
	}

	c, ok := cc.(*net.TCPConn)
	if !ok {
		err = errors.New("not a TCP connection")
	}
	return
}
