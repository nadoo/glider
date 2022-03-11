package tproxy

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
	proxy.RegisterServer("tproxy", NewTProxyServer)
}

// TProxy struct.
type TProxy struct {
	proxy proxy.Proxy
	addr  string
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

// ListenAndServeTCP listens and serves tcp.
func (s *TProxy) ListenAndServeTCP() {
	log.F("[tproxy] tcp mode not supported now, please use 'redir' instead")
}

// Serve serves tcp conn.
func (s *TProxy) Serve(c net.Conn) {
	log.F("[tproxy] func Serve: can not be called directly")
}

// ListenAndServeUDP listens and serves udp.
func (s *TProxy) ListenAndServeUDP() {
	laddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		log.Fatalf("[tproxyu] failed to resolve addr %s: %v", s.addr, err)
		return
	}

	lc, err := ListenUDP("udp", laddr)
	if err != nil {
		log.F("[tproxyu] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer lc.Close()

	log.F("[tproxyu] listening UDP on %s", s.addr)

	for {
		buf := pool.GetBuffer(proxy.UDPBufSize)
		n, srcAddr, dstAddr, err := ReadFromUDP(lc, buf)
		if err != nil {
			log.F("[tproxyu] read error: %v", err)
			continue
		}

		var sess *session
		sessKey := srcAddr.String()

		v, ok := nm.Load(sessKey)
		if !ok || v == nil {
			sess = newSession(sessKey, srcAddr, dstAddr)
			nm.Store(sessKey, sess)
			go s.serveSession(sess)
		} else {
			sess = v.(*session)
		}

		sess.msgCh <- message{dstAddr, buf[:n]}
	}
}

// serveSession serves a udp session.
func (s *TProxy) serveSession(session *session) {
	dstPC, dialer, err := s.proxy.DialUDP("udp", session.dst.String())
	if err != nil {
		log.F("[tproxyu] dial to %s error: %v", session.dst, err)
		nm.Delete(session.key)
		return
	}
	defer dstPC.Close()

	go func() {
		timeout, step := 2*time.Minute, 5*time.Second
		buf := pool.GetBuffer(proxy.UDPBufSize)
		defer pool.PutBuffer(buf)

		var t time.Duration
		for {
			if t += step; t == 0 || t > timeout {
				t = timeout
			}

			dstPC.SetReadDeadline(time.Now().Add(t))
			n, addr, err := dstPC.ReadFrom(buf)
			if err != nil {
				break
			}

			tgtAddr, err := net.ResolveUDPAddr("udp", addr.String())
			if err != nil {
				log.F("error in ResolveUDPAddr: %v", err)
				break
			}

			srcPC, err := ListenPacket(tgtAddr)
			if err != nil {
				log.F("[tproxyu] ListenPacket as %s error: %v", tgtAddr, err)
				break
			}

			_, err = srcPC.WriteTo(buf[:n], session.src)
			srcPC.Close()

			if err != nil {
				break
			}
		}

		nm.Delete(session.key)
		close(session.finCh)
	}()

	log.F("[tproxyu] %s <-> %s via %s", session.src, session.dst, dialer.Addr())

	for {
		select {
		case msg := <-session.msgCh:
			_, err = dstPC.WriteTo(msg.msg, msg.dst)
			if err != nil {
				log.F("[tproxyu] writeTo %s error: %v", msg.dst, err)
			}
			pool.PutBuffer(msg.msg)
			msg.msg = nil
		case <-session.finCh:
			return
		}
	}
}

type message struct {
	dst *net.UDPAddr
	msg []byte
}

type session struct {
	key      string
	src, dst *net.UDPAddr
	msgCh    chan message
	finCh    chan struct{}
}

func newSession(key string, src, dst *net.UDPAddr) *session {
	return &session{key, src, dst, make(chan message, 32), make(chan struct{})}
}
