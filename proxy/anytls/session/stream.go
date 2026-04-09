package session

import (
	"io"
	"net"
	"sync"
	"time"
)

// Stream implements net.Conn over a multiplexed session.
type Stream struct {
	id   uint32
	sess *Session

	pr *io.PipeReader
	pw *io.PipeWriter

	dieOnce  sync.Once
	dieErr   error
	CloseFunc func() error // overridable close hook
}

func newStream(id uint32, sess *Session) *Stream {
	pr, pw := io.Pipe()
	return &Stream{
		id:   id,
		sess: sess,
		pr:   pr,
		pw:   pw,
	}
}

func (s *Stream) Read(b []byte) (int, error) {
	return s.pr.Read(b)
}

func (s *Stream) Write(b []byte) (int, error) {
	if s.dieErr != nil {
		return 0, s.dieErr
	}
	return s.sess.writeDataFrame(s.id, b)
}

func (s *Stream) Close() error {
	if s.CloseFunc != nil {
		return s.CloseFunc()
	}
	return s.CloseRemote()
}

func (s *Stream) CloseRemote() error {
	var once bool
	s.dieOnce.Do(func() {
		s.dieErr = io.ErrClosedPipe
		s.pr.Close()
		once = true
	})
	if once {
		return s.sess.streamClosed(s.id)
	}
	return s.dieErr
}

// closeLocally closes the stream without notifying the remote peer.
func (s *Stream) closeLocally() {
	s.dieOnce.Do(func() {
		s.dieErr = net.ErrClosed
		s.pr.Close()
	})
}

func (s *Stream) LocalAddr() net.Addr {
	if ts, ok := s.sess.conn.(interface{ LocalAddr() net.Addr }); ok {
		return ts.LocalAddr()
	}
	return nil
}

func (s *Stream) RemoteAddr() net.Addr {
	if ts, ok := s.sess.conn.(interface{ RemoteAddr() net.Addr }); ok {
		return ts.RemoteAddr()
	}
	return nil
}

func (s *Stream) SetDeadline(t time.Time) error     { return nil }
func (s *Stream) SetReadDeadline(t time.Time) error  { return nil }
func (s *Stream) SetWriteDeadline(t time.Time) error { return nil }
