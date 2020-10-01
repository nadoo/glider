package trojan

import (
	"encoding/binary"
	"errors"
	"io"
	"net"

	"github.com/nadoo/glider/common/pool"
	"github.com/nadoo/glider/common/socks"
	"github.com/nadoo/glider/proxy"
)

// PktConn .
type PktConn struct {
	net.Conn

	tgtAddr socks.Addr
}

// NewPktConn returns a PktConn.
func NewPktConn(c net.Conn, tgtAddr socks.Addr) *PktConn {
	pc := &PktConn{
		Conn:    c,
		tgtAddr: tgtAddr,
	}
	return pc
}

// ReadFrom implements the necessary function of net.PacketConn.
func (pc *PktConn) ReadFrom(b []byte) (int, net.Addr, error) {
	// ATYP, DST.ADDR, DST.PORT
	_, err := socks.ReadAddr(pc.Conn)
	if err != nil {
		return 0, nil, err
	}

	// Length
	if _, err = io.ReadFull(pc.Conn, b[:2]); err != nil {
		return 0, nil, err
	}

	length := int(binary.BigEndian.Uint16(b[:2]))
	if length > proxy.UDPBufSize {
		return 0, nil, errors.New("packet invalid")
	}

	// CRLF
	if _, err = io.ReadFull(pc.Conn, b[:2]); err != nil {
		return 0, nil, err
	}

	// Payload
	n, err := io.ReadFull(pc.Conn, b[:length])
	if err != nil {
		return 0, nil, err
	}

	// TODO: check the addr in return value, it's a fake packetConn so the addr is not valid
	return n, nil, err
}

// WriteTo implements the necessary function of net.PacketConn.
func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	buf := pool.GetWriteBuffer()
	defer pool.PutWriteBuffer(buf)

	buf.Write(pc.tgtAddr)
	binary.Write(buf, binary.BigEndian, uint16(len(b)))
	buf.WriteString("\r\n")
	buf.Write(b)
	return pc.Write(buf.Bytes())
}
