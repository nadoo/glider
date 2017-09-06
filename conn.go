package main

import (
	"bufio"
	"io"
	"net"
	"time"
)

type conn struct {
	r *bufio.Reader
	net.Conn
}

func newConn(c net.Conn) conn {
	return conn{bufio.NewReader(c), c}
}

func newConnSize(c net.Conn, n int) conn {
	return conn{bufio.NewReaderSize(c, n), c}
}

func (c conn) Peek(n int) ([]byte, error) {
	return c.r.Peek(n)
}

func (c conn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

func relay(left, right net.Conn) (int64, int64, error) {
	type res struct {
		N   int64
		Err error
	}
	ch := make(chan res)

	go func() {
		n, err := io.Copy(right, left)
		right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
		left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
		ch <- res{n, err}
	}()

	n, err := io.Copy(left, right)
	right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
	left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
	rs := <-ch

	if err == nil {
		err = rs.Err
	}
	return n, rs.N, err
}

// copy from src to dst at target with read timeout
func timedCopy(dst net.PacketConn, target net.Addr, src net.PacketConn, timeout time.Duration, srcIncluded bool) error {
	buf := make([]byte, udpBufSize)

	for {
		src.SetReadDeadline(time.Now().Add(timeout))
		n, raddr, err := src.ReadFrom(buf)
		if err != nil {
			return err
		}

		if srcIncluded { // server -> client: add original packet source
			srcAddr := ParseAddr(raddr.String())
			copy(buf[len(srcAddr):], buf[:n])
			copy(buf, srcAddr)
			_, err = dst.WriteTo(buf[:len(srcAddr)+n], target)
		} else { // client -> user: strip original packet source
			srcAddr := SplitAddr(buf[:n])
			_, err = dst.WriteTo(buf[len(srcAddr):n], target)
		}

		if err != nil {
			return err
		}
	}
}
