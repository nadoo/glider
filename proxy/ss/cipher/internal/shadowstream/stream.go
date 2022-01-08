package shadowstream

import (
	"crypto/cipher"
	"io"

	"github.com/nadoo/glider/pkg/pool"
)

const bufSize = 32 * 1024

type writer struct {
	io.Writer
	cipher.Stream
}

// NewWriter wraps an io.Writer with stream cipher encryption.
func NewWriter(w io.Writer, s cipher.Stream) io.Writer {
	return &writer{Writer: w, Stream: s}
}

func (w *writer) Write(p []byte) (n int, err error) {
	buf := pool.GetBuffer(bufSize)
	defer pool.PutBuffer(buf)

	for nw := 0; n < len(p) && err == nil; n += nw {
		end := n + len(buf)
		if end > len(p) {
			end = len(p)
		}
		w.XORKeyStream(buf, p[n:end])
		nw, err = w.Writer.Write(buf[:end-n])
	}
	return
}

type reader struct {
	io.Reader
	cipher.Stream
}

// NewReader wraps an io.Reader with stream cipher decryption.
func NewReader(r io.Reader, s cipher.Stream) io.Reader {
	return &reader{Reader: r, Stream: s}
}

func (r *reader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil {
		return 0, err
	}
	p = p[:n]
	r.XORKeyStream(p, p)
	return n, nil
}
