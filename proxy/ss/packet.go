package ss

import (
	"errors"
	"net"

	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy/protocol/socks"
)

// PktConn .
type PktConn struct {
	net.PacketConn

	writeAddr net.Addr // write to and read from addr

	tgtAddr   socks.Addr
	tgtHeader bool
}

// NewPktConn returns a PktConn
func NewPktConn(c net.PacketConn, writeAddr net.Addr, tgtAddr socks.Addr, tgtHeader bool) *PktConn {
	pc := &PktConn{
		PacketConn: c,
		writeAddr:  writeAddr,
		tgtAddr:    tgtAddr,
		tgtHeader:  tgtHeader}
	return pc
}

// ReadFrom overrides the original function from net.PacketConn
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

	tgtAddr := socks.SplitAddr(buf[:n])
	if tgtAddr == nil {
		return n, raddr, errors.New("can not get addr")
	}

	n = copy(b, buf[len(tgtAddr):n])

	//test
	if pc.writeAddr == nil {
		pc.writeAddr = raddr
	}

	if pc.tgtAddr == nil {
		pc.tgtAddr = make([]byte, len(tgtAddr))
		copy(pc.tgtAddr, tgtAddr)
	}

	return n, raddr, err
}

// WriteTo overrides the original function from net.PacketConn
func (pc *PktConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if !pc.tgtHeader {
		return pc.PacketConn.WriteTo(b, addr)
	}

	buf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(buf)

	tgtLen, _ := buf.Write(pc.tgtAddr)
	buf.Write(b)

	n, err := pc.PacketConn.WriteTo(buf.Bytes(), pc.writeAddr)
	if n > tgtLen {
		return n - tgtLen, err
	}

	return 0, err
}
