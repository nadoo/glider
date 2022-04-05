package shadowstream

import (
	"crypto/rand"
	"io"
	"net"
)

type conn struct {
	net.Conn
	Cipher
	r *reader
	w *writer
}

// NewConn wraps a stream-oriented net.Conn with stream cipher encryption/decryption.
func NewConn(c net.Conn, ciph Cipher) net.Conn {
	return &conn{Conn: c, Cipher: ciph}
}

func (c *conn) initReader() error {
	if c.r == nil {
		iv := make([]byte, c.IVSize())
		if _, err := io.ReadFull(c.Conn, iv); err != nil {
			return err
		}
		c.r = &reader{Reader: c.Conn, Stream: c.Decrypter(iv)}
	}
	return nil
}

func (c *conn) Read(b []byte) (int, error) {
	if c.r == nil {
		if err := c.initReader(); err != nil {
			return 0, err
		}
	}
	return c.r.Read(b)
}

func (c *conn) initWriter() error {
	if c.w == nil {
		iv := make([]byte, c.IVSize())
		if _, err := io.ReadFull(rand.Reader, iv); err != nil {
			return err
		}
		if _, err := c.Conn.Write(iv); err != nil {
			return err
		}
		c.w = &writer{Writer: c.Conn, Stream: c.Encrypter(iv)}
	}
	return nil
}

func (c *conn) Write(b []byte) (int, error) {
	if c.w == nil {
		if err := c.initWriter(); err != nil {
			return 0, err
		}
	}
	return c.w.Write(b)
}
