// https://www.ietf.org/rfc/rfc5246.txt
// https://golang.org/src/crypto/tls/handshake_messages.go

// NOTE:
// https://github.com/shadowsocks/simple-obfs/blob/master/src/obfs_tls.c
// The official obfs-server only checks 6 static bytes of client hello packet,
// so if we send a malformed packet, e.g: set a wrong length number of extensions,
// obfs-server will treat it as a correct packet, but in wireshak, it's malformed.

package obfs

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"time"
)

const (
	lenSize   = 2
	chunkSize = 1 << 13 // 8192
)

// TLSObfs struct
type TLSObfs struct {
	obfsHost string
}

// NewTLSObfs returns a TLSObfs object
func NewTLSObfs(obfsHost string) *TLSObfs {
	return &TLSObfs{obfsHost: obfsHost}
}

// TLSObfsConn struct
type TLSObfsConn struct {
	*TLSObfs

	net.Conn
	reqSent   bool
	reader    *bufio.Reader
	buf       []byte
	leftBytes int
}

// NewConn returns a new obfs connection
func (p *TLSObfs) NewConn(c net.Conn) (net.Conn, error) {
	cc := &TLSObfsConn{
		Conn:    c,
		TLSObfs: p,
		buf:     make([]byte, lenSize),
	}

	return cc, nil
}

func (c *TLSObfsConn) Write(b []byte) (int, error) {
	if !c.reqSent {
		c.reqSent = true
		return c.handshake(b)
	}

	n := len(b)
	for i := 0; i < n; i += chunkSize {
		end := i + chunkSize
		if end > n {
			end = n
		}

		buf := new(bytes.Buffer)
		buf.Write([]byte{0x17, 0x03, 0x03})
		binary.Write(buf, binary.BigEndian, uint16(len(b[i:end])))
		buf.Write(b[i:end])

		_, err := c.Conn.Write(buf.Bytes())
		if err != nil {
			return 0, err
		}
	}

	return n, nil
}

func (c *TLSObfsConn) Read(b []byte) (int, error) {
	if c.reader == nil {
		c.reader = bufio.NewReader(c.Conn)
		// Server Hello
		// TLSv1.2 Record Layer: Handshake Protocol: Server Hello (96 bytes)
		// TLSv1.2 Record Layer: Change Cipher Spec Protocol: Change Cipher Spec (6 bytes)
		c.reader.Discard(102)
	}

	if c.leftBytes == 0 {
		// TLSv1.2 Record Layer:
		// 1st packet: handshake encrypted message / following packets: application data
		// 1 byte: Content Type: Handshake (22) / Application Data (23)
		// 2 bytes: Version: TLS 1.2 (0x0303)
		c.reader.Discard(3)

		// get length
		_, err := io.ReadFull(c.reader, c.buf[:lenSize])
		if err != nil {
			return 0, err
		}

		c.leftBytes = int(binary.BigEndian.Uint16(c.buf[:lenSize]))
	}

	readLen := len(b)
	if readLen > c.leftBytes {
		readLen = c.leftBytes
	}

	m, err := c.reader.Read(b[:readLen])
	if err != nil {
		return 0, err
	}

	c.leftBytes -= m

	return m, nil
}

func (c *TLSObfsConn) handshake(b []byte) (int, error) {
	buf := new(bytes.Buffer)

	// prepare extension & clientHello content
	bufExt, bufHello := extension(b, c.obfsHost), clientHello()

	// prepare lengths
	extLen := bufExt.Len()
	helloLen := bufHello.Len() + 2 + extLen // 2: len(extContentLength)
	handshakeLen := 4 + helloLen            // 1: len(0x01) + 3: len(clientHelloContentLength)

	// TLS Record Layer Begin
	// Content Type: Handshake (22)
	buf.WriteByte(0x16)

	// Version: TLS 1.0 (0x0301)
	buf.Write([]byte{0x03, 0x01})

	// length
	binary.Write(buf, binary.BigEndian, uint16(handshakeLen))

	// Handshake Begin
	// Handshake Type: Client Hello (1)
	buf.WriteByte(0x01)

	// length: uint24(3 bytes), but golang doesn't have this type
	buf.Write([]byte{uint8(helloLen >> 16), uint8(helloLen >> 8), uint8(helloLen)})

	// clientHello content
	buf.Write(bufHello.Bytes())

	// Extension Begin
	// ext content length
	binary.Write(buf, binary.BigEndian, uint16(extLen))

	// ext content
	buf.Write(bufExt.Bytes())

	_, err := c.Conn.Write(buf.Bytes())
	if err != nil {
		return 0, err
	}

	return len(b), nil
}

