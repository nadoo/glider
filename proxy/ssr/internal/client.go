// source from https://github.com/v2rayA/shadowsocksR
// just copy here to use the builtin buffer pool.
// as this protocol hasn't been maintained since 2017, it doesn't deserve our research to rewrite it.

package internal

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/mzz2017/shadowsocksR/obfs"
	"github.com/mzz2017/shadowsocksR/protocol"

	"github.com/nadoo/glider/pool"
)

const bufSize = 16 << 10

func init() {
	rand.Seed(time.Now().UnixNano())
}

// SSTCPConn the struct that override the net.Conn methods
type SSTCPConn struct {
	net.Conn
	*StreamCipher
	IObfs               obfs.IObfs
	IProtocol           protocol.IProtocol
	readBuf             []byte
	underPostdecryptBuf *bytes.Buffer
	readIndex           uint64
	decryptedBuf        *bytes.Buffer
	writeBuf            []byte
	lastReadError       error
}

func NewSSTCPConn(c net.Conn, cipher *StreamCipher) *SSTCPConn {
	return &SSTCPConn{
		Conn:                c,
		StreamCipher:        cipher,
		readBuf:             pool.GetBuffer(bufSize),
		decryptedBuf:        pool.GetWriteBuffer(),
		underPostdecryptBuf: pool.GetWriteBuffer(),
		writeBuf:            pool.GetBuffer(bufSize),
	}
}

func (c *SSTCPConn) Close() error {
	pool.PutBuffer(c.readBuf)
	pool.PutWriteBuffer(c.decryptedBuf)
	pool.PutWriteBuffer(c.underPostdecryptBuf)
	pool.PutBuffer(c.writeBuf)
	return c.Conn.Close()
}

func (c *SSTCPConn) GetIv() (iv []byte) {
	iv = make([]byte, len(c.IV()))
	copy(iv, c.IV())
	return
}

func (c *SSTCPConn) GetKey() (key []byte) {
	key = make([]byte, len(c.Key()))
	copy(key, c.Key())
	return
}

func (c *SSTCPConn) initEncryptor(b []byte) (iv []byte, err error) {
	if !c.EncryptInited() {
		iv, err = c.InitEncrypt()
		if err != nil {
			return nil, err
		}

		overhead := c.IObfs.GetOverhead() + c.IProtocol.GetOverhead()
		// should initialize obfs/protocol now, because iv is ready now
		obfsServerInfo := c.IObfs.GetServerInfo()
		obfsServerInfo.SetHeadLen(b, 30)
		obfsServerInfo.IV, obfsServerInfo.IVLen = c.IV(), c.InfoIVLen()
		obfsServerInfo.Key, obfsServerInfo.KeyLen = c.Key(), c.InfoKeyLen()
		obfsServerInfo.Overhead = overhead
		c.IObfs.SetServerInfo(obfsServerInfo)

		protocolServerInfo := c.IProtocol.GetServerInfo()
		protocolServerInfo.SetHeadLen(b, 30)
		protocolServerInfo.IV, protocolServerInfo.IVLen = c.IV(), c.InfoIVLen()
		protocolServerInfo.Key, protocolServerInfo.KeyLen = c.Key(), c.InfoKeyLen()
		protocolServerInfo.Overhead = overhead
		c.IProtocol.SetServerInfo(protocolServerInfo)
	}
	return
}

func (c *SSTCPConn) Read(b []byte) (n int, err error) {
	for {
		n, err = c.doRead(b)
		if b == nil || n != 0 || err != nil {
			return n, err
		}
	}
}

