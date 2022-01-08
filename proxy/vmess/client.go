package vmess

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math/rand"
	"net"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/chacha20poly1305"

	"github.com/nadoo/glider/pkg/pool"
)

// Request Options
const (
	OptBasicFormat byte = 0
	OptChunkStream byte = 1
	// OptReuseTCPConnection byte = 2
	OptChunkMasking byte = 4
)

// Security types
const (
	SecurityAES128GCM        byte = 3
	SecurityChacha20Poly1305 byte = 4
	SecurityNone             byte = 5
)

// CmdType is the type of vmess cmd
type CmdType byte

// Cmd Types
const (
	CmdTCP CmdType = 1
	CmdUDP CmdType = 2
)

// Client is a vmess client.
type Client struct {
	users    []*User
	count    int
	opt      byte
	aead     bool
	security byte
}

// Conn is a connection to vmess server.
type Conn struct {
	user     *User
	opt      byte
	aead     bool
	security byte

	atyp Atyp
	addr Addr
	port Port

	reqBodyIV   [16]byte
	reqBodyKey  [16]byte
	reqRespV    byte
	respBodyIV  [16]byte
	respBodyKey [16]byte

	writeChunkSizeParser ChunkSizeEncoder
	readChunkSizeParser  ChunkSizeDecoder

	net.Conn
	dataReader io.Reader
	dataWriter io.Writer
}

// NewClient returns a new vmess client.
func NewClient(uuidStr, security string, alterID int, aead bool) (*Client, error) {
	uuid, err := StrToUUID(uuidStr)
	if err != nil {
		return nil, err
	}

	c := &Client{}
	user := NewUser(uuid)
	c.users = append(c.users, user)
	c.users = append(c.users, user.GenAlterIDUsers(alterID)...)
	c.count = len(c.users)

	c.opt = OptChunkStream | OptChunkMasking
	c.aead = aead

	security = strings.ToLower(security)
	switch security {
	case "aes-128-gcm":
		c.security = SecurityAES128GCM
	case "chacha20-poly1305":
		c.security = SecurityChacha20Poly1305
	case "none":
		c.security = SecurityNone
	case "zero":
		c.security = SecurityNone
		c.opt = OptBasicFormat
	case "":
		c.security = SecurityChacha20Poly1305
		if runtime.GOARCH == "amd64" || runtime.GOARCH == "s390x" || runtime.GOARCH == "arm64" {
			c.security = SecurityAES128GCM
		}
	default:
		return nil, errors.New("unknown security type: " + security)
	}

	// NOTE: give rand a new seed to avoid the same sequence of values
	rand.Seed(time.Now().UnixNano())

	return c, nil
}

