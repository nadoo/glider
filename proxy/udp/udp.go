package udp

import (
	"fmt"
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

		var session *natEntry
		v, ok := nm.Load(lraddr.String())
		if !ok && v == nil {
			// we know we are creating an udp tunnel, so the dial addr is meaningless,
			// we use lraddr here to help the unix client to identify the source socket.
			pc, dialer, writeTo, err := s.proxy.DialUDP("udp", lraddr.String())
			if err != nil {
				log.F("[udp] remote dial error: %v", err)
				continue
			}

			session = newNatEntry(pc, writeTo)
			nm.Store(lraddr.String(), session)

			go func(c, pc net.PacketConn, lraddr net.Addr) {
				proxy.RelayUDP(c, lraddr, pc, 2*time.Minute)
				pc.Close()
				nm.Delete(lraddr.String())
			}(c, pc, lraddr)

			log.F("[udp] %s <-> %s", lraddr, dialer.Addr())

		} else {
			session = v.(*natEntry)
		}

		_, err = session.WriteTo(buf[:n], session.writeTo)
		if err != nil {
			log.F("[udp] writeTo %s error: %v", session.writeTo, err)
			continue
		}

	}
}

type natEntry struct {
	net.PacketConn
	writeTo net.Addr
}

func newNatEntry(pc net.PacketConn, writeTo net.Addr) *natEntry {
	return &natEntry{PacketConn: pc, writeTo: writeTo}
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
	return nil, fmt.Errorf("can not dial tcp via udp dialer: %w", proxy.ErrNotSupported)
}

// DialUDP connects to the given address via the proxy.
func (s *UDP) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return s.dialer.DialUDP(network, s.addr)
}
