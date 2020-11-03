package vless

import (
	"encoding/binary"
	"errors"
	"io"
	"net"

	"github.com/nadoo/glider/pool"
)

// PktConn is a udp Packet.Conn.
type PktConn struct{ net.Conn }

// NewPktConn returns a PktConn.
func NewPktConn(c net.Conn) *PktConn { return &PktConn{Conn: c} }

// ReadFrom implements the necessary function of net.PacketConn.
// TODO: we know that we use it in proxy.RelayUDP and the length of b is enough, check it later.
func (pc *PktConn) ReadFrom(b []byte) (int, net.Addr, error) {
	if len(b) < 2 {
		return 0, nil, errors.New("buf size is not enough")
	}

	// Length
	if _, err := io.ReadFull(pc.Conn, b[:2]); err != nil {
		return 0, nil, err
	}
	length := int(binary.BigEndian.Uint16(b[:2]))

	if len(b) < length {
		return 0, nil, errors.New("buf size is not enough")
	}

	// Payload
	n, err := io.ReadFull(pc.Conn, b[:length])
	return n, nil, err
}

// WriteTo implements the necessary function of net.PacketConn.
func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	buf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(buf)

	binary.Write(buf, binary.BigEndian, uint16(len(b)))
	buf.Write(b)

	n, err := pc.Write(buf.Bytes())
	if n > 2 {
		return n - 2, err
	}
	return 0, err
}
