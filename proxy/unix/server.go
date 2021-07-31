package unix

import (
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy"
)

var nm sync.Map

func init() {
	proxy.RegisterServer("unix", NewUnixServer)
}

// NewUnixServer returns a unix domain socket server.
func NewUnixServer(s string, p proxy.Proxy) (proxy.Server, error) {
	schemes := strings.SplitN(s, ",", 2)
	unix, err := NewUnix(schemes[0], nil, p)
	if err != nil {
		return nil, err
	}

	if len(schemes) > 1 {
		unix.server, err = proxy.ServerFromURL(schemes[1], p)
		if err != nil {
			return nil, err
		}
	}

	return unix, nil
}

// ListenAndServe serves requests.
func (s *Unix) ListenAndServe() {
	go s.ListenAndServeUDP()
	s.ListenAndServeTCP()
}

// ListenAndServeTCP serves tcp requests.
func (s *Unix) ListenAndServeTCP() {
	os.Remove(s.addr)
	l, err := net.Listen("unix", s.addr)
	if err != nil {
		log.F("[unix] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[unix] listening on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[unix] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve serves requests.
func (s *Unix) Serve(c net.Conn) {
	if s.server != nil {
		s.server.Serve(c)
		return
	}

	defer c.Close()

	rc, dialer, err := s.proxy.Dial("unix", "")
	if err != nil {
		log.F("[unix] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), s.addr, dialer.Addr(), err)
		s.proxy.Record(dialer, false)
		return
	}
	defer rc.Close()

	log.F("[unix] %s <-> %s", c.RemoteAddr(), dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[unix] %s <-> %s, relay error: %v", c.RemoteAddr(), dialer.Addr(), err)
		// record remote conn failure only
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

// ListenAndServeUDP serves udp requests.
func (s *Unix) ListenAndServeUDP() {
	os.Remove(s.addru)
	c, err := net.ListenPacket("unixgram", s.addru)
	if err != nil {
		log.F("[unix] failed to ListenPacket on %s: %v", s.addru, err)
		return
	}
	defer c.Close()

	log.F("[unix] ListenPacket on %s", s.addru)

	for {
		buf := pool.GetBuffer(proxy.UDPBufSize)
		n, srcAddr, err := c.ReadFrom(buf)
		if err != nil {
			log.F("[unix] read error: %v", err)
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
func (s *Unix) serveSession(session *Session) {
	dstPC, dialer, writeTo, err := s.proxy.DialUDP("udp", "")
	if err != nil {
		log.F("[unix] remote dial error: %v", err)
		return
	}
	defer dstPC.Close()

	go func() {
		proxy.RelayUDP(session.srcPC, session.src, dstPC, 2*time.Minute)
		nm.Delete(session.key)
		close(session.finCh)
	}()

	log.F("[unix] %s <-> %s", session.src, dialer.Addr())

	for {
		select {
		case p := <-session.msgCh:
			_, err = dstPC.WriteTo(p, writeTo)
			if err != nil {
				log.F("[unix] writeTo %s error: %v", writeTo, err)
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
