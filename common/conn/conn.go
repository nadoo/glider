package conn

import (
	"bufio"
	"io"
	"net"
	"time"

	"github.com/nadoo/glider/common/pool"
)

const (
	// TCPBufSize is the size of tcp buffer
	TCPBufSize = 16 << 10

	// UDPBufSize is the size of udp buffer
	UDPBufSize = 64 << 10
)

// Conn is a base conn struct
type Conn struct {
	r *bufio.Reader
	net.Conn
}

// NewConn returns a new conn.
func NewConn(c net.Conn) *Conn {
	return &Conn{bufio.NewReader(c), c}
}

// Peek returns the next n bytes without advancing the reader.
func (c *Conn) Peek(n int) ([]byte, error) {
	return c.r.Peek(n)
}

func (c *Conn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

// Reader returns the internal bufio.Reader.
func (c *Conn) Reader() *bufio.Reader {
	return c.r
}

// Relay relays between left and right.
func Relay(left, right net.Conn) (int64, int64, error) {
	type res struct {
		N   int64
		Err error
	}
	ch := make(chan res)

	go func() {
		buf := pool.GetBuffer(TCPBufSize)
		n, err := io.CopyBuffer(right, left, buf)
		pool.PutBuffer(buf)

		right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
		left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
		ch <- res{n, err}
	}()

	buf := pool.GetBuffer(TCPBufSize)
	n, err := io.CopyBuffer(left, right, buf)
	pool.PutBuffer(buf)

	right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
	left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
	rs := <-ch

	if err == nil {
		err = rs.Err
	}
	return n, rs.N, err
}

// RelayUDP copys from src to dst at target with read timeout.
func RelayUDP(dst net.PacketConn, target net.Addr, src net.PacketConn, timeout time.Duration) error {
	buf := pool.GetBuffer(UDPBufSize)
	defer pool.PutBuffer(buf)

	for {
		src.SetReadDeadline(time.Now().Add(timeout))
		n, _, err := src.ReadFrom(buf)
		if err != nil {
			return err
		}

		_, err = dst.WriteTo(buf[:n], target)
		if err != nil {
			return err
		}
	}
}

// OutboundIP returns preferred outbound ip of this machine.
func OutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
