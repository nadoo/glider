package vmess

import (
	"io"
	"net"
)

const (
	chunkSize = 16 << 10
)

type chunkedWriter struct {
	io.Writer
	chunkSizeEncoder ChunkSizeEncoder
	buf              []byte
}

// ChunkedWriter returns a chunked writer.
func ChunkedWriter(w io.Writer, chunkSizeEncoder ChunkSizeEncoder) io.Writer {
	return &chunkedWriter{Writer: w, chunkSizeEncoder: chunkSizeEncoder, buf: make([]byte, chunkSizeEncoder.SizeBytes())}
}

func (w *chunkedWriter) Write(p []byte) (n int, err error) {
	var dataLen int
	for left := len(p); left != 0; {
		dataLen = left
		if dataLen > chunkSize {
			dataLen = chunkSize
		}
		w.chunkSizeEncoder.Encode(uint16(dataLen), w.buf)
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
	chunkSizeDecoder ChunkSizeDecoder
	buf              []byte
	left             int
}

// ChunkedReader returns a chunked reader.
func ChunkedReader(r io.Reader, chunkSizeDecoder ChunkSizeDecoder) io.Reader {
	return &chunkedReader{Reader: r, chunkSizeDecoder: chunkSizeDecoder, buf: make([]byte, chunkSizeDecoder.SizeBytes())}
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.left == 0 {
		// get length
		_, err := io.ReadFull(r.Reader, r.buf[:r.chunkSizeDecoder.SizeBytes()])
		if err != nil {
			return 0, err
		}
		n, err := r.chunkSizeDecoder.Decode(r.buf[:])
		if err != nil {
			return 0, err
		}
		r.left = int(n)

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
