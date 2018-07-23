package ws

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/textproto"
	"strings"
)

var keyGUID = []byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11")

// Client ws client
type Client struct {
	host string
	path string
}

// Conn is a connection to ws server
type Conn struct {
	net.Conn
	reader io.Reader
	writer io.Writer
}

// NewClient .
func NewClient(host, path string) (*Client, error) {
	if path == "" {
		path = "/"
	}
	c := &Client{host: host, path: path}
	return c, nil
}

// NewConn .
func (c *Client) NewConn(rc net.Conn, target string) (*Conn, error) {
	conn := &Conn{Conn: rc}
	return conn, conn.Handshake(c.host, c.path)
}

// Handshake handshakes with the server using HTTP to request a protocol upgrade
func (c *Conn) Handshake(host, path string) error {
	clientKey := generateClientKey()

	var buf bytes.Buffer
	buf.Write([]byte("GET " + path + " HTTP/1.1\r\n"))
	buf.Write([]byte("Host: " + host + "\r\n"))
	buf.Write([]byte("Upgrade: websocket\r\n"))
	buf.Write([]byte("Connection: Upgrade\r\n"))
	buf.Write([]byte("Origin: http://" + host + "\r\n"))
	buf.Write([]byte("Sec-WebSocket-Key: " + clientKey + "\r\n"))
	buf.Write([]byte("Sec-WebSocket-Protocol: binary\r\n"))
	buf.Write([]byte("Sec-WebSocket-Version: 13\r\n"))
	buf.Write([]byte("\r\n"))

	if _, err := c.Conn.Write(buf.Bytes()); err != nil {
		return err
	}

	tpr := textproto.NewReader(bufio.NewReader(c.Conn))
	_, code, _, ok := parseFirstLine(tpr)
	if !ok || code != "101" {
		return errors.New("[ws] error in ws handshake parseFirstLine")
	}

	respHeader, err := tpr.ReadMIMEHeader()
	if err != nil {
		return err
	}

	serverKey := respHeader.Get("Sec-WebSocket-Accept")
	if serverKey != computeServerKey(clientKey) {
		return errors.New("[ws] error in ws handshake, got wrong Sec-Websocket-Key")
	}

	return nil
}

func (c *Conn) Write(b []byte) (n int, err error) {
	if c.writer == nil {
		c.writer = FrameWriter(c.Conn)
	}

	return c.writer.Write(b)
}

func (c *Conn) Read(b []byte) (n int, err error) {
	if c.reader == nil {
		c.reader = FrameReader(c.Conn)
	}

	return c.reader.Read(b)
}

// parseFirstLine parses "GET /foo HTTP/1.1" OR "HTTP/1.1 200 OK" into its three parts.
// TODO: move to seperate http lib package for reuse(also for http proxy module)
func parseFirstLine(tp *textproto.Reader) (r1, r2, r3 string, ok bool) {
	line, err := tp.ReadLine()
	// log.F("first line: %s", line)
	if err != nil {
		// log.F("[ws] read first line error:%s", err)
		return
	}

	s1 := strings.Index(line, " ")
	s2 := strings.Index(line[s1+1:], " ")
	if s1 < 0 || s2 < 0 {
		return
	}
	s2 += s1 + 1
	return line[:s1], line[s1+1 : s2], line[s2+1:], true
}

func generateClientKey() string {
	p := make([]byte, 16)
	rand.Read(p)
	return base64.StdEncoding.EncodeToString(p)
}

func computeServerKey(clientKey string) string {
	h := sha1.New()
	h.Write([]byte(clientKey))
	h.Write(keyGUID)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
