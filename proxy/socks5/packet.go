package socks5

import (
	"errors"
	"net"

	"github.com/nadoo/glider/common/log"
	"github.com/nadoo/glider/common/pool"
	"github.com/nadoo/glider/common/socks"
)

// PktConn .
type PktConn struct {
	net.PacketConn

	writeAddr net.Addr // write to and read from addr

	tgtAddr   socks.Addr
	tgtHeader bool

	ctrlConn net.Conn // tcp control conn
}

// NewPktConn returns a PktConn.
func NewPktConn(c net.PacketConn, writeAddr net.Addr, tgtAddr socks.Addr, tgtHeader bool, ctrlConn net.Conn) *PktConn {
	pc := &PktConn{
		PacketConn: c,
		writeAddr:  writeAddr,
		tgtAddr:    tgtAddr,
		tgtHeader:  tgtHeader,
		ctrlConn:   ctrlConn}

	if ctrlConn != nil {
		go func() {
			buf := pool.GetBuffer(1)
			defer pool.PutBuffer(buf)
			for {
				_, err := ctrlConn.Read(buf)
				if err, ok := err.(net.Error); ok && err.Timeout() {
					continue
				}
				log.F("[socks5] dialudp udp associate end")
				return
			}
		}()
	}

	return pc
}

// ReadFrom overrides the original function from net.PacketConn.
func (pc *PktConn) ReadFrom(b []byte) (int, net.Addr, error) {
	if !pc.tgtHeader {
		return pc.PacketConn.ReadFrom(b)
	}

	buf := pool.GetBuffer(len(b))
	defer pool.PutBuffer(buf)

	n, raddr, err := pc.PacketConn.ReadFrom(buf)
	if err != nil {
		return n, raddr, err
	}

	if n < 3 {
		return n, raddr, errors.New("not enough size to get addr")
	}

	// https://tools.ietf.org/html/rfc1928#section-7
	// +----+------+------+----------+----------+----------+
	// |RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
	// +----+------+------+----------+----------+----------+
	// | 2  |  1   |  1   | Variable |    2     | Variable |
	// +----+------+------+----------+----------+----------+
	tgtAddr := socks.SplitAddr(buf[3:])
	if tgtAddr == nil {
		return n, raddr, errors.New("can not get addr")
	}
	copy(b, buf[3+len(tgtAddr):])

	//test
	if pc.writeAddr == nil {
		pc.writeAddr = raddr
	}

	if pc.tgtAddr == nil {
		pc.tgtAddr = tgtAddr
	}

	return n - len(tgtAddr) - 3, raddr, err
}

// WriteTo overrides the original function from net.PacketConn.
func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if !pc.tgtHeader {
		return pc.PacketConn.WriteTo(b, addr)
	}

	buf := pool.GetBuffer(3 + len(pc.tgtAddr) + len(b))
	defer pool.PutBuffer(buf)

	copy(buf, []byte{0, 0, 0})
	copy(buf[3:], pc.tgtAddr)
	copy(buf[3+len(pc.tgtAddr):], b)

	return pc.PacketConn.WriteTo(buf, pc.writeAddr)
}

// Close .
func (pc *PktConn) Close() error {
	if pc.ctrlConn != nil {
		pc.ctrlConn.Close()
	}

	return pc.PacketConn.Close()
}