func (c *SSTCPConn) doRead(b []byte) (n int, err error) {
	if c.decryptedBuf.Len() > 0 {
		return c.decryptedBuf.Read(b)
	}
	n, err = c.Conn.Read(c.readBuf)
	if n == 0 || err != nil {
		return n, err
	}
	decodedData, needSendBack, err := c.IObfs.Decode(c.readBuf[:n])
	if err != nil {
		//log.Println(c.Conn.LocalAddr().String(), c.IObfs.(*obfs.tls12TicketAuth).handshakeStatus, err)
		return 0, err
	}

	//do send back
	if needSendBack {
		c.Write(nil)
		//log.Println("sendBack")
		return 0, nil
	}
	//log.Println(len(decodedData), needSendBack, err, n)
	if len(decodedData) == 0 {
		//log.Println(string(c.readBuf[:200]))
	}
	decodedDataLen := len(decodedData)
	if decodedDataLen == 0 {
		return 0, nil
	}

	if !c.DecryptInited() {

		if len(decodedData) < c.InfoIVLen() {
			return 0, errors.New(fmt.Sprintf("invalid ivLen:%v, actual length:%v", c.InfoIVLen(), len(decodedData)))
		}
		iv := decodedData[0:c.InfoIVLen()]
		if err = c.InitDecrypt(iv); err != nil {
			return 0, err
		}

		if len(c.IV()) == 0 {
			c.SetIV(iv)
		}
		decodedDataLen -= c.InfoIVLen()
		if decodedDataLen <= 0 {
			return 0, nil
		}
		decodedData = decodedData[c.InfoIVLen():]
	}

	// nadoo: comment out codes here to use pool buffer
	// modify start -->
	// buf := make([]byte, decodedDataLen)
	// // decrypt decodedData and save it to buf
	// c.Decrypt(buf, decodedData)
	// // append buf to c.underPostdecryptBuf
	// c.underPostdecryptBuf.Write(buf)
	// // and read it to buf immediately
	// buf = c.underPostdecryptBuf.Bytes()

	buf1 := pool.GetBuffer(decodedDataLen)
	defer pool.PutBuffer(buf1)

	c.Decrypt(buf1, decodedData)
	c.underPostdecryptBuf.Write(buf1)
	buf := c.underPostdecryptBuf.Bytes()
	// --> modify end

	postDecryptedData, length, err := c.IProtocol.PostDecrypt(buf)
	if err != nil {
		c.underPostdecryptBuf.Reset()
		//log.Println(string(decodebytes))
		//log.Println("err", err)
		return 0, err
	}
	if length == 0 {
		// not enough to postDecrypt
		return 0, nil
	} else {
		c.underPostdecryptBuf.Next(length)
	}

	postDecryptedLength := len(postDecryptedData)
	blength := len(b)
	if blength >= postDecryptedLength {
		copy(b, postDecryptedData)
		return postDecryptedLength, nil
	}
	copy(b, postDecryptedData[:blength])
	c.decryptedBuf.Write(postDecryptedData[blength:])
	return blength, nil
}

func (c *SSTCPConn) preWrite(b []byte) (outData []byte, err error) {
	if b == nil {
		b = make([]byte, 0)
	}
	var iv []byte
	if iv, err = c.initEncryptor(b); err != nil {
		return
	}

	var preEncryptedData []byte
	preEncryptedData, err = c.IProtocol.PreEncrypt(b)
	if err != nil {
		return
	}
	preEncryptedDataLen := len(preEncryptedData)
	//! \attention here the expected output buffer length MUST be accurate, it is preEncryptedDataLen now!

	cipherData := c.writeBuf
	dataSize := preEncryptedDataLen + len(iv)
	if dataSize > len(cipherData) {
		cipherData = make([]byte, dataSize)
	} else {
		cipherData = cipherData[:dataSize]
	}

	if iv != nil {
		// Put initialization vector in buffer before be encoded
		copy(cipherData, iv)
	}
	c.Encrypt(cipherData[len(iv):], preEncryptedData)
	return c.IObfs.Encode(cipherData)
}

func (c *SSTCPConn) Write(b []byte) (n int, err error) {
	outData, err := c.preWrite(b)
	if err != nil {
		return 0, err
	}
	n, err = c.Conn.Write(outData)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}
