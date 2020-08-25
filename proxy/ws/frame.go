// https://tools.ietf.org/html/rfc6455#section-5.2
//
// Frame Format
// 0                   1                   2                   3
// 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-------+-+-------------+-------------------------------+
// |F|R|R|R| opcode|M| Payload len |    Extended payload length    |
// |I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
// |N|V|V|V|       |S|             |   (if payload len==126/127)   |
// | |1|2|3|       |K|             |                               |
// +-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
// |     Extended payload length continued, if payload len == 127  |
// + - - - - - - - - - - - - - - - +-------------------------------+
// |                               |Masking-key, if MASK set to 1  |
// +-------------------------------+-------------------------------+
// | Masking-key (continued)       |          Payload Data         |
// +-------------------------------- - - - - - - - - - - - - - - - +
// :                     Payload Data continued ...                :
// + - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
// |                     Payload Data continued ...                |
// +---------------------------------------------------------------+

package ws

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/rand"
)

const (
	defaultFrameSize   = 4096
	maxFrameHeaderSize = 2 + 8 + 4 // Fixed header + length + mask
	maskKeyLen         = 4

	finalBit     byte = 1 << 7
	maskBit      byte = 1 << 7
	opCodeBinary byte = 2
)

type frameWriter struct {
	io.Writer
	buf     []byte
	maskKey []byte
}

// FrameWriter returns a frame writer.
func FrameWriter(w io.Writer) io.Writer {
	n := rand.Uint32()
	return &frameWriter{
		Writer:  w,
		buf:     make([]byte, maxFrameHeaderSize+defaultFrameSize),
		maskKey: []byte{byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)},
	}
}

func (w *frameWriter) Write(b []byte) (int, error) {
	n, err := w.ReadFrom(bytes.NewBuffer(b))
	return int(n), err
}

func (w *frameWriter) ReadFrom(r io.Reader) (n int64, err error) {
	for {
		buf := w.buf
		payloadBuf := buf[maxFrameHeaderSize:]

		nr, er := r.Read(payloadBuf)
		if nr > 0 {
			n += int64(nr)
			buf[0] = finalBit | opCodeBinary
			buf[1] = maskBit

			lengthFieldLen := 0
			switch {
			case nr <= 125:
				buf[1] |= byte(nr)
			case nr < 65536:
				buf[1] |= 126
				lengthFieldLen = 2
				binary.BigEndian.PutUint16(buf[2:2+lengthFieldLen], uint16(nr))
			default:
				buf[1] |= 127
				lengthFieldLen = 8
				binary.BigEndian.PutUint64(buf[2:2+lengthFieldLen], uint64(nr))
			}

			// header and length
			_, ew := w.Writer.Write(buf[:2+lengthFieldLen])
			if ew != nil {
				err = ew
				break
			}

			// maskkey
			_, ew = w.Writer.Write(w.maskKey)
			if ew != nil {
				err = ew
				break
			}

			// payload
			payloadBuf = payloadBuf[:nr]
			for i := range payloadBuf {
				payloadBuf[i] = payloadBuf[i] ^ w.maskKey[i%4]
			}

			_, ew = w.Writer.Write(payloadBuf)
			if ew != nil {
				err = ew
				break
			}
		}

		if er != nil {
			if er != io.EOF { // ignore EOF as per io.ReaderFrom contract
				err = er
			}
			break
		}

	}

	return n, err
}

type frameReader struct {
	io.Reader
	buf  [8]byte
	left int64
}

// FrameReader returns a chunked reader.
func FrameReader(r io.Reader) io.Reader {
	return &frameReader{Reader: r}
}

func (r *frameReader) Read(b []byte) (int, error) {
	if r.left == 0 {
		// get msg header
		_, err := io.ReadFull(r.Reader, r.buf[:2])
		if err != nil {
			return 0, err
		}

		// final := r.buf[0]&finalBit != 0
		// frameType := int(r.buf[0] & 0xf)
		// mask := r.buf[1]&maskBit != 0
		r.left = int64(r.buf[1] & 0x7f)
		switch r.left {
		case 126:
			_, err := io.ReadFull(r.Reader, r.buf[:2])
			if err != nil {
				return 0, err
			}
			r.left = int64(binary.BigEndian.Uint16(r.buf[:2]))
		case 127:
			_, err := io.ReadFull(r.Reader, r.buf[:8])
			if err != nil {
				return 0, err
			}
			r.left = int64(binary.BigEndian.Uint64(r.buf[:8]))
		}
	}

	readLen := int64(len(b))
	if readLen > r.left {
		readLen = r.left
	}

	m, err := r.Reader.Read(b[:readLen])
	if err != nil {
		return 0, err
	}

	r.left -= int64(m)
	return m, err
}
