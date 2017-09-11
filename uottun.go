package main

import (
	"io/ioutil"
	"net"
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

		go func() {
			// NOTE: acturally udp over tcp
			rc, err := s.sDialer.Dial("udp", s.raddr)
			if err != nil {
				logf("failed to connect to server %v: %v", s.raddr, err)
				return
			}

			rc.Write(buf[:n])

			resp, err := ioutil.ReadAll(rc)
			if err != nil {
				logf("error in ioutil.ReadAll: %s\n", err)
				return
			}
			rc.Close()

			c.WriteTo(resp, clientAddr)

			logf("proxy-uottun %s <-> %s", clientAddr, s.raddr)
		}()
	}
}
