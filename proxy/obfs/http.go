package obfs

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net"
)

// HTTPObfs struct
type HTTPObfs struct {
	obfsHost string
	obfsURI  string
	obfsUA   string
}

// NewHTTPObfs returns a HTTPObfs object
func NewHTTPObfs(obfsHost, obfsURI, obfsUA string) *HTTPObfs {
	return &HTTPObfs{obfsHost, obfsURI, obfsUA}
}

// HTTPObfsConn struct
type HTTPObfsConn struct {
	*HTTPObfs

	net.Conn
	reader io.Reader
}

// NewConn returns a new obfs connection
func (p *HTTPObfs) NewConn(c net.Conn) (net.Conn, error) {
	cc := &HTTPObfsConn{
		Conn:     c,
		HTTPObfs: p,
	}

	// send http header to remote server
	_, err := cc.writeHeader()
	return cc, err
}

func (c *HTTPObfsConn) writeHeader() (int, error) {
	buf := new(bytes.Buffer)
	buf.Write([]byte("GET " + c.obfsURI + " HTTP/1.1\r\n"))
	buf.Write([]byte("Host: " + c.obfsHost + "\r\n"))
	buf.Write([]byte("User-Agent: " + c.obfsUA + "\r\n"))
	buf.Write([]byte("Upgrade: websocket\r\n"))
	buf.Write([]byte("Connection: Upgrade\r\n"))

	p := make([]byte, 16)
	rand.Read(p)
	buf.Write([]byte("Sec-WebSocket-Key: " + base64.StdEncoding.EncodeToString(p) + "\r\n"))

	buf.Write([]byte("\r\n"))

	return c.Conn.Write(buf.Bytes())
}

func (c *HTTPObfsConn) Read(b []byte) (n int, err error) {
	if c.reader == nil {
		r := bufio.NewReader(c.Conn)
		c.reader = r
		for {
			l, _, err := r.ReadLine()
			if err != nil {
				return 0, err
			}

			if len(l) == 0 {
				break
			}
		}
	}

	return c.reader.Read(b)
}
