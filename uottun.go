package main

import (
	"io/ioutil"
	"net"
	"time"
)

// UoTTun udp over tcp tunnel
type UoTTun struct {
	dialer Dialer
	addr   string

	raddr string
}

// NewUoTTun returns a UoTTun proxy.
func NewUoTTun(addr, raddr string, dialer Dialer) (*UoTTun, error) {
	s := &UoTTun{
		dialer: dialer,
		addr:   addr,
		raddr:  raddr,
	}

	return s, nil
}

// ListenAndServe .
func (s *UoTTun) ListenAndServe() {
	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		logf("proxy-uottun failed to listen on %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	logf("proxy-uottun listening UDP on %s", s.addr)

	buf := make([]byte, udpBufSize)

	for {
		n, clientAddr, err := c.ReadFrom(buf)
		if err != nil {
			logf("proxy-uottun read error: %v", err)
			continue
		}

		rc, err := s.dialer.Dial("uot", s.raddr)
		if err != nil {
			logf("proxy-uottun failed to connect to server %v: %v", s.raddr, err)
			continue
		}

		go func() {
			// no remote forwarder, just a local udp forwarder
			if urc, ok := rc.(*net.UDPConn); ok {
				timedCopy(c, clientAddr, urc, 2*time.Minute)
				urc.Close()
				return
			}

			// remote forwarder, udp over tcp
			resp, err := ioutil.ReadAll(rc)
			if err != nil {
				logf("error in ioutil.ReadAll: %s\n", err)
				return
			}
			rc.Close()
			c.WriteTo(resp, clientAddr)
		}()

		_, err = rc.Write(buf[:n])
		if err != nil {
			logf("proxy-uottun remote write error: %v", err)
			continue
		}

		logf("proxy-uottun %s <-> %s", clientAddr, s.raddr)
	}
}
