// https://www.rfc-editor.org/rfc/rfc6455#section-5.2
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
	"encoding/binary"
	"io"
	"math/rand"
	"net"

	"github.com/nadoo/glider/pkg/pool"
)

const (
	hdrSize = 2 + 8 // Fixed header + length

	// byte 0
	finalBit     byte = 1 << 7
	opCodeBinary byte = 2

	// byte 1
	maskBit byte = 1 << 7
)

type frameWriter struct {
	io.Writer
	header  [hdrSize]byte
	server  bool
	maskKey [4]byte
}

// FrameWriter returns a frame writer.
func FrameWriter(w io.Writer, server bool) io.Writer {
	n := rand.Uint32()
	return &frameWriter{
		Writer:  w,
		server:  server,
		maskKey: [4]byte{byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)},
	}
}

func (w *frameWriter) Write(b []byte) (int, error) {
	hdr := w.header
	hdr[0], hdr[1] = opCodeBinary|finalBit, 0
	if !w.server {
		hdr[1] = maskBit
	}

	nPayload, nHeader := len(b), 2
	switch {
	case nPayload <= 125:
		hdr[1] |= byte(nPayload)
	case nPayload < 65536:
		hdr[1] |= 126
		nHeader += 2
		binary.BigEndian.PutUint16(hdr[2:nHeader], uint16(nPayload))
	default:
		hdr[1] |= 127
		nHeader += 8
		binary.BigEndian.PutUint64(hdr[2:nHeader], uint64(nPayload))
	}

	header := hdr[:nHeader]
	if w.server {
		n, err := (&net.Buffers{header, b}).WriteTo(w.Writer)
		if int(n) > nHeader {
			return int(n) - nHeader, err
		}
		return 0, err
	}

	payload := pool.GetBuffer(nPayload)
	defer pool.PutBuffer(payload)

	// payload with mask
	for i := 0; i < nPayload; i++ {
		payload[i] = b[i] ^ w.maskKey[i%4]
	}

	n, err := (&net.Buffers{header, w.maskKey[:], payload}).WriteTo(w.Writer)
	if int(n) > nHeader+4 {
		return int(n) - nHeader - 4, err
	}
	return 0, err
}

type frameReader struct {
	io.Reader
	buf        [8]byte
	left       int64
	server     bool
	maskKey    [4]byte
	maskOffset int
}

// FrameReader returns a chunked reader.
func FrameReader(r io.Reader, server bool) io.Reader {
	return &frameReader{Reader: r, server: server}
}

func (r *frameReader) Read(b []byte) (int, error) {
	if r.left == 0 {
		// get msg header
		_, err := io.ReadFull(r.Reader, r.buf[:2])
		if err != nil {
			return 0, err
		}

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

		if r.server {
			_, err := io.ReadFull(r.Reader, r.maskKey[:])
			if err != nil {
				return 0, err
			}
			r.maskOffset = 0
		}
	}

	readLen := int64(len(b))
	if readLen > r.left {
		readLen = r.left
	}

	m, err := io.ReadFull(r.Reader, b[:readLen])
	if err != nil {
		return m, err
	}

	if r.server {
		for i := range b[:m] {
			b[i] = b[i] ^ r.maskKey[(i+r.maskOffset)%4]
		}
		r.maskOffset = (m + r.maskOffset) % 4
	}

	r.left -= int64(m)
	return m, err
}
