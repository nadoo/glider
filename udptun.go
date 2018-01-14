package main

import (
	"net"
)

// UDPTun struct
type UDPTun struct {
	*Forwarder
	sDialer Dialer

	raddr string
}

// NewUDPTun returns a UDPTun proxy.
func NewUDPTun(addr, raddr string, sDialer Dialer) (*UDPTun, error) {
	s := &UDPTun{
		Forwarder: NewForwarder(addr, nil),
		sDialer:   sDialer,
		raddr:     raddr,
	}

	return s, nil
}

// ListenAndServe .
func (s *UDPTun) ListenAndServe() {
	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		logf("proxy-udptun failed to listen on %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	logf("proxy-udptun listening UDP on %s", s.addr)

	buf := make([]byte, udpBufSize)
	for {
		n, clientAddr, err := c.ReadFrom(buf)
		if err != nil {
			logf("proxy-udptun read error: %v", err)
			continue
		}

		rc, err := s.sDialer.Dial("udp", s.raddr)
		if err != nil {
			logf("proxy-udptun failed to connect to server %v: %v", s.raddr, err)
			continue
		}

		n, err = rc.Write(buf[:n])
		if err != nil {
			logf("proxy-udptun rc.Write error: %v", err)
			continue
		}

		buf = make([]byte, udpBufSize)
		n, err = rc.Read(buf)
		if err != nil {
			logf("proxy-udptun rc.Read error: %v", err)
			continue
		}
		rc.Close()

		// logf("rc resp: \n%s", hex.Dump(buf[:n]))

		c.WriteTo(buf[:n], clientAddr)
		logf("proxy-udptun %s <-> %s", clientAddr, s.raddr)
	}
}
