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
	chunkSizeEncoder ChunkSizeEncoder
	cipher.AEAD
	nonce [32]byte
	count uint16
}

// AEADWriter returns a aead writer.
func AEADWriter(w io.Writer, aead cipher.AEAD, iv []byte, chunkSizeEncoder ChunkSizeEncoder) io.Writer {
	aw := &aeadWriter{Writer: w, AEAD: aead, chunkSizeEncoder: chunkSizeEncoder}
	copy(aw.nonce[2:], iv[2:aead.NonceSize()])
	return aw
}

func (w *aeadWriter) Write(b []byte) (n int, err error) {
	buf := pool.GetBuffer(chunkSize)
	defer pool.PutBuffer(buf)

	lenBuf := make([]byte, w.chunkSizeEncoder.SizeBytes())
	var writeLen, dataLen int

	nonce := w.nonce[:w.NonceSize()]
	for left := len(b); left != 0; {
		writeLen = left + w.Overhead()
		if writeLen > chunkSize {
			writeLen = chunkSize
		}
		dataLen = writeLen - w.Overhead()

		w.chunkSizeEncoder.Encode(uint16(writeLen), lenBuf)
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
	chunkSizeDecoder ChunkSizeDecoder
	cipher.AEAD
	nonce  [32]byte
	count  uint16
	buf    []byte
	offset int
}

// AEADReader returns a aead reader.
func AEADReader(r io.Reader, aead cipher.AEAD, iv []byte, chunkSizeDecoder ChunkSizeDecoder) io.Reader {
	ar := &aeadReader{Reader: r, AEAD: aead, chunkSizeDecoder: chunkSizeDecoder}
	copy(ar.nonce[2:], iv[2:aead.NonceSize()])
	return ar
}

func (r *aeadReader) readChunkPool() ([]byte, error) {
	bSize := pool.GetBuffer(int(r.chunkSizeDecoder.SizeBytes()))
	defer pool.PutBuffer(bSize)
	if _, err := io.ReadFull(r.Reader, bSize); err != nil {
		return nil, err
	}

	size, err := r.chunkSizeDecoder.Decode(bSize)
	if err != nil {
		return nil, err
	}
	chunk := pool.GetBuffer(int(size))
	if _, err := io.ReadFull(r.Reader, chunk); err != nil {
		return nil, err
	}

	binary.BigEndian.PutUint16(r.nonce[:2], r.count)
	_, err = r.Open(chunk[:0], r.nonce[:r.NonceSize()], chunk, nil)
	r.count++

	if err != nil {
		return nil, err
	}

	return chunk[:int(size)-r.Overhead()], nil
}

func (r *aeadReader) Read(p []byte) (int, error) {
	if r.buf != nil {
		n := copy(p, r.buf[r.offset:])
		r.offset += n
		if r.offset >= len(r.buf) {
			pool.PutBuffer(r.buf)
			r.buf = nil
		}
		return n, nil
	}
	chunk, err := r.readChunkPool()
	if err != nil {
		return 0, err
	}
	n := copy(p, chunk)
	if len(chunk) > len(p) {
		r.buf = chunk
		r.offset = len(p)
	} else {
		pool.PutBuffer(chunk)
	}
	return n, nil
}
