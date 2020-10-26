// protocol:
// format: [data length] [data]
// sizes: 2 bytes, n bytes
// max(n): 2^14 bytes
// [data]: [encrypted payload] + [Overhead]

package vmess

import (
	"crypto/cipher"
	"encoding/binary"
	"io"
	"net"

	"github.com/nadoo/glider/pool"
)

type aeadWriter struct {
	io.Writer
	cipher.AEAD
	nonce [32]byte
	count uint16
}

// AEADWriter returns a aead writer.
func AEADWriter(w io.Writer, aead cipher.AEAD, iv []byte) io.Writer {
	aw := &aeadWriter{Writer: w, AEAD: aead}
	copy(aw.nonce[2:], iv[2:12])
	return aw
}

func (w *aeadWriter) Write(b []byte) (n int, err error) {
	buf := pool.GetBuffer(chunkSize)
	defer pool.PutBuffer(buf)

	var lenBuf [lenSize]byte
	var writeLen, dataLen int

	nonce := w.nonce[:w.NonceSize()]
	for left := len(b); left != 0; {
		writeLen = left + w.Overhead()
		if writeLen > chunkSize {
			writeLen = chunkSize
		}
		dataLen = writeLen - w.Overhead()

		binary.BigEndian.PutUint16(lenBuf[:], uint16(writeLen))
		binary.BigEndian.PutUint16(nonce[:2], w.count)

		w.Seal(buf[:0], nonce, b[n:n+dataLen], nil)
		w.count++

		if _, err = (&net.Buffers{lenBuf[:], buf[:writeLen]}).WriteTo(w.Writer); err != nil {
			break
		}

		n += dataLen
		left -= dataLen
	}

	return
}

type aeadReader struct {
	io.Reader
	cipher.AEAD
	nonce  [32]byte
	count  uint16
	buf    []byte
	offset int
}

// AEADReader returns a aead reader.
func AEADReader(r io.Reader, aead cipher.AEAD, iv []byte) io.Reader {
	ar := &aeadReader{Reader: r, AEAD: aead}
	copy(ar.nonce[2:], iv[2:12])
	return ar
}

func (r *aeadReader) read(p []byte) (int, error) {
	if _, err := io.ReadFull(r.Reader, p[:lenSize]); err != nil {
		return 0, err
	}

	size := int(binary.BigEndian.Uint16(p[:lenSize]))
	p = p[:size]
	if _, err := io.ReadFull(r.Reader, p); err != nil {
		return 0, err
	}

	binary.BigEndian.PutUint16(r.nonce[:2], r.count)
	_, err := r.Open(p[:0], r.nonce[:r.NonceSize()], p, nil)
	r.count++

	if err != nil {
		return 0, err
	}

	return size - r.Overhead(), nil
}

func (r *aeadReader) Read(p []byte) (int, error) {
	if r.buf == nil {
		if len(p) >= chunkSize {
			return r.read(p)
		}

		buf := pool.GetBuffer(chunkSize)
		n, err := r.read(buf)
		if err != nil || n == 0 {
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
