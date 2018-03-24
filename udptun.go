package main

import (
	"net"
	"sync"
	"time"
)

// UDPTun struct
type UDPTun struct {
	dialer Dialer
	addr   string

	raddr string
}

// NewUDPTun returns a UDPTun proxy.
func NewUDPTun(addr, raddr string, dialer Dialer) (*UDPTun, error) {
	s := &UDPTun{
		dialer: dialer,
		addr:   addr,
		raddr:  raddr,
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

	var nm sync.Map
	buf := make([]byte, udpBufSize)

	for {
		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			logf("proxy-udptun read error: %v", err)
			continue
		}

		var pc net.PacketConn
		var writeAddr net.Addr

		v, ok := nm.Load(raddr.String())
		if !ok && v == nil {

			pc, writeAddr, err = s.dialer.DialUDP("udp", s.raddr)
			if err != nil {
				logf("proxy-udptun remote dial error: %v", err)
				continue
			}

			nm.Store(raddr.String(), pc)

			go func() {
				timedCopy(c, raddr, pc, 2*time.Minute)
				pc.Close()
				nm.Delete(raddr.String())
			}()

		} else {
			pc = v.(net.PacketConn)
		}

		_, err = pc.WriteTo(buf[:n], writeAddr)
		if err != nil {
			logf("proxy-udptun remote write error: %v", err)
			continue
		}

		logf("proxy-udptun %s <-> %s", raddr, s.raddr)

	}
}
