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

	c = NewServerConn(c)
	cmd, err := s.readHeader(c)
	if err != nil {
		log.F("[vless] verify header error: %v", err)
		return
	}

	tgt, err := ReadAddrString(c)
	if err != nil {
		log.F("[vless] get target error: %v", err)
		return
	}

	switch cmd {
	case CmdTCP:
		s.ServeTCP(c, tgt)
	case CmdUDP:
		s.ServeUOT(c, tgt)
	}
}

// ServeTCP serves tcp requests.
func (s *VLess) ServeTCP(c net.Conn, tgt string) {
	dialer := s.proxy.NextDialer(tgt)
	rc, err := dialer.Dial("tcp", tgt)
	if err != nil {
		log.F("[vless] %s <-> %s via %s, error in dial: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		return
	}
	defer rc.Close()

	log.F("[vless] %s <-> %s via %s", c.RemoteAddr(), tgt, dialer.Addr())

	if err = proxy.Relay(c, rc); err != nil {
		log.F("[vless] %s <-> %s via %s, relay error: %v", c.RemoteAddr(), tgt, dialer.Addr(), err)
		// record remote conn failure only
		if !strings.Contains(err.Error(), s.addr) {
			s.proxy.Record(dialer, false)
		}
	}
}

// ServeUOT serves udp over tcp requests.
func (s *VLess) ServeUOT(c net.Conn, tgt string) {
}

func (s *VLess) readHeader(r io.Reader) (CmdType, error) {
	buf := pool.GetBuffer(16)
	defer pool.PutBuffer(buf)

	// ver
	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return CmdErr, fmt.Errorf("get version error: %v", err)
	}

	if buf[0] != Version {
		return CmdErr, fmt.Errorf("version %d not supported", buf[0])
	}

	// uuid
	if _, err := io.ReadFull(r, buf[:16]); err != nil {
		return CmdErr, fmt.Errorf("get uuid error: %v", err)
	}

	if !bytes.Equal(s.uuid[:], buf) {
		return CmdErr, fmt.Errorf("auth failed, client id: %02x", buf[:16])
	}

	// addLen
	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return CmdErr, fmt.Errorf("get addon length error: %v", err)
	}

	// ignore addons
	if addLen := int64(buf[0]); addLen > 0 {
		proxy.CopyN(ioutil.Discard, r, addLen)
	}

	// cmd
	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return CmdErr, fmt.Errorf("get cmd error: %v", err)
	}

	return CmdType(buf[0]), nil
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
