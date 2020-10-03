package vless

import (
	"bytes"
	"encoding/binary"
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

	network := "tcp"
	dialer := s.proxy.NextDialer(tgt)
	if cmd == CmdUDP {
		// there is no upstream proxy, just serve it
		if dialer.Addr() == "DIRECT" {
			s.ServeUoT(c, tgt)
			return
		}
		network = "udp"
	}

	rc, err := dialer.Dial(network, tgt)
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
func (s *VLess) ServeUoT(c net.Conn, tgt string) {
	rc, err := net.ListenPacket("udp", "")
	if err != nil {
		log.F("[vless] UDP remote listen error: %v", err)
		return
	}
	defer rc.Close()

	tgtAddr, err := net.ResolveUDPAddr("udp", tgt)
	if err != nil {
		log.F("[vless] error in ResolveUDPAddr: %v", err)
		return
	}

	go func() {
		buf := pool.GetBuffer(proxy.UDPBufSize)
		defer pool.PutBuffer(buf)
		for {
			_, err := io.ReadFull(c, buf[:2])
			if err != nil {
				log.F("[vless] read c error: %s\n", err)
				return
			}

			length := binary.BigEndian.Uint16(buf[:2])
			n, err := io.ReadFull(c, buf[:length])

			_, err = rc.WriteTo(buf[:n], tgtAddr)
			if err != nil {
				log.F("[vless] write rc error: %s\n", err)
				return
			}
		}
	}()

	log.F("[vless] %s <-tcp-> %s - %s <-udp-> %s via DIRECT", c.RemoteAddr(), c.LocalAddr(), rc.LocalAddr(), tgt)

	buf := pool.GetBuffer(proxy.UDPBufSize)
	defer pool.PutBuffer(buf)

	for {
		n, _, err := rc.ReadFrom(buf[2:])
		if err != nil {
			log.F("[vless] read rc error: %v", err)
			break
		}

		binary.BigEndian.PutUint16(buf[:2], uint16(n))
		_, err = c.Write(buf[:2+n])
		if err != nil {
			log.F("[vless] write c error: %v", err)
			break
		}
	}

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
