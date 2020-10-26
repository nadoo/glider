package vmess

import (
	"encoding/binary"
	"io"
	"net"
)

const (
	lenSize   = 2
	chunkSize = 16 << 10
)

type chunkedWriter struct {
	io.Writer
	buf [lenSize]byte
}

// ChunkedWriter returns a chunked writer.
func ChunkedWriter(w io.Writer) io.Writer {
	return &chunkedWriter{Writer: w}
}

func (w *chunkedWriter) Write(p []byte) (n int, err error) {
	var dataLen int
	for left := len(p); left != 0; {
		dataLen = left
		if dataLen > chunkSize {
			dataLen = chunkSize
		}

		binary.BigEndian.PutUint16(w.buf[:], uint16(dataLen))
		if _, err = (&net.Buffers{w.buf[:], p[n : n+dataLen]}).WriteTo(w.Writer); err != nil {
			break
		}

		n += dataLen
		left -= dataLen
	}
	return
}

type chunkedReader struct {
	io.Reader
	buf  [lenSize]byte
	left int
}

// ChunkedReader returns a chunked reader.
func ChunkedReader(r io.Reader) io.Reader {
	return &chunkedReader{Reader: r}
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.left == 0 {
		// get length
		_, err := io.ReadFull(r.Reader, r.buf[:lenSize])
		if err != nil {
			return 0, err
		}
		r.left = int(binary.BigEndian.Uint16(r.buf[:lenSize]))

		// if left == 0, then this is the end
		if r.left == 0 {
			return 0, nil
		}
	}

	readLen := len(p)
	if readLen > r.left {
		readLen = r.left
	}

	n, err := r.Reader.Read(p[:readLen])
	if err != nil {
		return 0, err
	}

	r.left -= n

	return n, err
}
