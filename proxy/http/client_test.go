package http

import (
	"bufio"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nadoo/glider/proxy"
)

type connectTestConn struct {
	net.Conn
	closed     atomic.Bool
	writeError error
}

func (c *connectTestConn) Close() error {
	c.closed.Store(true)
	return c.Conn.Close()
}

func (c *connectTestConn) Write(p []byte) (int, error) {
	if c.writeError != nil {
		return 0, c.writeError
	}
	return c.Conn.Write(p)
}

type connectTestDialer struct{ conn net.Conn }

func (d connectTestDialer) Addr() string                          { return "test-proxy" }
func (d connectTestDialer) Dial(string, string) (net.Conn, error) { return d.conn, nil }
func (d connectTestDialer) DialUDP(string, string) (net.PacketConn, error) {
	return nil, proxy.ErrNotSupported
}

func TestDialClosesFailedCONNECT(t *testing.T) {
	for _, tt := range []struct {
		name       string
		response   string
		writeError error
	}{
		{name: "request write", writeError: errors.New("write failed")},
		{name: "response EOF"},
		{name: "invalid status", response: "invalid\r\n\r\n"},
		{name: "rejected status", response: "HTTP/1.1 407 Proxy Authentication Required\r\n\r\n"},
		{name: "invalid headers", response: "HTTP/1.1 200 OK\r\ninvalid\r\n\r\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { client.Close(); server.Close() })
			client.SetDeadline(time.Now().Add(3 * time.Second))
			tracked := &connectTestConn{Conn: client, writeError: tt.writeError}
			if tt.writeError == nil {
				go func() {
					r := bufio.NewReader(server)
					for {
						line, err := r.ReadString('\n')
						if err != nil {
							return
						}
						if line == "\r\n" {
							break
						}
					}
					if tt.response == "" {
						server.Close()
						return
					}
					io.WriteString(server, tt.response)
				}()
			}
			h := &HTTP{addr: "proxy.example:8080", dialer: connectTestDialer{tracked}}
			conn, err := h.Dial("tcp", "target.example:443")
			if err == nil {
				t.Error("Dial succeeded for a failed CONNECT response")
			}
			if conn != nil {
				t.Error("Dial returned a connection after a failed CONNECT")
			}
			if !tracked.closed.Load() {
				t.Error("failed CONNECT left its connection open")
			}
		})
	}
}

func TestDialPreservesBufferedTunnelData(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })
	client.SetDeadline(time.Now().Add(3 * time.Second))
	tracked := &connectTestConn{Conn: client}
	go func() {
		r := bufio.NewReader(server)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if line == "\r\n" {
				break
			}
		}
		io.WriteString(server, "HTTP/1.1 200 Connection established\r\nProxy-Agent: test\r\n\r\nhello")
	}()
	h := &HTTP{addr: "proxy.example:8080", dialer: connectTestDialer{tracked}}
	conn, err := h.Dial("tcp", "target.example:443")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if tracked.closed.Load() {
		t.Fatal("successful tunnel was closed")
	}
	data := make([]byte, 5)
	if _, err := io.ReadFull(conn, data); err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("tunnel data = %q", data)
	}
}
