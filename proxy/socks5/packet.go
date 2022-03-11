package socks5

import (
	"errors"
	"net"

	"github.com/nadoo/glider/pkg/pool"
	"github.com/nadoo/glider/pkg/socks"
)

// PktConn .
type PktConn struct {
	net.PacketConn
	ctrlConn net.Conn // tcp control conn
	writeTo  net.Addr // write to and read from addr
	target   socks.Addr
}

// NewPktConn returns a PktConn, the writeAddr must be *net.UDPAddr or *net.UnixAddr.
func NewPktConn(c net.PacketConn, writeAddr net.Addr, targetAddr socks.Addr, ctrlConn net.Conn) *PktConn {
	pc := &PktConn{
		PacketConn: c,
		writeTo:    writeAddr,
		target:     targetAddr,
		ctrlConn:   ctrlConn,
	}

	if ctrlConn != nil {
		go func() {
			buf := pool.GetBuffer(1)
			defer pool.PutBuffer(buf)
			for {
				_, err := ctrlConn.Read(buf)
				if err, ok := err.(net.Error); ok && err.Timeout() {
					continue
				}
				// log.F("[socks5] dialudp udp associate end")
				return
			}
		}()
	}

	return pc
}

// ReadFrom overrides the original function from net.PacketConn.
func (pc *PktConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, _, target, err := pc.readFrom(b)
	return n, target, err
}

func (pc *PktConn) readFrom(b []byte) (int, net.Addr, net.Addr, error) {
	buf := pool.GetBuffer(len(b))
	defer pool.PutBuffer(buf)

	n, raddr, err := pc.PacketConn.ReadFrom(buf)
	if err != nil {
		return n, raddr, nil, err
	}

	if n < 3 {
		return n, raddr, nil, errors.New("not enough size to get addr")
	}

	// https://www.rfc-editor.org/rfc/rfc1928#section-7
	// +----+------+------+----------+----------+----------+
	// |RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
	// +----+------+------+----------+----------+----------+
	// | 2  |  1   |  1   | Variable |    2     | Variable |
	// +----+------+------+----------+----------+----------+
	tgtAddr := socks.SplitAddr(buf[3:n])
	if tgtAddr == nil {
		return n, raddr, nil, errors.New("can not get target addr")
	}

	target, err := net.ResolveUDPAddr("udp", tgtAddr.String())
	if err != nil {
		return n, raddr, nil, errors.New("wrong target addr")
	}

	if pc.writeTo == nil {
		pc.writeTo = raddr
	}

	if pc.target == nil {
		pc.target = make([]byte, len(tgtAddr))
		copy(pc.target, tgtAddr)
	}

	n = copy(b, buf[3+len(tgtAddr):n])
	return n, raddr, target, err
}

// WriteTo overrides the original function from net.PacketConn.
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

	buf.Write([]byte{0, 0, 0})
	tgtLen, _ := buf.Write(target)
	buf.Write(b)

	n, err := pc.PacketConn.WriteTo(buf.Bytes(), pc.writeTo)
	if n > tgtLen+3 {
		return n - tgtLen - 3, err
	}

	return 0, err
}

// Close .
func (pc *PktConn) Close() error {
	if pc.ctrlConn != nil {
		pc.ctrlConn.Close()
	}

	return pc.PacketConn.Close()
}
