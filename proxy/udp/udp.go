package udp

import (
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy"
)

var nm sync.Map

func init() {
	proxy.RegisterDialer("udp", NewUDPDialer)
	proxy.RegisterServer("udp", NewUDPServer)
}

// UDP struct.
type UDP struct {
	addr   string
	dialer proxy.Dialer
	proxy  proxy.Proxy
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
		log.F("[udp] failed to listen on UDP %s: %v", s.addr, err)
		return
	}
	defer c.Close()

	log.F("[udp] listening UDP on %s", s.addr)

	for {
		buf := pool.GetBuffer(proxy.UDPBufSize)
		n, srcAddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("[udp] read error: %v", err)
			continue
		}

		var session *Session
		sessionKey := srcAddr.String()

		v, ok := nm.Load(sessionKey)
		if !ok || v == nil {
			session = newSession(sessionKey, srcAddr, c)
			nm.Store(sessionKey, session)
			go s.serveSession(session)
		} else {
			session = v.(*Session)
		}

		session.msgCh <- buf[:n]
	}
}

func (s *UDP) serveSession(session *Session) {
	// we know we are creating an udp tunnel, so the dial addr is meaningless,
	// we use srcAddr here to help the unix client to identify the source socket.
	dstPC, dialer, writeTo, err := s.proxy.DialUDP("udp", session.src.String())
	if err != nil {
		log.F("[udp] remote dial error: %v", err)
		return
	}
	defer dstPC.Close()

	go func() {
		proxy.RelayUDP(session.srcPC, session.src, dstPC, 2*time.Minute)
		nm.Delete(session.key)
		close(session.finCh)
	}()

	log.F("[udp] %s <-> %s", session.src, dialer.Addr())

	for {
		select {
		case p := <-session.msgCh:
			_, err = dstPC.WriteTo(p, writeTo)
			if err != nil {
				log.F("[udp] writeTo %s error: %v", writeTo, err)
			}
			pool.PutBuffer(p)
		case <-session.finCh:
			return
		}
	}
}

// Session is a udp session
type Session struct {
	key   string
	src   net.Addr
	srcPC net.PacketConn
	msgCh chan []byte
	finCh chan struct{}
}

func newSession(key string, src net.Addr, srcPC net.PacketConn) *Session {
	return &Session{key, src, srcPC, make(chan []byte, 32), make(chan struct{})}
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
