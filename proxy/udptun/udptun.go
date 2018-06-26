package udptun

import (
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nadoo/glider/common/conn"
	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/proxy"
)

// UDPTun struct
type UDPTun struct {
	dialer proxy.Dialer
	addr   string

	raddr string
}

func init() {
	proxy.RegisterServer("udptun", NewUDPTunServer)
}

// NewUDPTun returns a UDPTun proxy.
func NewUDPTun(s string, dialer proxy.Dialer) (*UDPTun, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	d := strings.Split(addr, "=")

	p := &UDPTun{
		dialer: dialer,
		addr:   d[0],
		raddr:  d[1],
	}

	return p, nil
}

// NewUDPTunServer returns a udp tunnel server.
func NewUDPTunServer(s string, dialer proxy.Dialer) (proxy.Server, error) {
	return NewUDPTun(s, dialer)
}

// ListenAndServe .
func (s *UDPTun) ListenAndServe() {
	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.F("proxy-udptun failed to listen on %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	log.F("proxy-udptun listening UDP on %s", s.addr)

	var nm sync.Map
	buf := make([]byte, conn.UDPBufSize)

	for {
		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("proxy-udptun read error: %v", err)
			continue
		}

		var pc net.PacketConn
		var writeAddr net.Addr

		v, ok := nm.Load(raddr.String())
		if !ok && v == nil {

			pc, writeAddr, err = s.dialer.DialUDP("udp", s.raddr)
			if err != nil {
				log.F("proxy-udptun remote dial error: %v", err)
				continue
			}

			nm.Store(raddr.String(), pc)

			go func() {
				conn.TimedCopy(c, raddr, pc, 2*time.Minute)
				pc.Close()
				nm.Delete(raddr.String())
			}()

		} else {
			pc = v.(net.PacketConn)
		}

		_, err = pc.WriteTo(buf[:n], writeAddr)
		if err != nil {
			log.F("proxy-udptun remote write error: %v", err)
			continue
		}

		log.F("proxy-udptun %s <-> %s", raddr, s.raddr)

	}
}
