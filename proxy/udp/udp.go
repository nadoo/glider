package udp

import (
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/proxy"
)

// UDP struct.
type UDP struct {
	addr   string
	dialer proxy.Dialer
	proxy  proxy.Proxy
	server proxy.Server
}

func init() {
	proxy.RegisterDialer("udp", NewUDPDialer)
	proxy.RegisterServer("udp", NewUDPServer)
}

// NewUDP returns a udp struct.
func NewUDP(s string, d proxy.Dialer, p proxy.Proxy) (*UDP, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[udp] parse url err: %s", err)
		return nil, err
	}

	t := &UDP{
		dialer: d,
		proxy:  p,
		addr:   u.Host,
	}

	return t, nil
}

// NewUDPDialer returns a udp dialer.
func NewUDPDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewUDP(s, d, nil)
}

// NewUDPServer returns a udp transport layer before the real server.
func NewUDPServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewUDP(s, nil, p)
}

// ListenAndServe listens on server's addr and serves connections.
func (s *UDP) ListenAndServe() {
	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.F("[udp] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	log.F("[udp] listening UDP on %s", s.addr)

	var nm sync.Map
	buf := make([]byte, proxy.UDPBufSize)

	for {
		n, lraddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("[udp] read error: %v", err)
			continue
		}

		var raddr net.Addr
		var pc net.PacketConn

		v, ok := nm.Load(lraddr.String())
		if !ok && v == nil {
			pc, raddr, err = s.proxy.DialUDP("udp", "")
			if err != nil {
				log.F("[udp] remote dial error: %v", err)
				continue
			}

			nm.Store(lraddr.String(), pc)

			go func(c, pc net.PacketConn, lraddr net.Addr) {
				proxy.RelayUDP(c, lraddr, pc, 2*time.Minute)
				pc.Close()
				nm.Delete(lraddr.String())
			}(c, pc, lraddr)

		} else {
			pc = v.(net.PacketConn)
		}

		_, err = pc.WriteTo(buf[:n], raddr)
		if err != nil {
			log.F("[udp] remote write error: %v", err)
			continue
		}

		log.F("[udp] %s <-> %s", lraddr, raddr)

	}
}

// Serve serves a connection.
func (s *UDP) Serve(c net.Conn) {
	log.F("[udp] func Serve: can not be called directly")
}

// Addr returns forwarder's address.
func (s *UDP) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (s *UDP) Dial(network, addr string) (net.Conn, error) {
	return s.dialer.Dial("udp", s.addr)
}

// DialUDP connects to the given address via the proxy.
func (s *UDP) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return s.dialer.DialUDP(network, s.addr)
}
