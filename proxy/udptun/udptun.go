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

// UDPTun is a base udptun struct.
type UDPTun struct {
	dialer proxy.Dialer
	addr   string
	taddr  string       // tunnel addr string
	tuaddr *net.UDPAddr // tunnel addr
}

func init() {
	proxy.RegisterServer("udptun", NewUDPTunServer)
}

// NewUDPTun returns a UDPTun proxy.
func NewUDPTun(s string, dialer proxy.Dialer) (*UDPTun, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[udptun] parse err: %s", err)
		return nil, err
	}

	addr := u.Host
	d := strings.Split(addr, "=")

	p := &UDPTun{
		dialer: dialer,
		addr:   d[0],
		taddr:  d[1],
	}

	p.tuaddr, err = net.ResolveUDPAddr("udp", p.taddr)
	return p, err
}

// NewUDPTunServer returns a udp tunnel server.
func NewUDPTunServer(s string, dialer proxy.Dialer) (proxy.Server, error) {
	return NewUDPTun(s, dialer)
}

// ListenAndServe listen and serves on the given address.
func (s *UDPTun) ListenAndServe() {
	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.F("[udptun] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	log.F("[udptun] listening UDP on %s", s.addr)

	var nm sync.Map
	buf := make([]byte, conn.UDPBufSize)

	for {
		n, raddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("[udptun] read error: %v", err)
			continue
		}

		var pc net.PacketConn

		v, ok := nm.Load(raddr.String())
		if !ok && v == nil {
			pc, _, err = s.dialer.DialUDP("udp", s.taddr)
			if err != nil {
				log.F("[udptun] remote dial error: %v", err)
				continue
			}

			nm.Store(raddr.String(), pc)

			go func() {
				conn.RelayUDP(c, raddr, pc, 2*time.Minute)
				pc.Close()
				nm.Delete(raddr.String())
			}()

		} else {
			pc = v.(net.PacketConn)
		}

		_, err = pc.WriteTo(buf[:n], s.tuaddr)
		if err != nil {
			log.F("[udptun] remote write error: %v", err)
			continue
		}

		log.F("[udptun] %s <-> %s", raddr, s.taddr)

	}
}

// Serve serves a net.Conn, can not be called directly.
func (s *UDPTun) Serve(c net.Conn) {
	log.F("[udptun] func Serve: can not be called directly")
}
