// protocol:
// format: [encrypted payload length] [Overhead] [encrypted payload] [Overhead]
// sizes: 2 bytes, aead.Overhead() bytes, n bytes, aead.Overhead() bytes
// max(n): 0x3FFF

package shadowaead

import (
	"crypto/cipher"
	"encoding/binary"
	"io"

	"github.com/nadoo/glider/pkg/pool"
)

const (
	lenSize = 2
	// max payload size: 16383 from ss-libev aead.c: CHUNK_SIZE_MASK
	maxPayload = 0x3FFF
	// buf size, shoud enough to save lenSize + aead.Overhead() + maxPayload + aead.Overhead()
	bufSize = 17 << 10
)

type writer struct {
	io.Writer
	cipher.AEAD
	nonce [32]byte
}

// NewWriter wraps an io.Writer with AEAD encryption.
func NewWriter(w io.Writer, aead cipher.AEAD) io.Writer { return newWriter(w, aead) }

func newWriter(w io.Writer, aead cipher.AEAD) *writer {
	return &writer{Writer: w, AEAD: aead}
}

// Write encrypts p and writes to the embedded io.Writer.
func (w *writer) Write(p []byte) (n int, err error) {
	buf := pool.GetBuffer(bufSize)
	defer pool.PutBuffer(buf)

	nonce := w.nonce[:w.NonceSize()]
	encLenSize := lenSize + w.Overhead()
	for nw := maxPayload; n < len(p); n += nw {
		if left := len(p) - n; left < maxPayload {
			nw = left
		}

		binary.BigEndian.PutUint16(buf[:lenSize], uint16(nw))
		w.Seal(buf[:0], nonce, buf[:lenSize], nil)
		increment(nonce)

		w.Seal(buf[:encLenSize], nonce, p[n:n+nw], nil)
		increment(nonce)

		if _, err = w.Writer.Write(buf[:encLenSize+nw+w.Overhead()]); err != nil {
			return
		}
	}
	return
}

type reader struct {
	io.Reader
	cipher.AEAD
	nonce  [32]byte
	buf    []byte
	offset int
}

// NewReader wraps an io.Reader with AEAD decryption.
func NewReader(r io.Reader, aead cipher.AEAD) io.Reader { return newReader(r, aead) }

func newReader(r io.Reader, aead cipher.AEAD) *reader {
	return &reader{Reader: r, AEAD: aead}
}

// NOTE: len(p) MUST >= max payload size + AEAD overhead.
func (r *reader) read(p []byte) (int, error) {
	nonce := r.nonce[:r.NonceSize()]

	// read encrypted lenSize + overhead
	p = p[:lenSize+r.Overhead()]
	if _, err := io.ReadFull(r.Reader, p); err != nil {
		return 0, err
	}

	// decrypt lenSize
	_, err := r.Open(p[:0], nonce, p, nil)
	increment(nonce)
	if err != nil {
		return 0, err
	}

	// get payload size
	size := int(binary.BigEndian.Uint16(p[:lenSize]))

	// read encrypted payload + overhead
	p = p[:size+r.Overhead()]
	if _, err := io.ReadFull(r.Reader, p); err != nil {
		return 0, err
	}

	// decrypt payload
	_, err = r.Open(p[:0], nonce, p, nil)
	increment(nonce)

	if err != nil {
		return 0, err
	}

	return size, nil
}

func (r *reader) Read(p []byte) (int, error) {
	if r.buf == nil {
		if len(p) >= maxPayload+r.Overhead() {
			return r.read(p)
		}

		buf := pool.GetBuffer(bufSize)
		n, err := r.read(buf)
		if err != nil {
			pool.PutBuffer(buf)
			return 0, err
		}

		r.buf = buf[:n]
		r.offset = 0
	}

	n := copy(p, r.buf[r.offset:])
	r.offset += n
	if r.offset == len(r.buf) {
		pool.PutBuffer(r.buf)
		r.buf = nil
	}

	return n, nil
}

// increment little-endian encoded unsigned integer b. Wrap around on overflow.
func increment(b []byte) {
	for i := range b {
		b[i]++
		if b[i] != 0 {
			return
		}
	}
}
