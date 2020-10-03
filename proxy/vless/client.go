package vless

import (
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"
	"net"

	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy"
)

// NewVLessDialer returns a vless proxy dialer.
func NewVLessDialer(s string, dialer proxy.Dialer) (proxy.Dialer, error) {
	return NewVLess(s, dialer, nil)
}

// Addr returns forwarder's address.
func (s *VLess) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (s *VLess) Dial(network, addr string) (net.Conn, error) {
	rc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		return nil, err
	}
	return NewClientConn(rc, s.uuid, network, addr)
}

// DialUDP connects to the given address via the proxy.
func (s *VLess) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	c, err := s.Dial("udp", addr)
	if err != nil {
		return nil, nil, err
	}
	pkc := NewPktConn(c)
	return pkc, nil, nil
}

// ClientConn is a vless client connection.
type ClientConn struct {
	net.Conn
	rcved bool
}

// NewClientConn returns a new vless client conn.
func NewClientConn(c net.Conn, uuid [16]byte, network, target string) (*ClientConn, error) {
	atyp, addr, port, err := ParseAddr(target)
	if err != nil {
		return nil, err
	}

	buf := pool.GetWriteBuffer()
	defer pool.PutWriteBuffer(buf)

	buf.WriteByte(Version) // ver
	buf.Write(uuid[:])     // uuid
	buf.WriteByte(0)       // addLen

	cmd := CmdTCP
	if network == "udp" {
		cmd = CmdUDP
	}
	buf.WriteByte(byte(cmd)) // cmd

	// target
	err = binary.Write(buf, binary.BigEndian, uint16(port)) // port
	if err != nil {
		return nil, err
	}
	buf.WriteByte(byte(atyp)) // atyp
	buf.Write(addr)           //addr

	_, err = c.Write(buf.Bytes())
	return &ClientConn{Conn: c}, err
}

func (c *ClientConn) Read(b []byte) (n int, err error) {
	if !c.rcved {
		buf := pool.GetBuffer(2)
		defer pool.PutBuffer(buf)

		n, err = io.ReadFull(c.Conn, buf)
		if err != nil {
			return
		}

		if buf[0] != Version {
			return n, errors.New("version not supported")
		}

		if addLen := int64(buf[1]); addLen > 0 {
			proxy.CopyN(ioutil.Discard, c.Conn, addLen)
		}
		c.rcved = true
	}

	return c.Conn.Read(b)
}
