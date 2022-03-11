package trojan

import (
	"encoding/binary"
	"errors"
	"io"
	"net"

	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/pkg/socks"
)

// PktConn is a udp Packet.Conn.
type PktConn struct {
	net.Conn
	target socks.Addr
}

// NewPktConn returns a PktConn.
func NewPktConn(c net.Conn, target socks.Addr) *PktConn {
	return &PktConn{Conn: c, target: target}
}

// ReadFrom implements the necessary function of net.PacketConn.
func (pc *PktConn) ReadFrom(b []byte) (int, net.Addr, error) {
	// ATYP, DST.ADDR, DST.PORT
	tgtAddr, err := socks.ReadAddr(pc.Conn)
	if err != nil {
		return 0, nil, err
	}

	target, err := net.ResolveUDPAddr("udp", tgtAddr.String())
	if err != nil {
		return 0, nil, err
	}

	// TODO: we know that we use it in proxy.CopyUDP and the length of b is enough, check it later.
	if len(b) < 2 {
		return 0, nil, errors.New("buf size is not enough")
	}

	// Length
	if _, err = io.ReadFull(pc.Conn, b[:2]); err != nil {
		return 0, nil, err
	}

	length := int(binary.BigEndian.Uint16(b[:2]))

	if len(b) < length {
		return 0, nil, errors.New("buf size is not enough")
	}

	// CRLF
	if _, err = io.ReadFull(pc.Conn, b[:2]); err != nil {
		return 0, nil, err
	}

	// Payload
	n, err := io.ReadFull(pc.Conn, b[:length])
	if err != nil {
		return n, nil, err
	}

	return n, target, err
}

// WriteTo implements the necessary function of net.PacketConn.
func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	target := pc.target
	if addr != nil {
		target = socks.ParseAddr(addr.String())
	}

	if target == nil {
		return 0, errors.New("invalid addr")
	}

	buf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(buf)

	tgtLen, _ := buf.Write(target)
	binary.Write(buf, binary.BigEndian, uint16(len(b)))
	buf.WriteString("\r\n")
	buf.Write(b)

	n, err := pc.Write(buf.Bytes())
	if n > tgtLen+4 {
		return n - tgtLen - 4, nil
	}

	return 0, err
}
