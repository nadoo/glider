package vmess

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"io"
	"math/rand"
	"net"
	"time"
)

// Request Options
const (
	OptChunkStream        byte = 1
	OptReuseTCPConnection byte = 2
	OptMetadataObfuscate  byte = 4
)

// SEC types
const (
	SecTypeUnknown          byte = 0
	SecTypeLegacy           byte = 1
	SecTypeAuto             byte = 2
	SecTypeAES128GCM        byte = 3
	SecTypeChacha20Poly1305 byte = 4
	SecTypeNone             byte = 5
)

// CMD types
const (
	CmdTCP byte = 1
	CmdUDP byte = 2
)

// Client vmess client
type Client struct {
	user  *User
	atype AType
	addr  Addr
	port  Port

	reqBodyIV   [16]byte
	reqBodyKey  [16]byte
	reqRespV    byte
	respBodyKey [16]byte
	respBodyIV  [16]byte

	net.Conn
	connected bool
}

// NewClient .
func NewClient(uuid, target string) (*Client, error) {
	user, err := NewUser(uuid)
	if err != nil {
		return nil, err
	}

	c := &Client{user: user}

	c.atype, c.addr, c.port, err = ParseAddr(target)
	if err != nil {
		return nil, err
	}

	randBytes := make([]byte, 33)
	rand.Read(randBytes)

	copy(c.reqBodyIV[:], randBytes[:16])
	copy(c.reqBodyKey[:], randBytes[16:32])
	c.reqRespV = randBytes[32]

	c.respBodyIV = md5.Sum(c.reqBodyIV[:])
	c.respBodyKey = md5.Sum(c.reqBodyKey[:])

	return c, nil
}

// EncodeAuthInfo returns HMAC("md5", UUID, UTC) result
func (c *Client) EncodeAuthInfo() []byte {
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(time.Now().UTC().Unix()))

	h := hmac.New(md5.New, c.user.UUID[:])
	h.Write(ts)

	return h.Sum(nil)
}

// EncodeRequest encodes requests to network bytes
func (c *Client) EncodeRequest() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Request
	buf.WriteByte(1)           // Ver
	buf.Write(c.reqBodyIV[:])  // IV
	buf.Write(c.reqBodyKey[:]) // Key
	buf.WriteByte(c.reqRespV)  // V
	buf.WriteByte(0)           // Opt

	// pLen and Sec
	paddingLen := rand.Intn(16)
	pSec := byte(paddingLen<<4) | SecTypeNone // P(4bit) and Sec(4bit)
	buf.WriteByte(pSec)

	buf.WriteByte(0)      // reserved
	buf.WriteByte(CmdTCP) // cmd

	// target
	binary.Write(buf, binary.BigEndian, uint16(c.port)) // port
	buf.WriteByte(byte(c.atype))                        // atype
	buf.Write(c.addr)                                   // addr

	// padding
	if paddingLen > 0 {
		padding := make([]byte, paddingLen)
		rand.Read(padding)
		buf.Write(padding)
	}

	// F
	fnv1a := fnv.New32a()
	fnv1a.Write(buf.Bytes())
	buf.Write(fnv1a.Sum(nil))

	// AES-128-CFB crypt the request：
	// Key：MD5(UUID + []byte('c48619fe-8f02-49e0-b9e9-edf763e17e21'))
	// IV：MD5(X + X + X + X)，X = []byte(timestamp.now) (8 bytes, Big Endian)
	md5hash := md5.New()
	md5hash.Write(c.user.UUID[:])
	md5hash.Write([]byte("c48619fe-8f02-49e0-b9e9-edf763e17e21"))
	key := md5hash.Sum(nil)

	md5hash.Reset()
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(time.Now().UTC().Unix()))
	md5hash.Write(ts)
	md5hash.Write(ts)
	md5hash.Write(ts)
	md5hash.Write(ts)
	iv := md5hash.Sum(nil)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(buf.Bytes(), buf.Bytes())

	return buf.Bytes(), nil
}

// DecodeRespHeader .
func (c *Client) DecodeRespHeader() error {
	block, err := aes.NewCipher(c.respBodyKey[:])
	if err != nil {
		return err
	}

	stream := cipher.NewCFBDecrypter(block, c.respBodyIV[:])
	buf := make([]byte, 4)
	io.ReadFull(c.Conn, buf)
	stream.XORKeyStream(buf, buf)

	if buf[0] != c.reqRespV {
		return errors.New("unexpected response header")
	}

	// TODO: Dynamic port supported
	// if buf[2] != 0 {
	// 	cmd := buf[2]
	// 	dataLen := int32(buf[3])
	// }

	c.connected = true
	return nil

}

// NewConn wraps a net.Conn to VMessConn
func (c *Client) NewConn(rc net.Conn) (net.Conn, error) {
	// AuthInfo
	rc.Write(c.EncodeAuthInfo())

	// Request
	req, err := c.EncodeRequest()
	if err != nil {
		return nil, err
	}
	rc.Write(req)

	c.Conn = rc

	return c, err
}

func (c *Client) Read(b []byte) (n int, err error) {
	if !c.connected {
		c.DecodeRespHeader()
	}

	return c.Conn.Read(b)
}

func (c *Client) Write(b []byte) (n int, err error) {
	return c.Conn.Write(b)
}
