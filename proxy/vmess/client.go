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
	"strings"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

// Request Options
const (
	OptBasicFormat byte = 0
	OptChunkStream byte = 1
	// OptReuseTCPConnection byte = 2
	// OptMetadataObfuscate  byte = 4
)

// Security types
const (
	SecurityAES128GCM        byte = 3
	SecurityChacha20Poly1305 byte = 4
	SecurityNone             byte = 5
)

// CMD types
const (
	CmdTCP byte = 1
	CmdUDP byte = 2
)

// Client vmess client
type Client struct {
	users    []*User
	count    int
	opt      byte
	security byte
}

// Conn is a connection to vmess server
type Conn struct {
	user     *User
	opt      byte
	security byte

	atyp Atyp
	addr Addr
	port Port

	reqBodyIV   [16]byte
	reqBodyKey  [16]byte
	reqRespV    byte
	respBodyIV  [16]byte
	respBodyKey [16]byte

	net.Conn
	dataReader io.Reader
	dataWriter io.Writer
}

// NewClient .
func NewClient(uuidStr, security string, alterID int) (*Client, error) {
	uuid, err := StrToUUID(uuidStr)
	if err != nil {
		return nil, err
	}

	c := &Client{}
	user := NewUser(uuid)
	c.users = append(c.users, user)
	c.users = append(c.users, user.GenAlterIDUsers(alterID)...)
	c.count = len(c.users)

	c.opt = OptChunkStream

	security = strings.ToLower(security)
	switch security {
	case "aes-128-gcm":
		c.security = SecurityAES128GCM
	case "chacha20-poly1305":
		c.security = SecurityChacha20Poly1305
	case "none":
		c.security = SecurityNone
	case "":
		// NOTE: use basic format when no method specified
		c.opt = OptBasicFormat
		c.security = SecurityNone
	default:
		return nil, errors.New("unknown security type: " + security)
	}

	// NOTE: give rand a new seed to avoid the same sequence of values
	rand.Seed(time.Now().UnixNano())

	return c, nil
}

// NewConn .
func (c *Client) NewConn(rc net.Conn, target string) (*Conn, error) {
	r := rand.Intn(c.count)
	conn := &Conn{user: c.users[r], opt: c.opt, security: c.security}

	var err error
	conn.atyp, conn.addr, conn.port, err = ParseAddr(target)
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
	_, err = rc.Write(conn.EncodeAuthInfo())
	if err != nil {
		return nil, err
	}
	// Request
	req, err := conn.EncodeRequest()
	if err != nil {
		return nil, err
	}

	_, err = rc.Write(req)
	if err != nil {
		return nil, err
	}

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
	buf.WriteByte(c.opt)       // Opt

	// pLen and Sec
	paddingLen := rand.Intn(16)
	pSec := byte(paddingLen<<4) | c.security // P(4bit) and Sec(4bit)
	buf.WriteByte(pSec)

	buf.WriteByte(0)      // reserved
	buf.WriteByte(CmdTCP) // cmd

	// target
	err := binary.Write(buf, binary.BigEndian, uint16(c.port)) // port
	if err != nil {
		return nil, err
	}

	buf.WriteByte(byte(c.atyp)) // atyp
	buf.Write(c.addr)           // addr

	// padding
	if paddingLen > 0 {
		padding := make([]byte, paddingLen)
		rand.Read(padding)
		buf.Write(padding)
	}

	// F
	fnv1a := fnv.New32a()
	_, err = fnv1a.Write(buf.Bytes())
	if err != nil {
		return nil, err
	}
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
	_, err = io.ReadFull(c.Conn, buf)
	if err != nil {
		return err
	}

	stream.XORKeyStream(buf, buf)

	if buf[0] != c.reqRespV {
		return errors.New("unexpected response header")
	}

	// TODO: Dynamic port support
	if buf[2] != 0 {
		// dataLen := int32(buf[3])
		return errors.New("dynamic port is not supported now")
	}

	return nil
}

func (c *Conn) Write(b []byte) (n int, err error) {
	if c.dataWriter != nil {
		return c.dataWriter.Write(b)
	}

	c.dataWriter = c.Conn
	if c.opt&OptChunkStream == OptChunkStream {
		switch c.security {
		case SecurityNone:
			c.dataWriter = ChunkedWriter(c.Conn)

		case SecurityAES128GCM:
			block, _ := aes.NewCipher(c.reqBodyKey[:])
			aead, _ := cipher.NewGCM(block)
			c.dataWriter = AEADWriter(c.Conn, aead, c.reqBodyIV[:])

		case SecurityChacha20Poly1305:
			key := make([]byte, 32)
			t := md5.Sum(c.reqBodyKey[:])
			copy(key, t[:])
			t = md5.Sum(key[:16])
			copy(key[16:], t[:])
			aead, _ := chacha20poly1305.New(key)
			c.dataWriter = AEADWriter(c.Conn, aead, c.reqBodyIV[:])
		}
	}

	return c.dataWriter.Write(b)
}

func (c *Conn) Read(b []byte) (n int, err error) {
	if c.dataReader != nil {
		return c.dataReader.Read(b)
	}

	err = c.DecodeRespHeader()
	if err != nil {
		return 0, err
	}

	c.dataReader = c.Conn
	if c.opt&OptChunkStream == OptChunkStream {
		switch c.security {
		case SecurityNone:
			c.dataReader = ChunkedReader(c.Conn)

		case SecurityAES128GCM:
			block, _ := aes.NewCipher(c.respBodyKey[:])
			aead, _ := cipher.NewGCM(block)
			c.dataReader = AEADReader(c.Conn, aead, c.respBodyIV[:])

		case SecurityChacha20Poly1305:
			key := make([]byte, 32)
			t := md5.Sum(c.respBodyKey[:])
			copy(key, t[:])
			t = md5.Sum(key[:16])
			copy(key[16:], t[:])
			aead, _ := chacha20poly1305.New(key)
			c.dataReader = AEADReader(c.Conn, aead, c.respBodyIV[:])
		}
	}

	return c.dataReader.Read(b)
}
