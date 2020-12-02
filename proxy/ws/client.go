package ws

import (
	"errors"
	"io"
	"net"
	"net/textproto"

	"github.com/nadoo/glider/pool"
	"github.com/nadoo/glider/proxy"
)

func init() {
	proxy.RegisterDialer("ws", NewWSDialer)
}

// NewWSDialer returns a ws proxy dialer.
func NewWSDialer(s string, d proxy.Dialer) (proxy.Dialer, error) {
	return NewWS(s, d, nil)
}

// Addr returns forwarder's address.
func (s *WS) Addr() string {
	if s.addr == "" {
		return s.dialer.Addr()
	}
	return s.addr
}

// Dial connects to the address addr on the network net via the proxy.
func (s *WS) Dial(network, addr string) (net.Conn, error) {
	rc, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		return nil, err
	}

	return s.NewClientConn(rc)
}

// DialUDP connects to the given address via the proxy.
func (s *WS) DialUDP(network, addr string) (net.PacketConn, net.Addr, error) {
	return nil, nil, proxy.ErrNotSupported
}

// ClientConn is a connection to ws server.
type ClientConn struct {
	net.Conn
	reader io.Reader
	writer io.Writer
}

// NewClientConn creates a new ws client connection.
func (s *WS) NewClientConn(rc net.Conn) (*ClientConn, error) {
	conn := &ClientConn{Conn: rc}
	return conn, conn.Handshake(s.host, s.path, s.origin)
}

// Handshake handshakes with the server using HTTP to request a protocol upgrade.
func (c *ClientConn) Handshake(host, path, origin string) error {
	clientKey := generateClientKey()

	buf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(buf)

	buf.WriteString("GET " + path + " HTTP/1.1\r\n")
	buf.WriteString("Host: " + host + "\r\n")
	buf.WriteString("Upgrade: websocket\r\n")
	buf.WriteString("Connection: Upgrade\r\n")
	if origin != "" {
		buf.WriteString("Origin: http://" + origin + "\r\n")
	}
	buf.WriteString("Sec-WebSocket-Key: " + clientKey + "\r\n")
	buf.WriteString("Sec-WebSocket-Protocol: binary\r\n")
	buf.WriteString("Sec-WebSocket-Version: 13\r\n")
	buf.WriteString(("\r\n"))

	if _, err := c.Conn.Write(buf.Bytes()); err != nil {
		return err
	}

	br := pool.GetBufReader(c.Conn)
	defer pool.PutBufReader(br)

	tpr := textproto.NewReader(br)
	line, err := tpr.ReadLine()
	if err != nil {
		return err
	}

	_, code, _, ok := parseFirstLine(line)
	if !ok || code != "101" {
		return errors.New("[ws] error in ws handshake, got wrong response: " + line)
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

func (c *ClientConn) Write(b []byte) (n int, err error) {
	if c.writer == nil {
		c.writer = FrameWriter(c.Conn, false)
	}
	return c.writer.Write(b)
}

func (c *ClientConn) Read(b []byte) (n int, err error) {
	if c.reader == nil {
		c.reader = FrameReader(c.Conn, false)
	}
	return c.reader.Read(b)
}
