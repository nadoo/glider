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
	SecTypeUnknown          byte = 0 // don't use in client
	SecTypeLegacy           byte = 1 // don't use in client (aes-128-cfb)
	SecTypeAuto             byte = 2 // don't use in client
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
	users []*User
	count int
}

// Conn is a connection to vmess server
type Conn struct {
	user *User

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
func NewClient(uuidStr string, alterID int) (*Client, error) {
	uuid, err := StrToUUID(uuidStr)
	if err != nil {
		return nil, err
	}

	c := &Client{}
	user := NewUser(uuid)
	c.users = append(c.users, user)
	c.users = append(c.users, user.GenAlterIDUsers(alterID)...)
	c.count = len(c.users)

	return c, nil
}

// NewConn .
func (c *Client) NewConn(rc net.Conn, target string) (*Conn, error) {
	r := rand.Intn(c.count)
	conn := &Conn{user: c.users[r]}

	var err error
	conn.atype, conn.addr, conn.port, err = ParseAddr(target)
	if err != nil {
		return nil, err
	}

	randBytes := make([]byte, 33)
	rand.Read(randBytes)

	copy(conn.reqBodyIV[:], randBytes[:16])
	copy(conn.reqBodyKey[:], randBytes[16:32])
	conn.reqRespV = randBytes[32]

	conn.respBodyIV = md5.Sum(conn.reqBodyIV[:])
	conn.respBodyKey = md5.Sum(conn.reqBodyKey[:])

	// AuthInfo
	rc.Write(conn.EncodeAuthInfo())

	// Request
	req, err := conn.EncodeRequest()
	if err != nil {
		return nil, err
	}
	rc.Write(req)

	conn.Conn = rc

	return conn, nil
}

// EncodeAuthInfo returns HMAC("md5", UUID, UTC) result
func (c *Conn) EncodeAuthInfo() []byte {
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(time.Now().UTC().Unix()))

	h := hmac.New(md5.New, c.user.UUID[:])
	h.Write(ts)

	return h.Sum(nil)
}

// EncodeRequest encodes requests to network bytes
func (c *Conn) EncodeRequest() ([]byte, error) {
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

	block, err := aes.NewCipher(c.user.CmdKey[:])
	if err != nil {
		return nil, err
	}

	stream := cipher.NewCFBEncrypter(block, TimestampHash(time.Now().UTC()))
	stream.XORKeyStream(buf.Bytes(), buf.Bytes())

	return buf.Bytes(), nil
}

// DecodeRespHeader .
func (c *Conn) DecodeRespHeader() error {
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

func (c *Conn) Read(b []byte) (n int, err error) {
	if !c.connected {
		c.DecodeRespHeader()
	}
	return c.Conn.Read(b)
}

func (c *Conn) Write(b []byte) (n int, err error) {
	return c.Conn.Write(b)
}
