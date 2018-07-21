// https://tools.ietf.org/html/rfc6455

package ws

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/rand"
)

const (
	finalBit           = 1 << 7
	defaultFrameSize   = 1 << 13   // 8192
	maxFrameHeaderSize = 2 + 8 + 4 // Fixed header + length + mask
	maskBit            = 1 << 7
	opCodeBinary       = 2
	opClose            = 8
	maskKeyLen         = 4
)

type frame struct {
}

type frameWriter struct {
	io.Writer
	buf     []byte
	maskKey []byte
}

// FrameWriter returns a frame writer
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

		nr, er := r.Read(buf)
		if nr > 0 {
			n += int64(nr)
			buf[0] |= finalBit
			buf[0] |= opCodeBinary
			buf[1] |= maskBit

			lengthFieldLen := 0
			switch {
			case nr <= 125:
				buf[1] |= byte(nr)
				lengthFieldLen = 2
			case nr < 65536:
				buf[1] |= 126
				lengthFieldLen = 2
				binary.BigEndian.PutUint16(buf[2:2+lengthFieldLen], uint16(nr))
			default:
				buf[1] |= 127
				lengthFieldLen = 8
				binary.BigEndian.PutUint64(buf[2:2+lengthFieldLen], uint64(nr))
			}

			copy(buf[2+lengthFieldLen:], w.maskKey)
			payloadBuf := buf[2+lengthFieldLen+maskKeyLen:]
			for i := range payloadBuf {
				payloadBuf[i] = payloadBuf[i] ^ w.maskKey[i%4]
			}

			_, ew := w.Writer.Write(buf)
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
	buf      []byte
	leftover []byte
}

// FrameReader returns a chunked reader
func FrameReader(r io.Reader) io.Reader {
	return &frameReader{
		Reader: r,
		buf:    make([]byte, defaultFrameSize),
	}
}

func (r *frameReader) Read(b []byte) (int, error) {
	if len(r.leftover) > 0 {
		n := copy(b, r.leftover)
		r.leftover = r.leftover[n:]
		return n, nil
	}

	// get msg header
	_, err := io.ReadFull(r.Reader, r.buf[:2])
	if err != nil {
		return 0, err
	}

	// final := r.buf[0]&finalBit != 0
	// frameType := int(r.buf[0] & 0xf)
	// mask := r.buf[1]&maskBit != 0
	len := int64(r.buf[1] & 0x7f)
	switch len {
	case 126:
		io.ReadFull(r.Reader, r.buf[:2])
		len = int64(binary.BigEndian.Uint16(r.buf[0:]))
	case 127:
		io.ReadFull(r.Reader, r.buf[:8])
		len = int64(binary.BigEndian.Uint64(r.buf[0:]))
	}

	// get payload
	_, err = io.ReadFull(r.Reader, r.buf[:len])
	if err != nil {
		return 0, err
	}

	m := copy(b, r.buf[:len])
	if m < int(len) {
		r.leftover = r.buf[m:len]
	}

	return m, err
}
