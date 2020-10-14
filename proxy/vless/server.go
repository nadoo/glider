package vless

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strings"

	"github.com/nadoo/glider/log"
	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy"
)

// NewVLessServer returns a vless proxy server.
func NewVLessServer(s string, p proxy.Proxy) (proxy.Server, error) {
	return NewVLess(s, nil, p)
}

// ListenAndServe listen and serves connections.
func (s *VLess) ListenAndServe() {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.F("[vless] failed to listen on %s: %v", s.addr, err)
		return
	}
	defer l.Close()

	log.F("[vless] listening TCP on %s", s.addr)

	for {
		c, err := l.Accept()
		if err != nil {
			log.F("[vless] failed to accept: %v", err)
			continue
		}

		go s.Serve(c)
	}
}

// Serve serves a connection.
func (s *VLess) Serve(c net.Conn) {
	defer c.Close()

	if c, ok := c.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
	}

	headBuf := pool.GetWriteBuffer()
	defer pool.PutWriteBuffer(headBuf)

	c = NewServerConn(c)

	cmd, target, err := s.readHeader(io.TeeReader(c, headBuf))
	if err != nil {
		log.F("[vless] verify header from %s error: %v", c.RemoteAddr(), err)
		if s.fallback != "" {
			s.serveFallback(c, s.fallback, headBuf)
		}
		return
	}

	network := "tcp"
	dialer := s.proxy.NextDialer(target)

	if cmd == CmdUDP {
		// there is no upstream proxy, just serve it
		if dialer.Addr() == "DIRECT" {
			s.ServeUoT(c, target)
			return
		}
		network = "udp"
	}

	rc, err := dialer.Dial(network, target)
	if err != nil {
		log.F("[vless] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), target, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	log.F("[vless] %s <-> %s via %s", c.RemoteAddr(), target, dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[vless] %s <-> %s via %s, relay error: %v", c.RemoteAddr(), target, dialer.Addr(), err)
		// record remote conn failure only
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

func (s *VLess) serveFallback(c net.Conn, tgt string, headBuf *bytes.Buffer) {
	// TODO: should we access fallback directly or via proxy?
	dialer := s.proxy.NextDialer(tgt)
	rc, err := dialer.Dial("tcp", tgt)
	if err != nil {
		log.F("[vless-fallback] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	_, err = rc.Write(headBuf.Bytes())
	if err != nil {
		log.F("[vless-fallback] write to rc error: %v", err)
		return
	}

	log.F("[vless-fallback] %s <-> %s via %s", c.RemoteAddr(), tgt, dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[vless-fallback] %s <-> %s via %s, relay error: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
	}
}

func (s *VLess) readHeader(r io.Reader) (CmdType, string, error) {
	buf := pool.GetBuffer(16)
	defer pool.PutBuffer(buf)

	// ver
	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return CmdErr, "", fmt.Errorf("get version error: %v", err)
	}

	if buf[0] != Version {
		return CmdErr, "", fmt.Errorf("version %d not supported", buf[0])
	}

	// uuid
	if _, err := io.ReadFull(r, buf[:16]); err != nil {
		return CmdErr, "", fmt.Errorf("get uuid error: %v", err)
	}

	if !bytes.Equal(s.uuid[:], buf) {
		return CmdErr, "", fmt.Errorf("auth failed, client id: %02x", buf[:16])
	}

	// addLen
	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return CmdErr, "", fmt.Errorf("get addon length error: %v", err)
	}

	// ignore addons
	if addLen := int64(buf[0]); addLen > 0 {
		proxy.CopyN(ioutil.Discard, r, addLen)
	}

	// cmd
	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return CmdErr, "", fmt.Errorf("get cmd error: %v", err)
	}

	// target
	target, err := ReadAddrString(r)

	return CmdType(buf[0]), target, err
}

// ServeUoT serves udp over tcp requests.
func (s *VLess) ServeUoT(c net.Conn, tgt string) {
	rc, err := net.ListenPacket("udp", "")
	if err != nil {
		log.F("[vless] UDP listen error: %v", err)
		return
	}
	defer rc.Close()

	tgtAddr, err := net.ResolveUDPAddr("udp", tgt)
	if err != nil {
		log.F("[vless] error in ResolveUDPAddr: %v", err)
		return
	}

	pc := NewPktConn(c)

	go func() {
		buf := pool.GetBuffer(proxy.UDPBufSize)
		defer pool.PutBuffer(buf)
		for {
			n, _, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}

			_, err = rc.WriteTo(buf[:n], tgtAddr)
			if err != nil {
				return
			}
		}
	}()

	log.F("[vless] %s <-tcp-> %s - %s <-udp-> %s", c.RemoteAddr(), c.LocalAddr(), rc.LocalAddr(), tgt)

	buf := pool.GetBuffer(proxy.UDPBufSize)
	defer pool.PutBuffer(buf)

	for {
		n, _, err := rc.ReadFrom(buf)
		if err != nil {
			break
		}

		_, err = pc.WriteTo(buf[:n], nil)
		if err != nil {
			break
		}
	}
}

// ServerConn is a vless client connection.
type ServerConn struct {
	net.Conn
	sent bool
}

// NewServerConn returns a new vless client conn.
func NewServerConn(c net.Conn) *ServerConn {
	return &ServerConn{Conn: c}
}

func (c *ServerConn) Write(b []byte) (int, error) {
	if !c.sent {
		buf := pool.GetWriteBuffer()
		defer pool.PutWriteBuffer(buf)

		buf.WriteByte(Version) // ver
		buf.WriteByte(0)       // addonLen

		buf.Write(b)
		c.sent = true

		n, err := c.Conn.Write(buf.Bytes())
		return n - 2, err
	}

	return c.Conn.Write(b)
}
