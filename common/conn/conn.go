package conn

import (
	"bufio"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/nadoo/glider/common/pool"
)

const (
	// TCPBufSize is the size of tcp buffer.
	TCPBufSize = 16 << 10

	// UDPBufSize is the size of udp buffer.
	UDPBufSize = 64 << 10
)

// Conn is a connection with buffered reader.
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
func Relay(left, right net.Conn) error {
	var err, err1 error
	var wg sync.WaitGroup
	var wait = 5 * time.Second

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err1 = Copy(right, left)
		right.SetReadDeadline(time.Now().Add(wait)) // unblock read on right
	}()

	_, err = Copy(left, right)
	left.SetReadDeadline(time.Now().Add(wait)) // unblock read on left
	wg.Wait()

	if err1 != nil && !errors.Is(err1, os.ErrDeadlineExceeded) { // requires Go 1.15+
		return err1
	}

	if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	}

	return nil
}

// Copy copies from src to dst.
func Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	buf := pool.GetBuffer(TCPBufSize)
	defer pool.PutBuffer(buf)

	return io.CopyBuffer(dst, src, buf)
}

// RelayUDP copys from src to dst at target with read timeout.
func RelayUDP(dst net.PacketConn, target net.Addr, src net.PacketConn, timeout time.Duration) error {
	b := pool.GetBuffer(UDPBufSize)
	defer pool.PutBuffer(b)

	for {
		src.SetReadDeadline(time.Now().Add(timeout))
		n, _, err := src.ReadFrom(b)
		if err != nil {
			return err
		}

		_, err = dst.WriteTo(b[:n], target)
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