func clientHello() *bytes.Buffer {
	buf := new(bytes.Buffer)

	// Version: TLS 1.2 (0x0303)
	buf.Write([]byte{0x03, 0x03})

	// Random
	// https://tools.ietf.org/id/draft-mathewson-no-gmtunixtime-00.txt
	// NOTE:
	// Most tls implementations do not deal with the first 4 bytes unix time,
	// clients do not send current time, and server do not check it,
	// golang tls client and chrome browser send random bytes instead.
	//
	binary.Write(buf, binary.BigEndian, uint32(time.Now().Unix()))
	random := make([]byte, 28)
	// The above 2 lines of codes was added to make it compatible with some server implementation,
	// if we don't need the compatibility, just use the following code instead.
	// random := make([]byte, 32)

	rand.Read(random)
	buf.Write(random)

	// Session ID Length: 32
	buf.WriteByte(32)
	// Session ID
	sessionID := make([]byte, 32)
	rand.Read(sessionID)
	buf.Write(sessionID)

	// https://github.com/shadowsocks/simple-obfs/blob/7659eeccf473aa41eb294e92c32f8f60a8747325/src/obfs_tls.c#L57
	// Cipher Suites Length: 56
	binary.Write(buf, binary.BigEndian, uint16(56))
	// Cipher Suites (28 suites)
	buf.Write([]byte{
		0xc0, 0x2c, 0xc0, 0x30, 0x00, 0x9f, 0xcc, 0xa9, 0xcc, 0xa8, 0xcc, 0xaa, 0xc0, 0x2b, 0xc0, 0x2f,
		0x00, 0x9e, 0xc0, 0x24, 0xc0, 0x28, 0x00, 0x6b, 0xc0, 0x23, 0xc0, 0x27, 0x00, 0x67, 0xc0, 0x0a,
		0xc0, 0x14, 0x00, 0x39, 0xc0, 0x09, 0xc0, 0x13, 0x00, 0x33, 0x00, 0x9d, 0x00, 0x9c, 0x00, 0x3d,
		0x00, 0x3c, 0x00, 0x35, 0x00, 0x2f, 0x00, 0xff,
	})

	// Compression Methods Length: 1
	buf.WriteByte(0x01)
	// Compression Methods (1 method)
	buf.WriteByte(0x00)

	return buf
}

func extension(b []byte, server string) *bytes.Buffer {
	buf := new(bytes.Buffer)

	// Extension: SessionTicket TLS
	buf.Write([]byte{0x00, 0x23}) // type
	// NOTE: send some data in sessionticket, the server will treat it as data too
	binary.Write(buf, binary.BigEndian, uint16(len(b))) // length
	buf.Write(b)

	// Extension: server_name
	buf.Write([]byte{0x00, 0x00})                              // type
	binary.Write(buf, binary.BigEndian, uint16(len(server)+5)) // length
	binary.Write(buf, binary.BigEndian, uint16(len(server)+3)) // Server Name list length
	buf.WriteByte(0x00)                                        // Server Name Type: host_name (0)
	binary.Write(buf, binary.BigEndian, uint16(len(server)))   // Server Name length
	buf.Write([]byte(server))

	// https://github.com/shadowsocks/simple-obfs/blob/7659eeccf473aa41eb294e92c32f8f60a8747325/src/obfs_tls.c#L88
	// Extension: ec_point_formats (len=4)
	buf.Write([]byte{0x00, 0x0b})                  // type
	binary.Write(buf, binary.BigEndian, uint16(4)) // length
	buf.WriteByte(0x03)                            // format length
	buf.Write([]byte{0x01, 0x00, 0x02})

	// Extension: supported_groups (len=10)
	buf.Write([]byte{0x00, 0x0a})                   // type
	binary.Write(buf, binary.BigEndian, uint16(10)) // length
	binary.Write(buf, binary.BigEndian, uint16(8))  // Supported Groups List Length: 8
	buf.Write([]byte{0x00, 0x1d, 0x00, 0x17, 0x00, 0x19, 0x00, 0x18})

	// Extension: signature_algorithms (len=32)
	buf.Write([]byte{0x00, 0x0d})                   // type
	binary.Write(buf, binary.BigEndian, uint16(32)) // length
	binary.Write(buf, binary.BigEndian, uint16(30)) // Signature Hash Algorithms Length: 30
	buf.Write([]byte{
		0x06, 0x01, 0x06, 0x02, 0x06, 0x03, 0x05, 0x01, 0x05, 0x02, 0x05, 0x03, 0x04, 0x01, 0x04, 0x02,
		0x04, 0x03, 0x03, 0x01, 0x03, 0x02, 0x03, 0x03, 0x02, 0x01, 0x02, 0x02, 0x02, 0x03,
	})

	// Extension: encrypt_then_mac (len=0)
	buf.Write([]byte{0x00, 0x16})                  // type
	binary.Write(buf, binary.BigEndian, uint16(0)) // length

	// Extension: extended_master_secret (len=0)
	buf.Write([]byte{0x00, 0x17})                  // type
	binary.Write(buf, binary.BigEndian, uint16(0)) // length

	return buf
}
