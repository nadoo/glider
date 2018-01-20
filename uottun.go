package main

import (
	"io/ioutil"
	"net"
	"time"
)

// UoTTun udp over tcp tunnel
type UoTTun struct {
	*Forwarder
	sDialer Dialer

	raddr string
}

// NewUoTTun returns a UoTTun proxy.
func NewUoTTun(addr, raddr string, sDialer Dialer) (*UoTTun, error) {
	s := &UoTTun{
		Forwarder: NewForwarder(addr, nil),
		sDialer:   sDialer,
		raddr:     raddr,
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

		rc, err := s.sDialer.Dial("uot", s.raddr)
		if err != nil {
			logf("proxy-uottun failed to connect to server %v: %v", s.raddr, err)
			continue
		}

		rc.Write(buf[:n])

		// no remote forwarder, just a local udp forwarder
		if urc, ok := rc.(*net.UDPConn); ok {
			go func() {
				timedCopy(c, clientAddr, urc, 5*time.Minute)
				urc.Close()
			}()
		} else { // remote forwarder, udp over tcp
			resp, err := ioutil.ReadAll(rc)
			if err != nil {
				logf("error in ioutil.ReadAll: %s\n", err)
				continue
			}
			rc.Close()
			c.WriteTo(resp, clientAddr)
		}

		logf("proxy-uottun %s <-> %s", clientAddr, s.raddr)
	}
}
