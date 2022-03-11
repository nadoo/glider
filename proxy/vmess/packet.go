package vmess

import (
	"net"
)

// PktConn is a udp Packet.Conn.
type PktConn struct {
	net.Conn
	target *net.UDPAddr
}

// NewPktConn returns a PktConn.
func NewPktConn(c net.Conn, target *net.UDPAddr) *PktConn {
	return &PktConn{Conn: c, target: target}
}

// ReadFrom implements the necessary function of net.PacketConn.
func (pc *PktConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, err := pc.Read(b)
	return n, pc.target, err
}

// WriteTo implements the necessary function of net.PacketConn.
func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return pc.Write(b)
}
