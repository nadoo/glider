package main

import (
	"net"
	"time"
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

		go func() {
			rc, wt, err := s.sDialer.DialUDP("udp", s.raddr)
			if err != nil {
				logf("proxy-udptun failed to connect to server %v: %v", s.raddr, err)
				return
			}

			n, err = rc.WriteTo(buf[:n], wt)
			if err != nil {
				logf("proxy-udptun rc.Write error: %v", err)
				return
			}

			rcBuf := make([]byte, udpBufSize)
			rc.SetReadDeadline(time.Now().Add(time.Minute))

			n, _, err = rc.ReadFrom(rcBuf)
			if err != nil {
				logf("proxy-udptun rc.Read error: %v", err)
				return
			}
			rc.Close()

			c.WriteTo(rcBuf[:n], clientAddr)
			logf("proxy-udptun %s <-> %s", clientAddr, s.raddr)
		}()

	}
}
