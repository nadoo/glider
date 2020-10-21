package proxy

import (
	"bufio"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/nadoo/glider/pool"
)

const (
	// TCPBufSize is the size of tcp buffer.
	TCPBufSize = 32 << 10

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
	if conn, ok := c.(*Conn); ok {
		return conn
	}
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

	if err1 != nil && !errors.Is(err1, os.ErrDeadlineExceeded) {
		return err1
	}

	if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	}

	return nil
}

func worthReadFrom(src io.Reader) bool {
	switch v := src.(type) {
	case *net.TCPConn, *net.UnixConn:
		return true
	case *os.File:
		fi, err := v.Stat()
		if err != nil {
			return false
		}
		return fi.Mode().IsRegular()
	case *io.LimitedReader:
		return worthReadFrom(v.R)
	default:
		return false
	}
}

// Copy copies from src to dst.
// it will try to avoid memory allocating by using WriteTo or ReadFrom method,
// if both failed, then it'll fallback to call CopyBuffer method.
func Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}
	if rt, ok := dst.(io.ReaderFrom); ok && worthReadFrom(src) {
		return rt.ReadFrom(src)
	}
	return CopyBuffer(dst, src)
}

// CopyN copies n bytes (or until an error) from src to dst.
func CopyN(dst io.Writer, src io.Reader, n int64) (written int64, err error) {
	written, err = Copy(dst, io.LimitReader(src, n))
	if written == n {
		return n, nil
	}
	if written < n && err == nil {
		// src stopped early; must have been EOF.
		err = io.EOF
	}
	return
}

// CopyBuffer copies from src to dst with a userspace buffer.
func CopyBuffer(dst io.Writer, src io.Reader) (written int64, err error) {
	size := TCPBufSize
	if l, ok := src.(*io.LimitedReader); ok && int64(size) > l.N {
		if l.N < 1 {
			size = 1
		} else {
			size = int(l.N)
		}
	}

	buf := pool.GetBuffer(size)
	defer pool.PutBuffer(buf)

	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
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
