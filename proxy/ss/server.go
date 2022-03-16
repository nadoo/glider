package ss

import (
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/pkg/socks"
	"github.com/nadoo/glider/proxy"
)

var nm sync.Map

// NewSSServer returns a ss proxy server.
func NewSSServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewSS(s, nil, p)
}

// ListenAndServe serves ss requests.
func (s *SS) ListenAndServe() {
	go s.ListenAndServeUDP()
	s.ListenAndServeTCP()
}

// ListenAndServeTCP serves tcp ss requests.
func (s *SS) ListenAndServeTCP() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.Fatalf("[ss] failed to listen on %s: %v", s.addr, err)
		return
	}

	log.F("[ss] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[ss] failed to accept: %v", err)
			continue
		}
		go s.Serve(c)
	}

}

// Serve serves a connection.
func (s *SS) Serve(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	sc := s.StreamConn(c)

	tgt, err := socks.ReadAddr(sc)
	if err != nil {
		log.F("[ss] failed to get target address: %v", err)
		proxy.Copy(io.Discard, c) // https://github.com/nadoo/glider/issues/180
		return
	}

	dialer := s.proxy.NextDialer(tgt.String())
	rc, err := dialer.Dial("tcp", tgt.String())
	if err != nil {
		log.F("[ss] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	log.F("[ss] %s <-> %s via %s", c.RemoteAddr(), tgt, dialer.Addr())

	if err = proxy.Relay(sc, rc); err != nil {
		log.F("[ss] %s <-> %s via %s, relay error: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		// record remote conn failure only
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

// ListenAndServeUDP serves udp requests.
func (s *SS) ListenAndServeUDP() {
	lc, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		log.Fatalf("[ss] failed to listen on UDP %s: %v", s.addr, err)
		return
	}
	defer lc.Close()

	log.F("[ss] listening UDP on %s", s.addr)

	s.ServePacket(lc)
}

// ServePacket implements proxy.PacketServer.
func (s *SS) ServePacket(pc net.PacketConn) {
	lc := s.PacketConn(pc)
	for {
		c := NewPktConn(lc, nil, nil)
		buf := pool.GetBuffer(proxy.UDPBufSize)

		n, srcAddr, dstAddr, err := c.readFrom(buf)
		if err != nil {
			log.F("[ssu] remote read error: %v", err)
			continue
		}

		var session *Session
		sessionKey := srcAddr.String()

		v, ok := nm.Load(sessionKey)
		if !ok || v == nil {
			session = newSession(sessionKey, srcAddr, dstAddr, c)
			nm.Store(sessionKey, session)
			go s.serveSession(session)
		} else {
			session = v.(*Session)
		}

		session.msgCh <- message{dstAddr, buf[:n]}
	}
}

func (s *SS) serveSession(session *Session) {
	dstPC, dialer, err := s.proxy.DialUDP("udp", session.dst.String())
	if err != nil {
		log.F("[ssu] remote dial error: %v", err)
		nm.Delete(session.key)
		return
	}
	defer dstPC.Close()

	go func() {
		proxy.CopyUDP(session.srcPC, nil, dstPC, 2*time.Minute, 5*time.Second)
		nm.Delete(session.key)
		close(session.finCh)
	}()

	log.F("[ssu] %s <-> %s via %s", session.src, session.dst, dialer.Addr())

	for {
		select {
		case msg := <-session.msgCh:
			_, err = dstPC.WriteTo(msg.msg, msg.dst)
			if err != nil {
				log.F("[ssu] writeTo %s error: %v", msg.dst, err)
			}
			pool.PutBuffer(msg.msg)
			msg.msg = nil
		case <-session.finCh:
			return
		}
	}
}

type message struct {
	dst net.Addr
	msg []byte
}

// Session is a udp session
type Session struct {
	key   string
	src   net.Addr
	dst   net.Addr
	srcPC *PktConn
	msgCh chan message
	finCh chan struct{}
}

func newSession(key string, src, dst net.Addr, srcPC *PktConn) *Session {
	return &Session{key, src, dst, srcPC, make(chan message, 32), make(chan struct{})}
}
