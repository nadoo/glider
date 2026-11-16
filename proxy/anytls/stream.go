package anytls

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

var errStreamClosed = errors.New("stream closed")

type stream struct {
	id uint32
	s  *session

	in        chan []byte
	readBuf   []byte
	readDone  chan struct{}
	done      chan struct{}
	closeIn   sync.Once
	closeOnce sync.Once
	fin       *frameWrite // published by closeOnce
	readMu    sync.Mutex
	writeMu   sync.Mutex

	readDeadline  streamDeadline
	writeDeadline streamDeadline
}

func newStream(id uint32, s *session) *stream {
	return &stream{
		id: id, s: s, in: make(chan []byte, 32),
		readDone: make(chan struct{}), done: make(chan struct{}),
	}
}

func (st *stream) Read(p []byte) (int, error) {
	st.readMu.Lock()
	defer st.readMu.Unlock()
	for {
		if channelClosed(st.done) {
			return 0, net.ErrClosed
		}
		deadline := st.readDeadline.wait()
		if channelClosed(deadline) {
			return 0, os.ErrDeadlineExceeded
		}
		if len(p) == 0 {
			return 0, nil
		}
		if len(st.readBuf) > 0 {
			n := copy(p, st.readBuf)
			st.readBuf = st.readBuf[n:]
			return n, nil
		}
		// A peer FIN ends input after previously received data has been read.
		select {
		case st.readBuf = <-st.in:
			continue
		default:
		}
		select {
		case st.readBuf = <-st.in:
		case <-st.readDone:
			select {
			case st.readBuf = <-st.in:
			default:
				return 0, io.EOF
			}
		case <-st.done:
			return 0, net.ErrClosed
		case <-st.s.done:
			return 0, st.s.Err()
		case <-deadline:
			return 0, os.ErrDeadlineExceeded
		}
	}
}

func (st *stream) Write(p []byte) (int, error) {
	st.writeMu.Lock()
	defer st.writeMu.Unlock()
	if channelClosed(st.done) {
		return 0, errStreamClosed
	}
	if channelClosed(st.writeDeadline.wait()) {
		return 0, os.ErrDeadlineExceeded
	}
	written := 0
	for len(p) > 0 {
		n := min(len(p), maxFrameData)
		accepted, err := st.s.writeStreamFrame(st, frame{command: cmdPSH, streamID: st.id, data: p[:n]})
		written += accepted
		if err != nil {
			return written, err
		}
		p = p[n:]
	}
	return written, nil
}

func (st *stream) Close() error {
	st.closeOnce.Do(func() {
		close(st.done)
		st.readDeadline.stop()
		st.writeDeadline.stop()
		st.s.removeStream(st.id)
		// FIN is queued behind any frame already being written. Closing a stream
		// must not wait for a stalled transport or interrupt another stream.
		st.fin, _ = st.s.queueFrame(frame{command: cmdFIN, streamID: st.id}, nil, nil)
	})
	return nil
}

func (st *stream) closeRead() {
	st.closeIn.Do(func() { close(st.readDone) })
}

func (st *stream) push(data []byte) {
	if channelClosed(st.done) || channelClosed(st.readDone) || channelClosed(st.s.done) {
		return
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	select {
	case st.in <- cp:
	case <-st.done:
	case <-st.readDone:
	case <-st.s.done:
	}
}

func (st *stream) LocalAddr() net.Addr  { return st.s.conn.LocalAddr() }
func (st *stream) RemoteAddr() net.Addr { return st.s.conn.RemoteAddr() }
func (st *stream) SetDeadline(t time.Time) error {
	if err := st.SetReadDeadline(t); err != nil {
		return err
	}
	return st.SetWriteDeadline(t)
}
func (st *stream) SetReadDeadline(t time.Time) error {
	if channelClosed(st.done) || channelClosed(st.s.done) {
		return net.ErrClosed
	}
	st.readDeadline.set(t)
	return nil
}
func (st *stream) SetWriteDeadline(t time.Time) error {
	if channelClosed(st.done) || channelClosed(st.s.done) {
		return net.ErrClosed
	}
	st.writeDeadline.set(t)
	return nil
}
