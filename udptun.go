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

	// var nm sync.Map
	buf := make([]byte, udpBufSize)
	tgt := ParseAddr(s.raddr)
	copy(buf, tgt)

	for {
		n, clientAddr, err := c.ReadFrom(buf[len(tgt):])
		if err != nil {
			logf("proxy-udptun read error: %v", err)
			continue
		}

		go func() {
			rc, err := s.sDialer.DialUDP("udp", s.raddr)
			if err != nil {
				logf("proxy-udptun failed to connect to server %v: %v", s.raddr, err)
				return
			}

			// TODO: check here, get the correct sDialer's addr
			sUDPAddr, err := net.ResolveUDPAddr("udp", s.sDialer.Addr())
			if err != nil {
				logf("proxy-udptun failed to ResolveUDPAddr %", s.sDialer.Addr())
				return
			}

			rc.WriteTo(buf[:len(tgt)+n], sUDPAddr)

			go func() {
				timedCopy(c, clientAddr, rc, 5*time.Minute, false)
				rc.Close()
			}()

			logf("proxy-udptun %s <-> %s", clientAddr, s.raddr)
		}()
	}
}
