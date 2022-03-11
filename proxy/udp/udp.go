package udp

import (
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/pool"
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
	uaddr  *net.UDPAddr
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

	t.uaddr, err = net.ResolveUDPAddr("udp", t.addr)
	return t, err
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
		log.Fatalf("[udp] failed to listen on UDP %s: %v", s.addr, err)
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

		var sess *session
		sessKey := srcAddr.String()

		v, ok := nm.Load(sessKey)
		if !ok || v == nil {
			sess = newSession(sessKey, srcAddr, c)
			nm.Store(sessKey, sess)
			go s.serveSession(sess)
		} else {
			sess = v.(*session)
		}

		sess.msgCh <- buf[:n]
	}
}

func (s *UDP) serveSession(session *session) {
	// we know we are creating an udp tunnel, so the dial addr is meaningless,
	// we use srcAddr here to help the unix client to identify the source socket.
	dstPC, dialer, err := s.proxy.DialUDP("udp", session.src.String())
	if err != nil {
		log.F("[udp] remote dial error: %v", err)
		nm.Delete(session.key)
		return
	}
	defer dstPC.Close()

	go func() {
		proxy.CopyUDP(session, session.src, dstPC, 2*time.Minute, 5*time.Second)
		nm.Delete(session.key)
		close(session.finCh)
	}()

	log.F("[udp] %s <-> %s", session.src, dialer.Addr())

	for {
		select {
		case p := <-session.msgCh:
			_, err = dstPC.WriteTo(p, nil) // we know it's tunnel so dst addr could be nil
			if err != nil {
				log.F("[udp] writeTo error: %v", err)
			}
			pool.PutBuffer(p)
		case <-session.finCh:
			return
		}
	}
}

type session struct {
	key string
	src *net.UDPAddr
	net.PacketConn
	msgCh chan []byte
	finCh chan struct{}
}

func newSession(key string, src net.Addr, srcPC net.PacketConn) *session {
	srcAddr, _ := net.ResolveUDPAddr("udp", src.String())
	return &session{key, srcAddr, srcPC, make(chan []byte, 32), make(chan struct{})}
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
	return nil, proxy.ErrNotSupported
}

// DialUDP connects to the given address via the proxy.
func (s *UDP) DialUDP(network, addr string) (net.PacketConn, error) {
	// return s.dialer.DialUDP(network, s.addr)
	pc, err := s.dialer.DialUDP(network, s.addr)
	return &PktConn{pc, s.uaddr}, err
}

// PktConn .
type PktConn struct {
	net.PacketConn
	uaddr *net.UDPAddr
}

// WriteTo overrides the original function from net.PacketConn.
func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return pc.PacketConn.WriteTo(b, pc.uaddr)
}
