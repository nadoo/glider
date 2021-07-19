package tproxy

import (
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/proxy"
)

// TProxy struct.
type TProxy struct {
	proxy proxy.Proxy
	addr  string
}

func init() {
	proxy.RegisterServer("tproxy", NewTProxyServer)
}

// NewTProxy returns a tproxy.
func NewTProxy(s string, p proxy.Proxy) (*TProxy, error) {
	u, err := url.Parse(s)
	if err != nil {
		log.F("[tproxy] parse err: %s", err)
		return nil, err
	}

	tp := &TProxy{
		proxy: p,
		addr:  u.Host,
	}

	return tp, nil
}

// NewTProxyServer returns a udp tunnel server.
func NewTProxyServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewTProxy(s, p)
}

// ListenAndServe listens on server's addr and serves connections.
func (s *TProxy) ListenAndServe() {
	// go s.ListenAndServeTCP()
	s.ListenAndServeUDP()
}

// ListenAndServeTCP .
func (s *TProxy) ListenAndServeTCP() {
	log.F("[tproxy] tcp mode not supported now, please use 'redir' instead")
}

// ListenAndServeUDP .
func (s *TProxy) ListenAndServeUDP() {
	laddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		log.F("[tproxyu] failed to resolve addr %s: %v", s.addr, err)
		return
	}

	lc, err := ListenUDP("udp", laddr)
	if err != nil {
		log.F("[tproxyu] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer lc.Close()

	log.F("[tproxyu] listening UDP on %s", s.addr)

	var nm sync.Map
	buf := make([]byte, proxy.UDPBufSize)

	for {
		n, lraddr, dstAddr, err := ReadFromUDP(lc, buf)
		if err != nil {
			log.F("[tproxyu] read error: %v", err)
			continue
		}

		var session *natEntry
		sessionKey := lraddr.String() + dstAddr.String()

		v, ok := nm.Load(sessionKey)
		if !ok && v == nil {
			pc, dialer, writeTo, err := s.proxy.DialUDP("udp", dstAddr.String())
			if err != nil {
				log.F("[tproxyu] dial to %s error: %v", dstAddr, err)
				continue
			}

			lpc, err := ListenPacket(dstAddr)
			if err != nil {
				log.F("[tproxyu] ListenPacket as %s error: %v", dstAddr, err)
				pc.Close()
				continue
			}

			session = newNatEntry(pc, writeTo)
			nm.Store(sessionKey, session)

			go func(lc net.PacketConn, pc net.PacketConn, lraddr *net.UDPAddr, key string) {
				proxy.RelayUDP(lc, lraddr, pc, 2*time.Minute)
				pc.Close()
				nm.Delete(key)
			}(lpc, pc, lraddr, sessionKey)

			log.F("[tproxyu] %s <-> %s via %s", lraddr, dstAddr, dialer.Addr())

		} else {
			session = v.(*natEntry)
		}

		_, err = session.WriteTo(buf[:n], session.writeTo)
		if err != nil {
			log.F("[tproxyu] writeTo %s error: %v", session.writeTo, err)
			continue
		}
	}
}

// Serve .
func (s *TProxy) Serve(c net.Conn) {
	log.F("[tproxy] func Serve: can not be called directly")
}

type natEntry struct {
	net.PacketConn
	writeTo net.Addr
}

func newNatEntry(pc net.PacketConn, writeTo net.Addr) *natEntry {
	return &natEntry{PacketConn: pc, writeTo: writeTo}
}
