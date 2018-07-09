package vmess

import (
	"bytes"
	"encoding/binary"
	"io"
)

// chunk: plain, AES-128-CFB, AES-128-GCM, ChaCha20-Poly1305

const maxChunkSize = 1 << 14     // 16384
const defaultChunkSize = 1 << 13 // 8192

type chunkedReader struct {
	io.Reader
	buf      []byte
	leftover []byte
}

func newChunkedReader(r io.Reader) io.Reader {
	return &chunkedReader{
		Reader: r,
		buf:    make([]byte, 2+maxChunkSize),
	}
}

func (r *chunkedReader) read() (int, error) {
	lenBuf := make([]byte, 2)
	_, err := io.ReadFull(r.Reader, lenBuf)
	if err != nil {
		return 0, err
	}

	len := binary.BigEndian.Uint16(lenBuf)

	buf := r.buf[:len]
	_, err = io.ReadFull(r.Reader, buf)
	if err != nil {
		return 0, err
	}

	return int(len), nil
}

func (r *chunkedReader) Read(b []byte) (int, error) {
	if len(r.leftover) > 0 {
		n := copy(b, r.leftover)
		r.leftover = r.leftover[n:]
		return n, nil
	}

	n, err := r.read()
	m := copy(b, r.buf[:n])
	if m < n {
		r.leftover = r.buf[m:n]
	}

	return m, err
}

type chunkedWriter struct {
	io.Writer
	buf []byte
}

func newChunkedWriter(w io.Writer) io.Writer {
	return &chunkedWriter{
		Writer: w,
		buf:    make([]byte, 2+maxChunkSize),
	}
}

func (w *chunkedWriter) Write(b []byte) (int, error) {
	n, err := w.ReadFrom(bytes.NewBuffer(b))
	return int(n), err
}

func (w *chunkedWriter) ReadFrom(r io.Reader) (n int64, err error) {
	for {
		buf := w.buf
		payloadBuf := buf[2 : 2+defaultChunkSize]

		nr, er := r.Read(payloadBuf)
		if nr > 0 {
			n += int64(nr)
			payloadBuf = payloadBuf[:nr]
			binary.BigEndian.PutUint16(buf[:], uint16(nr))

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