// NewConn returns a new vmess conn.
func (c *Client) NewConn(rc net.Conn, target string, cmd CmdType) (*Conn, error) {
	r := rand.Intn(c.count)
	conn := &Conn{user: c.users[r], opt: c.opt, aead: c.aead, security: c.security, Conn: rc}

	var err error
	conn.atyp, conn.addr, conn.port, err = ParseAddr(target)
	if err != nil {
		return nil, err
	}

	randBytes := pool.GetBuffer(32)
	rand.Read(randBytes)
	copy(conn.reqBodyIV[:], randBytes[:16])
	copy(conn.reqBodyKey[:], randBytes[16:32])
	pool.PutBuffer(randBytes)

	conn.reqRespV = byte(rand.Intn(1 << 8))

	if conn.aead {
		bodyIV := sha256.Sum256(conn.reqBodyIV[:])
		bodyKey := sha256.Sum256(conn.reqBodyKey[:])
		copy(conn.respBodyIV[:], bodyIV[:16])
		copy(conn.respBodyKey[:], bodyKey[:16])
	} else {
		conn.respBodyIV = md5.Sum(conn.reqBodyIV[:])
		conn.respBodyKey = md5.Sum(conn.reqBodyKey[:])

		// MD5 Auth
		err = conn.Auth()
		if err != nil {
			return nil, err
		}
	}
	conn.writeChunkSizeParser = NewShakeSizeParser(conn.reqBodyIV[:])
	conn.readChunkSizeParser = NewShakeSizeParser(conn.respBodyIV[:])

	// Request
	err = conn.Request(cmd)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// Auth send auth info: HMAC("md5", UUID, UTC).
func (c *Conn) Auth() error {
	ts := pool.GetBuffer(8)
	defer pool.PutBuffer(ts)

	binary.BigEndian.PutUint64(ts, uint64(time.Now().UTC().Unix()))

	h := hmac.New(md5.New, c.user.UUID[:])
	h.Write(ts)

	_, err := c.Conn.Write(h.Sum(nil))
	return err
}

// Request sends request to server.
func (c *Conn) Request(cmd CmdType) error {
	buf := pool.GetBytesBuffer()
	defer pool.PutBytesBuffer(buf)

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

	buf.WriteByte(0)         // reserved
	buf.WriteByte(byte(cmd)) // cmd

	// target
	binary.Write(buf, binary.BigEndian, uint16(c.port)) // port
	buf.WriteByte(byte(c.atyp))                         // atyp
	buf.Write(c.addr)                                   // addr

	// padding
	if paddingLen > 0 {
		padding := pool.GetBuffer(paddingLen)
		rand.Read(padding)
		buf.Write(padding)
		pool.PutBuffer(padding)
	}

	// F
	fnv1a := fnv.New32a()
	fnv1a.Write(buf.Bytes())
	buf.Write(fnv1a.Sum(nil))

	if c.aead {
		encHeader := sealVMessAEADHeader(c.user.CmdKey, buf.Bytes())
		_, err := c.Conn.Write(encHeader)
		return err
	}

	block, err := aes.NewCipher(c.user.CmdKey[:])
	if err != nil {
		return err
	}

	stream := cipher.NewCFBEncrypter(block, TimestampHash(time.Now().UTC()))
	stream.XORKeyStream(buf.Bytes(), buf.Bytes())

	_, err = c.Conn.Write(buf.Bytes())

	return err
}

// DecodeRespHeader decodes response header.
func (c *Conn) DecodeRespHeader() error {
	if c.aead {
		buf, err := openVMessAEADHeader(c.respBodyKey, c.respBodyIV, c.Conn)
		if err != nil {
			return err
		}

		if len(buf) < 4 {
			return errors.New("unexpected buffer length")
		}

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

	block, err := aes.NewCipher(c.respBodyKey[:])
	if err != nil {
		return err
	}
	stream := cipher.NewCFBDecrypter(block, c.respBodyIV[:])

	buf := pool.GetBuffer(4)
	defer pool.PutBuffer(buf)

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
			c.dataWriter = ChunkedWriter(c.Conn, c.writeChunkSizeParser)

		case SecurityAES128GCM:
			block, _ := aes.NewCipher(c.reqBodyKey[:])
			aead, _ := cipher.NewGCM(block)
			c.dataWriter = AEADWriter(c.Conn, aead, c.reqBodyIV[:], c.writeChunkSizeParser)

		case SecurityChacha20Poly1305:
			key := pool.GetBuffer(32)
			t := md5.Sum(c.reqBodyKey[:])
			copy(key, t[:])
			t = md5.Sum(key[:16])
			copy(key[16:], t[:])
			aead, _ := chacha20poly1305.New(key)
			c.dataWriter = AEADWriter(c.Conn, aead, c.reqBodyIV[:], c.writeChunkSizeParser)
			pool.PutBuffer(key)
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
		return 0, fmt.Errorf("[vmess] error in DecodeRespHeader: %w", err)
	}

	c.dataReader = c.Conn
	if c.opt&OptChunkStream == OptChunkStream {
		switch c.security {
		case SecurityNone:
			c.dataReader = ChunkedReader(c.Conn, c.readChunkSizeParser)

		case SecurityAES128GCM:
			block, _ := aes.NewCipher(c.respBodyKey[:])
			aead, _ := cipher.NewGCM(block)
			c.dataReader = AEADReader(c.Conn, aead, c.respBodyIV[:], c.readChunkSizeParser)

		case SecurityChacha20Poly1305:
			key := pool.GetBuffer(32)
			t := md5.Sum(c.respBodyKey[:])
			copy(key, t[:])
			t = md5.Sum(key[:16])
			copy(key[16:], t[:])
			aead, _ := chacha20poly1305.New(key)
			c.dataReader = AEADReader(c.Conn, aead, c.respBodyIV[:], c.readChunkSizeParser)
			pool.PutBuffer(key)
		}
	}

	return c.dataReader.Read(b)
}
