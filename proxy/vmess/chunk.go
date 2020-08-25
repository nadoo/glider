package vmess

import (
	"encoding/binary"
	"io"

	"github.com/nadoo/glider/common/pool"
)

const (
	lenSize   = 2
	chunkSize = 1 << 14 // 16384
)

type chunkedWriter struct {
	io.Writer
}

// ChunkedWriter returns a chunked writer.
func ChunkedWriter(w io.Writer) io.Writer {
	return &chunkedWriter{Writer: w}
}

func (w *chunkedWriter) Write(b []byte) (n int, err error) {
	buf := pool.GetBuffer(lenSize + chunkSize)
	defer pool.PutBuffer(buf)

	for left := len(b); left != 0; {
		writeLen := left
		if writeLen > chunkSize {
			writeLen = chunkSize
		}

		copy(buf[lenSize:], b[n:n+writeLen])
		binary.BigEndian.PutUint16(buf[:lenSize], uint16(writeLen))

		_, err = w.Writer.Write(buf[:lenSize+writeLen])
		if err != nil {
			break
		}

		n += writeLen
		left -= writeLen
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

func (r *chunkedReader) Read(b []byte) (int, error) {
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

	readLen := len(b)
	if readLen > r.left {
		readLen = r.left
	}

	n, err := r.Reader.Read(b[:readLen])
	if err != nil {
		return 0, err
	}

	r.left -= n

	return n, err
}
