package vless

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"strings"

	"github.com/nadoo/glider/common/pool"
	"github.com/nadoo/glider/proxy"
)

const Version byte = 0

// CMD types.
const (
	CmdTCP byte = 1
	CmdUDP byte = 2
)

// Conn is a vless client connection.
type Conn struct {
	net.Conn
	rcved bool
}

// NewConn returns a new vless client conn.
func NewConn(c net.Conn, uuid [16]byte, target string) (*Conn, error) {
	atyp, addr, port, err := ParseAddr(target)
	if err != nil {
		return nil, err
	}

	buf := pool.GetWriteBuffer()
	defer pool.PutWriteBuffer(buf)

	buf.WriteByte(Version) // ver
	buf.Write(uuid[:])     // uuid
	buf.WriteByte(0)       // addLen
	buf.WriteByte(CmdTCP)  // cmd

	// target
	err = binary.Write(buf, binary.BigEndian, uint16(port)) // port
	if err != nil {
		return nil, err
	}
	buf.WriteByte(byte(atyp)) // atyp
	buf.Write(addr)           //addr

	_, err = c.Write(buf.Bytes())
	return &Conn{Conn: c}, err
}

func (c *Conn) Read(b []byte) (n int, err error) {
	if !c.rcved {
		buf := pool.GetBuffer(2)
		defer pool.PutBuffer(buf)

		n, err = io.ReadFull(c.Conn, buf)
		if err != nil {
			return
		}

		if buf[0] != Version {
			return n, errors.New("version not supported")
		}

		if addLen := int64(buf[1]); addLen > 0 {
			proxy.CopyN(ioutil.Discard, c.Conn, addLen)
		}
		c.rcved = true
	}

	return c.Conn.Read(b)
}

// StrToUUID converts string to uuid.
// s fomat: "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
func StrToUUID(s string) (uuid [16]byte, err error) {
	b := []byte(strings.Replace(s, "-", "", -1))
	if len(b) != 32 {
		return uuid, errors.New("invalid UUID: " + s)
	}
	_, err = hex.Decode(uuid[:], b)
	return
}
