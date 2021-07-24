package trojan

import (
	"encoding/binary"
	"errors"
	"io"
	"net"

	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy/protocol/socks"
)

// PktConn is a udp Packet.Conn.
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
// NOTE: the underlying connection is not udp, we returned the target address here,
// it's not the vless server's address, do not WriteTo it.
func (pc *PktConn) ReadFrom(b []byte) (int, net.Addr, error) {
	// ATYP, DST.ADDR, DST.PORT
	_, err := socks.ReadAddr(pc.Conn)
	if err != nil {
		return 0, nil, err
	}

	// TODO: we know that we use it in proxy.RelayUDP and the length of b is enough, check it later.
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

	// TODO: check the addr in return value, it's a fake packetConn so the addr is not valid
	return n, pc.tgtAddr, err
}

// WriteTo implements the necessary function of net.PacketConn.
func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	buf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(buf)

	tgtLen, _ := buf.Write(pc.tgtAddr)
	binary.Write(buf, binary.BigEndian, uint16(len(b)))
	buf.WriteString("\r\n")
	buf.Write(b)

	n, err := pc.Write(buf.Bytes())
	if n > tgtLen+4 {
		return n - tgtLen - 4, nil
	}

	return 0, err
}
