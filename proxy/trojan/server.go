package trojan

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/pkg/socks"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterServer("trojan", NewTrojanServer)
	proxy.RegisterServer("trojanc", NewClearTextServer) // cleartext
}

// NewClearTextServer returns a trojan cleartext proxy server.
func NewClearTextServer(s string, p proxy.Proxy) (proxy.Server, error) {
	t, err := NewTrojan(s, nil, p)
	if err != nil {
		return nil, fmt.Errorf("[trojanc] create instance error: %s", err)
	}

	t.withTLS = false
	return t, nil
}

// NewTrojanServer returns a trojan proxy server.
func NewTrojanServer(s string, p proxy.Proxy) (proxy.Server, error) {
	t, err := NewTrojan(s, nil, p)
	if err != nil {
		return nil, fmt.Errorf("[trojan] create instance error: %s", err)
	}

	if t.certFile == "" || t.keyFile == "" {
		return nil, errors.New("[trojan] cert and key file path must be spcified")
	}

	cert, err := tls.LoadX509KeyPair(t.certFile, t.keyFile)
	if err != nil {
		return nil, fmt.Errorf("[trojan] unable to load cert: %s, key %s, error: %s",
			t.certFile, t.keyFile, err)
	}

	t.tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	return t, err
}

// ListenAndServe listen and serves connections.
func (s *Trojan) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.Fatalf("[trojan] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[trojan] listening TCP on %s, with TLS: %v", s.addr, s.withTLS)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[trojan] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve serves a connection.
func (s *Trojan) Serve(c net.Conn) {
	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	if s.withTLS {
		tlsConn := tls.Server(c, s.tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			tlsConn.Close()
			log.F("[trojan] error in tls handshake: %s", err)
			return
		}
		c = tlsConn
	}
	defer c.Close()

	headBuf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(headBuf)

	cmd, target, err := s.readHeader(io.TeeReader(c, headBuf))
	if err != nil {
		// log.F("[trojan] verify header from %s error: %v", c.RemoteAddr(), err)
		if s.fallback != "" {
			s.serveFallback(c, s.fallback, headBuf)
		}
		return
	}

	network := "tcp"
	dialer := s.proxy.NextDialer(target.String())

	if cmd == socks.CmdUDPAssociate {
		// there is no upstream proxy, just serve it
		if dialer.Addr() == "DIRECT" {
			s.ServeUoT(c, target)
			return
		}
		network = "udp"
	}

	rc, err := dialer.Dial(network, target.String())
	if err != nil {
		log.F("[trojan] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), target, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	log.F("[trojan] %s <-> %s via %s", c.RemoteAddr(), target, dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[trojan] %s <-> %s via %s, relay error: %v", c.RemoteAddr(), target, dialer.Addr(), err)
		// record remote conn failure only
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

func (s *Trojan) serveFallback(c net.Conn, tgt string, headBuf *bytes.Buffer) {
	// TODO: should we access fallback directly or via proxy?
	dialer := s.proxy.NextDialer(tgt)
	rc, err := dialer.Dial("tcp", tgt)
	if err != nil {
		log.F("[trojan-fallback] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	_, err = rc.Write(headBuf.Bytes())
	if err != nil {
		log.F("[trojan-fallback] write to rc error: %v", err)
		return
	}

	log.F("[trojan-fallback] %s <-> %s via %s", c.RemoteAddr(), tgt, dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[trojan-fallback] %s <-> %s via %s, relay error: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
	}
}

func (s *Trojan) readHeader(r io.Reader) (byte, socks.Addr, error) {
	// pass: 56, "\r\n": 2, cmd: 1
	buf := pool.GetBuffer(59)
	defer pool.PutBuffer(buf)

	if _, err := io.ReadFull(r, buf); err != nil {
		return socks.CmdError, nil, err
	}

	// pass, 56bytes
	if !bytes.Equal(buf[:56], s.pass[:]) {
		return socks.CmdError, nil, errors.New("wrong password")
	}

	// cmd, 1byte
	cmd := byte(buf[58])

	// target
	tgt, err := socks.ReadAddr(r)
	if err != nil {
		return cmd, nil, fmt.Errorf("read target address error: %v", err)
	}

	// "\r\n", 2bytes
	if _, err := io.ReadFull(r, buf[:2]); err != nil {
		return socks.CmdError, tgt, err
	}

	return cmd, tgt, nil
}

// ServeUoT serves udp over tcp requests.
func (s *Trojan) ServeUoT(c net.Conn, tgt socks.Addr) {
	lc, err := net.ListenPacket("udp", "")
	if err != nil {
		log.F("[trojan] UDP listen error: %v", err)
		return
	}
	defer lc.Close()

	pc := NewPktConn(c, tgt)
	log.F("[trojan] %s <-UoT-> %s <-> %s", c.RemoteAddr(), lc.LocalAddr(), tgt)

	go proxy.CopyUDP(lc, nil, pc, 2*time.Minute, 5*time.Second)
	proxy.CopyUDP(pc, nil, lc, 2*time.Minute, 5*time.Second)
}
